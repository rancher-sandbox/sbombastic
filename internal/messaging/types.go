package messaging

type Message interface {
	MessageType() string
}

const CreateCatalogType = "CreateCatalog"

type CreateCatalog struct {
	RegistryName      string `json:"registryName"`
	RegistryNamespace string `json:"registryNamespace"`
}

func (m *CreateCatalog) MessageType() string {
	return CreateCatalogType
}

const GenerateSBOMType = "GenerateSBOM"

type GenerateSBOM struct {
	ImageName      string `json:"imageName"`
	ImageNamespace string `json:"imageNamespace"`
}

func (m *GenerateSBOM) MessageType() string {
	return GenerateSBOMType
}
