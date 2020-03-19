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

func (r *ConcourseReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logger := r.Log.WithValues("concourse", req.NamespacedName)

	logger.Info("reconciling concourse")

	// TODO: create secrets (one of the main purposes of the operator...)

	var concourse deployv1alpha1.Concourse
	if err := r.Get(ctx, req.NamespacedName, &concourse); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
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

	concourse.Status.ATCURL = urlForService(atcService, 8080)
	err = r.Status().Update(ctx, &concourse)
	if err != nil {
		logger.Error(err, "update-status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ConcourseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&deployv1alpha1.Concourse{}).
		Complete(r)
}
