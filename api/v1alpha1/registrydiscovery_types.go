/*
Copyright 2025.

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

const (
	RegistryDiscoveringCondition = "Discovering"
	RegistryDiscoveredCondition  = "Discovered"
)

const (
	DiscoveryStatusPending     = "Pending"
	DiscoveryStatusRunning     = "Running"
	DiscoveryStatusFailStopped = "FailStopped"
	DiscoveryStatusCanceled    = "Cancel"
	DiscoveryStatusSucceeded   = "Succeeded"
)

const (
	RegistryFailedToRequestDiscoveryReason = "FailedToRequestDiscovery"
	RegistryDiscoveryPendingReason         = "DiscoveryPending"
	RegistryDiscoveryRunningReason         = "DiscoveryRunning"
	RegistryDiscoveryFailedReason          = "DiscoveryFailed"
	RegistryDiscoveryFinishedReason        = "DiscoveryFinished"
)

// RegistryDiscoverySpec defines the desired state of RegistryDiscovery.
type RegistryDiscoverySpec struct {
	// registry name in the same namespace
	// +kubebuilder:validation:Required
	Registry string `json:"registry"`

	// registry spec
	RegistrySpec RegistrySpec `json:"registrySpec"`
}

// RegistryDiscoveryStatus defines the observed state of RegistryDiscovery.
type RegistryDiscoveryStatus struct {
	// current status of the registry discovery
	// +kubebuilder:validation:Optional
	CurrentStatus string `json:"currentStatus"`
	// registry discovery is canceled
	// +kubebuilder:validation:Optional
	Canceled bool `json:"canceled"`
	// timestamp of the start handling time
	// +kubebuilder:validation:Optional
	StartedAt string `json:"startedAt"`
	// timestamp of the stop handling time
	// +kubebuilder:validation:Optional
	StoppedAt string `json:"finishedAt"`

	// Represents the observations of a Registry's current state.
	// Registry.status.conditions.type are: "Discovering", "Scanning", and "UpToDate"
	// Registry.status.conditions.status are one of True, False, Unknown.
	// Registry.status.conditions.reason the value should be a CamelCase string and producers of specific
	// condition types may define expected values and meanings for this field, and whether the values
	// are considered a guaranteed API.
	// Registry.status.conditions.Message is a human readable message indicating details about the transition.
	// For further information see: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RegistryDiscovery is the Schema for the registrydiscoveries API.
type RegistryDiscovery struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RegistryDiscoverySpec   `json:"spec,omitempty"`
	Status RegistryDiscoveryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RegistryDiscoveryList contains a list of RegistryDiscovery.
type RegistryDiscoveryList struct {
	metav1.TypeMeta `                    json:",inline"`
	metav1.ListMeta `                    json:"metadata,omitempty"`
	Items           []RegistryDiscovery `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RegistryDiscovery{}, &RegistryDiscoveryList{})
}
