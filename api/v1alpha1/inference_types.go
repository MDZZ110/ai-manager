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

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	InferenceFinalizer = "finalizer.inference.ai.manager.io"
	LabelModelName     = "ai.manager.io/model-name"
	LabelFramework     = "ai.manager.io/framework"
	LabelApp           = "app"
	LabelGPU           = "nvidia.com/gpu.present"
)

// InferenceSpec defines the desired state of Inference
type InferenceSpec struct {
	Model string `json:"model,omitempty"`

	Framework string `json:"framework,omitempty"`

	Replicas int32 `json:"int,omitempty"`

	PodMetadata *EmbeddedObjectMetadata `json:"podMetadata,omitempty"`

	Image string `json:"image,omitempty"`

	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy,omitempty"`

	ImagePullSecrets []v1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Default: "web"
	// +kubebuilder:default:="web"
	PortName string `json:"portName,omitempty"`

	// Defines the resources requests
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// Defines the Pods' affinity scheduling rules if specified.
	Affinity *v1.Affinity `json:"affinity,omitempty"`

	// Defines the Pods' tolerations if specified.
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`

	// Defines on which Nodes the Pods are scheduled.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// EmbeddedObjectMetadata contains a subset of the fields included in k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta
// Only fields which are relevant to embedded resources are included.
type EmbeddedObjectMetadata struct {
	// Name must be unique within a namespace. Is required when creating resources, although
	// some resources may allow a client to request the generation of an appropriate name
	// automatically. Name is primarily intended for creation idempotence and configuration
	// definition.
	// Cannot be updated.
	// More info: http://kubernetes.io/docs/user-guide/identifiers#names
	// +optional
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`

	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) objects. May match selectors of replication controllers
	// and services.
	// More info: http://kubernetes.io/docs/user-guide/labels
	// +optional
	Labels map[string]string `json:"labels,omitempty" protobuf:"bytes,11,rep,name=labels"`

	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: http://kubernetes.io/docs/user-guide/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty" protobuf:"bytes,12,rep,name=annotations"`
}

// InferenceStatus defines the observed state of Inference
type InferenceStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Inference is the Schema for the inferences API
type Inference struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InferenceSpec   `json:"spec,omitempty"`
	Status InferenceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// InferenceList contains a list of Inference
type InferenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Inference `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Inference{}, &InferenceList{})
}
