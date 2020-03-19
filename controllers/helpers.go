package controllers

import (
	"fmt"
	"github.com/aoldershaw/concourse-operator-poc/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"net"
	ctrl "sigs.k8s.io/controller-runtime"
)

func baseLabels(concourse v1alpha1.Concourse) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "concourse",
		"app.kubernetes.io/instance": concourse.Name,
	}
}

func atcLabels(concourse v1alpha1.Concourse) map[string]string {
	labels := baseLabels(concourse)
	labels["app.kubernetes.io/component"] = "atc"
	return labels
}

func workerLabels(concourse v1alpha1.Concourse) map[string]string {
	labels := baseLabels(concourse)
	labels["app.kubernetes.io/component"] = "worker"
	return labels
}

func atcServiceName(concourse v1alpha1.Concourse) string {
	return concourse.Name + "-atc"
}

func (r *ConcourseReconciler) desiredATCDeployment(concourse v1alpha1.Concourse) (appsv1.Deployment, error) {
	web := concourse.Spec.WebSpec
	psql := web.PostgresSpec
	env := []corev1.EnvVar{
		{Name: "CONCOURSE_LOG_LEVEL", Value: "debug"},
		{Name: "CONCOURSE_POSTGRES_HOST", Value: psql.Host},
		{Name: "CONCOURSE_POSTGRES_PORT", Value: psql.Port},
		{Name: "CONCOURSE_POSTGRES_USER", ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: psql.CredentialsSecret},
				Key:                  "username",
			},
		}},
		{Name: "CONCOURSE_POSTGRES_PASSWORD", ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: psql.CredentialsSecret},
				Key:                  "password",
			},
		}},
		{Name: "CONCOURSE_POSTGRES_DATABASE", Value: psql.Database},
		{Name: "CONCOURSE_CLUSTER_NAME", Value: web.ClusterName},
	}
	// TODO: not like this
	if true {
		env = append(env,
			corev1.EnvVar{Name: "CONCOURSE_ADD_LOCAL_USER", Value: "test:test,guest:guest"},
			corev1.EnvVar{Name: "CONCOURSE_MAIN_TEAM_LOCAL_USER", Value: "test"},
		)
	}
	// TODO: is there a better way to set CONCOURSE_EXTERNAL_URL without needing a rollout?
	// I guess if we configure the Ingress, we'll know - but for now, relying on LoadBalancer Service,
	// I don't think there's another way
	if concourse.Status.ATCURL != "" {
		env = append(env,
			corev1.EnvVar{Name: "CONCOURSE_EXTERNAL_URL", Value: concourse.Status.ATCURL},
		)
	}
	depl := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      concourse.Name,
			Namespace: concourse.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: web.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: atcLabels(concourse),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: atcLabels(concourse),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "atc",
							Image: "aoldershaw/concourse:local",
							Env:   env,
							Args:  []string{"web"},
						},
					},
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(&concourse, &depl, r.Scheme); err != nil {
		return depl, err
	}
	return depl, nil
}

func (r *ConcourseReconciler) desiredATCService(concourse v1alpha1.Concourse) (corev1.Service, error) {
	svc := corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      atcServiceName(concourse),
			Namespace: concourse.Namespace,
			Labels:    atcLabels(concourse),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 8080, Protocol: "TCP", TargetPort: intstr.FromString("http")},
			},
			Selector: atcLabels(concourse),
			// TODO: this should probably have an Ingress instead
			Type: corev1.ServiceTypeLoadBalancer,
		},
	}

	// always set the controller reference so that we know which object owns this.
	if err := ctrl.SetControllerReference(&concourse, &svc, r.Scheme); err != nil {
		return svc, err
	}
	return svc, nil
}

func (r *ConcourseReconciler) desiredWorkerDeployment(concourse v1alpha1.Concourse) (appsv1.Deployment, error) {
	worker := concourse.Spec.WorkerSpec
	env := []corev1.EnvVar{
		{Name: "CONCOURSE_LOG_LEVEL", Value: "debug"},
		{Name: "CONCOURSE_TSA_HOST", Value: atcServiceName(concourse) + ":2222"},
		{Name: "CONCOURSE_BAGGAGECLAIM_DRIVER", Value: "overlay"},
		{Name: "CONCOURSE_BIND_IP", Value: "0.0.0.0"},
		{Name: "CONCOURSE_BAGGAGE_CLAIM_BIND_IP", Value: "0.0.0.0"},
	}
	depl := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      concourse.Name,
			Namespace: concourse.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: worker.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: workerLabels(concourse),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: workerLabels(concourse),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "atc",
							Image: "aoldershaw/concourse:local",
							Env:   env,
							Args:  []string{"worker"},
						},
					},
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(&concourse, &depl, r.Scheme); err != nil {
		return depl, err
	}
	return depl, nil
}

func urlForService(svc corev1.Service, port int32) string {
	// TODO: what about Ingress?
	// notice that we unset this if it's not present -- we always want the
	// state to reflect what we observe.
	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		return ""
	}

	host := svc.Status.LoadBalancer.Ingress[0].Hostname
	if host == "" {
		host = svc.Status.LoadBalancer.Ingress[0].IP
	}

	return fmt.Sprintf("http://%s", net.JoinHostPort(host, fmt.Sprintf("%v", port)))
}
