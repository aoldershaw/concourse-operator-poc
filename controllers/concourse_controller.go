/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	deployv1alpha1 "github.com/aoldershaw/concourse-operator-poc/api/v1alpha1"
)

// ConcourseReconciler reconciles a Concourse object
type ConcourseReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=deploy.concourse-ci.org,resources=concourses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=deploy.concourse-ci.org,resources=concourses/status,verbs=get;update;patch

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=list;watch;get;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=list;watch;get;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=list;watch;get;create
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=list;watch;get;create

func (r *ConcourseReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logger := r.Log.WithValues("concourse", req.NamespacedName)

	logger.Info("reconciling concourse")

	var concourse deployv1alpha1.Concourse
	if err := r.Get(ctx, req.NamespacedName, &concourse); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "get-concourse")
			return ctrl.Result{}, err
		}
		logger.Info("concourse-not-found")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// TODO: clean up this awful spaghetti code

	var fetchDBVersionJobPtr *batchv1.Job
	var migrateDBJobPtr *batchv1.Job
	// If the image tag changed from what it was before, then fetch NextDBVersion
	// TODO: What if underlying image changes, but image tag doesn't
	if concourse.Spec.Image != concourse.Status.ActiveImage {
		fetchDBVersionJobKey := client.ObjectKey{
			Name:      fetchDBVersionJobName(concourse),
			Namespace: concourse.Namespace,
		}
		var fetchDBVersionJob batchv1.Job
		if err := r.Get(ctx, fetchDBVersionJobKey, &fetchDBVersionJob); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "get-fetch-db-version-job")
				return ctrl.Result{}, err
			}
			// The Job doesn't yet exist - create it
			fetchDBVersionJob, err = r.generateFetchDBVersionJob(concourse)
			if err != nil {
				logger.Error(err, "generate-fetch-db-version-job")
				return ctrl.Result{}, err
			}
			err = r.Create(ctx, &fetchDBVersionJob)
			if err != nil {
				logger.Error(err, "create-fetch-db-version-job")
				return ctrl.Result{}, err
			}
			logger.Info("created-fetch-db-version-job")
			return ctrl.Result{}, nil
		}
		fetchDBVersionJobPtr = &fetchDBVersionJob
		// Job exists. Check if it's succeeded
		if fetchDBVersionJob.Status.Succeeded == 0 {
			logger.Info("awaiting-success",
				"active", fetchDBVersionJob.Status.Active,
				"failed", fetchDBVersionJob.Status.Failed,
			)
			return ctrl.Result{}, nil
		}
		dbVersionStr, hasDBVersion := fetchDBVersionJob.Annotations[dbVersionAnnotation]
		// Job should be setting an annotation on itself. Not sure what to do if it succeeded, but there's no annotation
		if !hasDBVersion {
			err := errors.New("no DB version in annotations")
			logger.Error(err, "missing-annotation", "annotations", fetchDBVersionJob.Annotations)
			// TODO: what do we do here? delete the job? otherwise, we'll end up in infinite loop
			return ctrl.Result{}, err
		}
		dbVersion, err := strconv.ParseInt(dbVersionStr, 10, 64)
		if err != nil {
			logger.Error(err, "invalid-db-version", "dbVersion", dbVersionStr)
			// TODO: what do we do here? delete the job? otherwise, we'll end up in infinite loop
			return ctrl.Result{}, err
		}
		// Note that this update may not get applied - only if we successfully completed the rollback,
		// or if we successfully deployed
		activeDBVersion := concourse.Status.DBVersion
		concourse.Status.DBVersion = &dbVersion
		logger.Info("fetch-db-job-succeeded",
			"desiredDBVersion", dbVersion,
			"activeDBVersion", activeDBVersion,
		)
		if isRollback(dbVersion, activeDBVersion) {
			migrateDBJobKey := client.ObjectKey{
				Name:      migrateDBJobName(concourse),
				Namespace: concourse.Namespace,
			}
			var migrateDBJob batchv1.Job
			if err := r.Get(ctx, migrateDBJobKey, &migrateDBJob); err != nil {
				if !apierrors.IsNotFound(err) {
					logger.Error(err, "get-migrate-db-job")
					return ctrl.Result{}, err
				}
				// The Job doesn't yet exist - create it
				migrateDBJob, err = r.generateMigrateDBJob(concourse, dbVersion)
				if err != nil {
					logger.Error(err, "generate-migrate-db-job")
					return ctrl.Result{}, err
				}
				err = r.Create(ctx, &migrateDBJob)
				if err != nil {
					logger.Error(err, "create-migrate-db-job")
					return ctrl.Result{}, err
				}
				logger.Info("created-migrate-db-job")
				return ctrl.Result{}, nil
			}
			migrateDBJobPtr = &migrateDBJob
			// Job exists. Check if it's succeeded
			if migrateDBJob.Status.Succeeded == 0 {
				logger.Info("awaiting-success",
					"active", fetchDBVersionJob.Status.Active,
					"failed", fetchDBVersionJob.Status.Failed,
				)
				return ctrl.Result{}, nil
			}
			logger.Info("migration-successful")
			// Successfully migrated. Update DB Version on the Status
			err = r.Status().Update(ctx, &concourse)
			if err != nil {
				logger.Error(err, "update-status-db-version")
				return ctrl.Result{}, err
			}
		}
	}

	// TODO: how to do rotations
	secretKey := client.ObjectKey{
		Name:      getSecretName(concourse),
		Namespace: concourse.Namespace,
	}
	var secret corev1.Secret
	if err := r.Get(ctx, secretKey, &secret); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "get-secret")
			return ctrl.Result{}, err
		}
		logger.Info("secret-not-found")
		secret, err = generateSecret(concourse)
		if err != nil {
			logger.Error(err, "generate-secret")
			return ctrl.Result{}, err
		}
		err = r.Create(ctx, &secret)
		if err != nil {
			logger.Error(err, "create-secret")
			return ctrl.Result{}, err
		}
	}

	atcDeployment, err := r.desiredATCDeployment(concourse)
	if err != nil {
		logger.Error(err, "desired-atc-deployment")
		return ctrl.Result{}, err
	}
	atcService, err := r.desiredATCService(concourse)
	if err != nil {
		logger.Error(err, "desired-atc-service")
		return ctrl.Result{}, err
	}
	workerDeployment, err := r.desiredWorkerDeployment(concourse)
	if err != nil {
		logger.Error(err, "desired-worker-service")
		return ctrl.Result{}, err
	}

	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner("concourse-controller")}

	// Note: this relies on Server-Side Apply, think you need 1.16+ ...
	// TODO: make this work on older K8s
	err = r.Patch(ctx, &atcDeployment, client.Apply, applyOpts...)
	if err != nil {
		logger.Error(err, "apply-atc-deployment")
		return ctrl.Result{}, err
	}
	err = r.Patch(ctx, &atcService, client.Apply, applyOpts...)
	if err != nil {
		logger.Error(err, "apply-atc-service")
		return ctrl.Result{}, err
	}
	err = r.Patch(ctx, &workerDeployment, client.Apply, applyOpts...)
	if err != nil {
		logger.Error(err, "apply-worker-deployment")
		return ctrl.Result{}, err
	}

	concourse.Status.ActiveImage = concourse.Spec.Image
	concourse.Status.ATCURL = urlForService(atcService, 8080)
	err = r.Status().Update(ctx, &concourse)
	if err != nil {
		logger.Error(err, "update-status")
		return ctrl.Result{}, err
	}

	propPolicy := v1.DeletePropagationBackground
	deleteOpts := &client.DeleteOptions{PropagationPolicy: &propPolicy}
	// At this point, we can safely delete both jobs
	if migrateDBJobPtr != nil {
		err = r.Delete(ctx, migrateDBJobPtr, deleteOpts)
		if err != nil {
			logger.Error(err, "delete-migrate-db-job")
			return ctrl.Result{}, err
		}
	}
	if fetchDBVersionJobPtr != nil {
		err = r.Delete(ctx, fetchDBVersionJobPtr, deleteOpts)
		if err != nil {
			logger.Error(err, "delete-fetch-db-version-job")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *ConcourseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&deployv1alpha1.Concourse{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.Deployment{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
