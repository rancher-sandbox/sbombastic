package messaging

type Message interface {
	MessageType() string
}

const (
	CreateCatalogType = "CreateCatalog"
	GenerateSBOMType  = "GenerateSBOM"
	ScanSBOMType      = "ScanSBOM"
)

type CreateCatalog struct {
	RegistryName      string `json:"registryName"`
	RegistryNamespace string `json:"registryNamespace"`
}

func (m *CreateCatalog) MessageType() string {
	return CreateCatalogType
}

type GenerateSBOM struct {
	ImageName      string `json:"imageName"`
	ImageNamespace string `json:"imageNamespace"`
}

func (m *GenerateSBOM) MessageType() string {
	return GenerateSBOMType
}

type ScanSBOM struct {
	SBOMName      string `json:"sbomName"`
	SBOMNamespace string `json:"sbomNamespace"`
}

func (m *ScanSBOM) MessageType() string {
	return ScanSBOMType
}
