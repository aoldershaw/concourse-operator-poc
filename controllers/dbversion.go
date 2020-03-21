package controllers

import (
	"errors"
	"fmt"
	deployv1alpha1 "github.com/aoldershaw/concourse-operator-poc/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const dbVersionAnnotation = "concourse.dbVersion"

func fetchDBVersionJobName(concourse deployv1alpha1.Concourse) string {
	return concourse.Name + "-fetch-db-version"
}

func migrateDBJobName(concourse deployv1alpha1.Concourse) string {
	return concourse.Name + "-migrate-db"
}

func fetchDBVersionScript(concourse deployv1alpha1.Concourse) string {
	// TODO: don't install curl...maybe build a go binary and use client-go? How to mount that binary?
	// TODO: create service account and role/binding for this in code
	return fmt.Sprintf(`
supported_db_version="$(/usr/local/concourse/bin/concourse migrate --supported-db-version)"
apt-get update && apt-get install -y curl
curl \
	-X PATCH \
	--data "{\"metadata\": {\"annotations\": {\"%s\": \"${supported_db_version}\"}}}" \
	--cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt \
	-H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" \
	-H "Content-Type: application/merge-patch+json" \
	-H 'Accept: application/json' \
	"https://kubernetes.default.svc/apis/batch/v1/namespaces/%s/jobs/%s"
`,
		dbVersionAnnotation,
		concourse.Namespace,
		fetchDBVersionJobName(concourse),
	)
}

func (r *ConcourseReconciler) generateFetchDBVersionJob(concourse deployv1alpha1.Concourse) (batchv1.Job, error) {
	env := psqlEnv(concourse.Spec.WebSpec.PostgresSpec)
	job := batchv1.Job{
		TypeMeta: v1.TypeMeta{APIVersion: batchv1.SchemeGroupVersion.String(), Kind: "Job"},
		ObjectMeta: v1.ObjectMeta{
			Name:      fetchDBVersionJobName(concourse),
			Namespace: concourse.Namespace,
		},
		Spec: batchv1.JobSpec{
			Completions: int32Ptr(1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "fetcher",
							Image:   concourse.Spec.Image,
							Env:     env,
							Command: []string{"bash"},
							Args:    []string{"-cex", fetchDBVersionScript(concourse)},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(&concourse, &job, r.Scheme); err != nil {
		return job, err
	}
	return job, nil
}

func (r *ConcourseReconciler) generateMigrateDBJob(concourse deployv1alpha1.Concourse, dbVersion int64) (batchv1.Job, error) {
	if concourse.Status.ActiveImage == "" {
		return batchv1.Job{}, errors.New("status has no ActiveImage")
	}
	env := psqlEnv(concourse.Spec.WebSpec.PostgresSpec)
	job := batchv1.Job{
		TypeMeta: v1.TypeMeta{APIVersion: batchv1.SchemeGroupVersion.String(), Kind: "Job"},
		ObjectMeta: v1.ObjectMeta{
			Name:      migrateDBJobName(concourse),
			Namespace: concourse.Namespace,
		},
		Spec: batchv1.JobSpec{
			Completions: int32Ptr(1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "migrator",
							Image: concourse.Status.ActiveImage,
							Env:   env,
							Args:  []string{"migrate", fmt.Sprintf("--migrate-db-to-version=%d", dbVersion)},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(&concourse, &job, r.Scheme); err != nil {
		return job, err
	}
	return job, nil
}

func isRollback(desiredVersion int64, activeVersion *int64) bool {
	if activeVersion == nil {
		return false
	}
	return desiredVersion < *activeVersion
}
