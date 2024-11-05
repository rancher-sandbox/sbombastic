package messaging

type testMessage struct {
	Data string `json:"data"`
}

func (m testMessage) MessageType() string {
	return "test-type"
}
