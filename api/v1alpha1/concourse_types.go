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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TODO: a lot of properties...https://github.com/concourse/concourse-chart/blob/master/values.yaml

// ConcourseSpec defines the desired state of Concourse
type ConcourseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	WebSpec    WebSpec    `json:"web"`
	WorkerSpec WorkerSpec `json:"worker"`
}

type WebSpec struct {
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas"`

	PostgresSpec string `json:"postgres"`

	// +optional
	ClusterName string `json:"clusterName,omitempty"`
}

type PostgresSpec struct {
	Host string `json:"host"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=5432
	Port string `json:"port"`

	// TODO: what's the best way to do this
	CredentialsSecret string `json:"credentialsSecretName"`
}

type WorkerSpec struct {
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=2
	// +optional
	Replicas *int32 `json:"replicas"`

	// +optional
	WorkDir string `json:"work"`
}

// ConcourseStatus defines the observed state of Concourse
type ConcourseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ATCURL      string   `json:"url"`
	WorkerNames []string `json:"workerNames"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Concourse is the Schema for the concourses API
type Concourse struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConcourseSpec   `json:"spec,omitempty"`
	Status ConcourseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConcourseList contains a list of Concourse
type ConcourseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Concourse `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Concourse{}, &ConcourseList{})
}
