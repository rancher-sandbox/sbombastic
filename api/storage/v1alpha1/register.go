/*
Copyright 2017 The Kubernetes Authors.

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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupName is the group name used in this package
const GroupName = "storage.sbombastic.rancher.io"

// SchemeGroupVersion is group version used to register these objects
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}

// Kind takes an unqualified kind and returns back a Group qualified GroupKind
func Kind(kind string) schema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns back a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	// SchemeBuilder is the scheme builder with scheme init functions to run for this API package
	SchemeBuilder = runtime.NewSchemeBuilder(AddKnownTypes)
	// AddToScheme is a common registration function for mapping packaged scoped group & version keys to a scheme
	AddToScheme = SchemeBuilder.AddToScheme
)

// AddKnownTypes adds the list of known types to the given scheme.
func AddKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Image{},
		&ImageList{},

		&SBOM{},
		&SBOMList{},

		&VulnerabilityReport{},
		&VulnerabilityReportList{},

		&metav1.GetOptions{},
		&metav1.CreateOptions{},
		&metav1.UpdateOptions{},
		&metav1.DeleteOptions{},
		&metav1.ListOptions{},
	)

	err := scheme.AddFieldLabelConversionFunc(
		SchemeGroupVersion.WithKind("Image"),
		imageMetadataFieldSelectorConversion,
	)
	if err != nil {
		return fmt.Errorf("unable to add field selector conversion function to Image: %w", err)
	}

	err = scheme.AddFieldLabelConversionFunc(SchemeGroupVersion.WithKind("SBOM"), imageMetadataFieldSelectorConversion)
	if err != nil {
		return fmt.Errorf("unable to add field selector conversion function to SBOM: %w", err)
	}

	err = scheme.AddFieldLabelConversionFunc(
		SchemeGroupVersion.WithKind("VulnerabilityReport"),
		imageMetadataFieldSelectorConversion,
	)
	if err != nil {
		return fmt.Errorf("unable to add field selector conversion function to VulnerabilityReport: %w", err)
	}
	return nil
}

func imageMetadataFieldSelectorConversion(label, value string) (string, string, error) {
	switch label {
	case "metadata.name":
		return label, value, nil
	case "metadata.namespace":
		return label, value, nil
	case "imageMetadata.registry":
		return label, value, nil
	case "imageMetadata.registryURI":
		return label, value, nil
	case "imageMetadata.repository":
		return label, value, nil
	case "imageMetadata.tag":
		return label, value, nil
	case "imageMetadata.platform":
		return label, value, nil
	case "imageMetadata.digest":
		return label, value, nil
	default:
		return "", "", fmt.Errorf(
			"%q is not a known field selector: only %q, %q, %q",
			label,
			"metadata.name",
			"metadata.namespace",
			"imageMetadata.*",
		)
	}
}
