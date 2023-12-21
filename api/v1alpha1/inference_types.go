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
	InferenceFinalizer                        = "finalizer.inference.ai.manager.io"
	LabelModelName                            = "ai.manager.io/model-name"
	LabelFramework                            = "ai.manager.io/framework"
	LabelApp                                  = "app"
	LabelGPU                                  = "nvidia.com/gpu.present"
	DeployedInferStatus  InferComponentStatus = "deployed"
	FailedInferStatus    InferComponentStatus = "failed"
	LocalModelDir                             = "/models"
	InferContainerPort   int32                = 8000
	InferServicePort     int32                = 8000
	DefaultModelImageTag string               = "rebase"
)

type InferComponentStatus string

// InferenceSpec defines the desired state of Inference
type InferenceSpec struct {
	// +required
	Model string `json:"model"`
	// +required
	Framework string `json:"framework"`

	// Default: 1
	// +kubebuilder:default:=1
	Replicas *int32 `json:"replicas,omitempty"`

	// +optional
	PodMetadata *EmbeddedObjectMetadata `json:"podMetadata,omitempty"`

	// +optional
	Image *string `json:"image,omitempty"`

	// +kubebuilder:validation:Enum="";Always;Never;IfNotPresent
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy,omitempty"`

	ImagePullSecrets []v1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// +optional
	// +kubebuilder:default:="inference"
	PortName string `json:"portName,omitempty"`

	// +optional
	NodePort *int32 `json:"nodePort,omitempty"`

	// Defines the resources requests
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// Defines the Pods' affinity scheduling rules if specified.
	// +optional
	Affinity *v1.Affinity `json:"affinity,omitempty"`

	// Defines the Pods' tolerations if specified.
	// +optional
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`

	// Defines on which Nodes the Pods are scheduled.
	// +optional
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
	Hash string `json:"hash,omitempty"`

	Status InferComponentStatus `json:"status,omitempty"`
	// lastTransitionTime is the time of the last update to the current status property.
	// +required
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
	// Human-readable message indicating details for the last update.
	// +optional
	Message string `json:"message,omitempty"`
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
