package v1alpha1

type ImageMetadata struct {
	Registry   string `json:"registry"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Platform   string `json:"platform"`
	Digest     string `json:"digest"`
}

type ImageMetadataAccessor interface {
	GetImageMetadata() ImageMetadata
}
