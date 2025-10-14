package v1alpha1

// IndexImageMetadataRegistry is the field index for the registry of an image.
const IndexImageMetadataRegistry = "imageMetadata.registry"

// ImageMetadata contains the metadata details of an image.
type ImageMetadata struct {
	// Registry specifies the name of the Registry object in the same namespace where the image is stored.
	Registry string `json:"registry"`
	// RegistryURI specifies the URI of the registry where the image is stored. Example: "registry-1.docker.io:5000".`
	RegistryURI string `json:"registryURI"`
	// Repository specifies the repository path of the image. Example: "kubewarden/sbomscanner".
	Repository string `json:"repository"`
	// Tag specifies the tag of the image. Example: "latest".
	Tag string `json:"tag"`
	// Platform specifies the platform of the image. Example "linux/amd64".
	Platform string `json:"platform"`
	// Digest specifies the sha256 digest of the image.
	Digest string `json:"digest"`
}

type ImageMetadataAccessor interface {
	GetImageMetadata() ImageMetadata
}
