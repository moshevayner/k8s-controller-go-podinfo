/*
Copyright 2023.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type Resources struct {
	MemoryRequest string `json:"memoryRequest,omitempty"`
	MemoryLimit   string `json:"memoryLimit,omitempty"`
	CPURequest    string `json:"cpuRequest,omitempty"`
	CPULimit      string `json:"cpuLimit,omitempty"`
}

type Image struct {
	Repository string `json:"repository,omitempty"`
	Tag        string `json:"tag,omitempty"`
}

type UI struct {
	Color   string `json:"color,omitempty"`
	Message string `json:"message,omitempty"`
}

type Redis struct {
	Enabled   bool      `json:"enabled,omitempty"`
	Image     Image     `json:"image,omitempty"`
	Resources Resources `json:"resources,omitempty"`
	// TODO In the future we can consider supporting external Redis for better resiliency (i.e. AWS ElastiCache), but for the POC we'll just use an in-cluster one
	// Host    string `json:"host,omitempty"`
	// Port    int    `json:"port,omitempty"`
}

// PodInfoInstanceSpec defines the desired state of PodInfoInstance
type PodInfoInstanceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of PodInfoInstance. Edit podinfoinstance_types.go to remove/update
	ReplicaCount int32     `json:"replicaCount,omitempty"`
	Resources    Resources `json:"resources,omitempty"`
	Image        Image     `json:"image,omitempty"`
	UI           UI        `json:"ui,omitempty"`
	Redis        Redis     `json:"redis,omitempty"`
}

type Status struct {
	Name   string   `json:"name,omitempty"`
	Errors []string `json:"errors,omitempty"`
}

// PodInfoInstanceStatus defines the observed state of PodInfoInstance
type PodInfoInstanceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	AppDeployment   Status `json:"appDeployment,omitempty"`
	AppService      Status `json:"appService,omitempty"`
	RedisDeployment Status `json:"redis,omitempty"`
	RedisService    Status `json:"redisService,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PodInfoInstance is the Schema for the podinfoinstances API
type PodInfoInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodInfoInstanceSpec   `json:"spec,omitempty"`
	Status PodInfoInstanceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PodInfoInstanceList contains a list of PodInfoInstance
type PodInfoInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodInfoInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodInfoInstance{}, &PodInfoInstanceList{})
}
