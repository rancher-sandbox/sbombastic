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
