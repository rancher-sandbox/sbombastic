package handlers

const (
	GenerateSBOMSubject  = "sbombastic.sbom.generate"
	ScanSBOMSubject      = "sbombastic.sbom.scan"
	CreateCatalogSubject = "sbombastic.catalog.create"
)

// ObjectRef is a reference to a Kubernetes object, used in messages to identify resources.
type ObjectRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// BaseMessage is the base structure for messages.
type BaseMessage struct {
	ScanJob ObjectRef `json:"scanjob"`
}

// CreateCatalogMessage represents a request to create a catalog of images in a registry.
type CreateCatalogMessage struct {
	BaseMessage
}

// GenerateSBOMMessage represents the request message for generating a SBOM.
type GenerateSBOMMessage struct {
	BaseMessage
	Image ObjectRef `json:"image"`
}

// ScanSBOMMessage represents the request message for scanning a SBOM.
type ScanSBOMMessage struct {
	BaseMessage
	SBOM ObjectRef `json:"sbom"`
}
