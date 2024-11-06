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

const CreateSBOMType = "CreateSBOM"

type CreateSBOM struct {
	ImageName      string `json:"imageName"`
	ImageNamespace string `json:"imageNamespace"`
}

func (m *CreateSBOM) MessageType() string {
	return CreateSBOMType
}
