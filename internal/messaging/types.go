package messaging

type Message interface {
	MessageType() string
}

type CreateCatalog struct {
	RegistryName      string `json:"registryName"`
	RegistryNamespace string `json:"registryNamespace"`
}

func (m *CreateCatalog) MessageType() string {
	return "CreateCatalog"
}

type CreateSBOM struct {
	ImageName      string `json:"imageName"`
	ImageNamespace string `json:"imageNamespace"`
}

func (m *CreateSBOM) MessageType() string {
	return "CreateSBOM"
}
