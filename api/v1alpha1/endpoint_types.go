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
	EndpointFinalizer                              = "finalizer.endpoint.ai.manager.io"
	DeployedEndpointStatus EndpointComponentStatus = "deployed"
	FailedEndpointStatus   EndpointComponentStatus = "failed"
	LabelChatWebUI                                 = "chat-next-web"
	ChatWebName                                    = "chatNextWeb"
	DefaultChatWebImage                            = "yidadaa/chatgpt-next-web"
	DefaultChatWebImageTag                         = "latest"
	WebContainerPort       int32                   = 3000
)

type EndpointComponentStatus string

// EndpointSpec defines the desired state of Endpoint
type EndpointSpec struct {
	InferSpec *InferenceSpec   `json:"inferSpec,omitempty"`
	WebSpec   *EndpointWebSpec `json:"webSpec,omitempty"`
}

// EndpointStatus defines the observed state of Endpoint
type EndpointStatus struct {
	WebStatus   EndpointComponentStatus `json:"webStatus,omitempty"`
	InferStatus EndpointComponentStatus `json:"InferStatus,omitempty"`
}

type EndpointWebSpec struct {
	OpenAiKey string `json:"openAiKey,omitempty"`

	BaseURL string `json:"baseURL,omitempty"`

	Password string `json:"password,omitempty"`

	Replicas int32 `json:"int,omitempty"`

	PodMetadata *EmbeddedObjectMetadata `json:"podMetadata,omitempty"`

	Image string `json:"image,omitempty"`

	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy,omitempty"`

	ImagePullSecrets []v1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Default: "chatweb"
	// +kubebuilder:default:="chatweb"
	PortName string `json:"portName,omitempty"`

	ServicePort int32 `json:"servicePort,omitempty"`

	// Defines the resources requests
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// Defines the Pods' affinity scheduling rules if specified.
	Affinity *v1.Affinity `json:"affinity,omitempty"`

	// Defines the Pods' tolerations if specified.
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`

	// Defines on which Nodes the Pods are scheduled.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Endpoint is the Schema for the endpoints API
type Endpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointSpec   `json:"spec,omitempty"`
	Status EndpointStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EndpointList contains a list of Endpoint
type EndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Endpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Endpoint{}, &EndpointList{})
}
