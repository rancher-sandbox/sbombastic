/*
Copyright 2024.

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

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImageList contains a list of Image
type ImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Image `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:selectablefield:JSONPath=`.spec.imageMetadata.registry`
// +kubebuilder:selectablefield:JSONPath=`.spec.imageMetadata.registryURI`
// +kubebuilder:selectablefield:JSONPath=`.spec.imageMetadata.repository`
// +kubebuilder:selectablefield:JSONPath=`.spec.imageMetadata.tag`
// +kubebuilder:selectablefield:JSONPath=`.spec.imageMetadata.platform`
// +kubebuilder:selectablefield:JSONPath=`.spec.imageMetadata.digest`

// Image is the Schema for the images API
type Image struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageSpec   `json:"spec,omitempty"`
	Status ImageStatus `json:"status,omitempty"`
}

// ImageSpec defines the desired state of Image
type ImageSpec struct {
	// Metadata of the image
	ImageMetadata `json:"imageMetadata"`
	// List of the layers that make the image
	Layers []ImageLayer `json:"layers,omitempty"`
}

// ImageLayer define a layer part of an OCI Image
type ImageLayer struct {
	// command is the command that led to the creation
	// of the layer. The contents are base64 encoded
	Command string `json:"command"`
	// digest is the Hash of the compressed layer
	Digest string `json:"digest"`
	// diffID is the Hash of the uncompressed layer
	DiffID string `json:"diffID"`
}

// ImageStatus defines the observed state of Image
type ImageStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

func (i *Image) GetImageMetadata() ImageMetadata {
	return i.Spec.ImageMetadata
}
