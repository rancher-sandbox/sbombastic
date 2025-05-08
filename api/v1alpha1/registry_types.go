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
	RegistryJobDiscoveryType = "discovery"
	RegistryJobScanType      = "scan"
)

const (
	RegistryLastJobTypeAnnotation = "sbombastic.rancher.io/last-job-type"
	RegistryLastJobNameAnnotation = "sbombastic.rancher.io/last-job-name"
)
const (
	RegistryLastDiscoveryStartAtAnnotation    = "sbombastic.rancher.io/last-discovery-started-at"
	RegistryLastDiscoveryCompleteAtAnnotation = "sbombastic.rancher.io/last-discovery-completed-at"
	RegistryLastDiscoveryCompletedAnnotation  = "sbombastic.rancher.io/last-discovery-completed"
	RegistryLastDiscoveredImageNameAnnotation = "sbombastic.rancher.io/last-discovered-image-name"

	RegistryLastScannedAtAnnotation     = "sbombastic.rancher.io/last-scanned-at"
	RegistryLastScanCompletedAnnotation = "sbombastic.rancher.io/registry-last-scan-completed"
)

type NumericCron struct {
	// 0~6: Sunday to Saturday
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=6
	DayOfWeek *int8 `json:"dayOfWeek,omitempty"`
	// 1~12
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=12
	Month *int8 `json:"month,omitempty"`
	// 1~31
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=31
	DayOfMonth *int8 `json:"dayOfMonth,omitempty"`
	// 0~23
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=23
	Hour *int8 `json:"hour,omitempty"`
}

type DiscoveryJob struct {
	// cron in numeric format. see https://en.wikipedia.org/wiki/Cron
	// +kubebuilder:validation:Required
	Cron NumericCron `json:"cron"`
	// Suspend scheduled discovery
	Suspend bool `json:"suspend"`
	// number of RegistryDiscovery objects with failed state to keep
	// +default:value=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=2
	FailedJobsHistoryLimit uint8 `json:"failedJobsHistoryLimit"`
	// number of RegistryDiscovery objects with successful state to keep
	// +default:value=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=2
	SuccessfulJobsHistoryLimit uint8 `json:"successfulJobsHistoryLimit"`
}

// RegistrySpec defines the desired state of Registry.
type RegistrySpec struct {
	// URI is the URI of the container registry
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	URI string `json:"uri"`
	// DiscoveryJob is the configurations for scheduled discovery
	// +kubebuilder:validation:Required
	DiscoveryJob DiscoveryJob `json:"discoveryJob"`
	// Repositories is the list of the repositories to be scanned
	// An empty list means all the repositories found in the registry are going to be scanned
	Repositories []string `json:"repositories,omitempty"`
	// AuthSecret is the name of the secret in the same namespace that contains the credentials to access the registry.
	AuthSecret string `json:"authSecret,omitempty"`
	// CABundle is the CA bundle to use when connecting to the registry.
	CABundle string `json:"caBundle,omitempty"`
	// Insecure allows insecure connections to the registry when set to true.
	Insecure bool `json:"insecure,omitempty"`
}

// RegistryStatus defines the observed state of Registry.
type RegistryStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Registry is the Schema for the registries API.
type Registry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RegistrySpec   `json:"spec,omitempty"`
	Status RegistryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RegistryList contains a list of Registry.
type RegistryList struct {
	metav1.TypeMeta `           json:",inline"`
	metav1.ListMeta `           json:"metadata,omitempty"`
	Items           []Registry `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Registry{}, &RegistryList{})
}
