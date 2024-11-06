package messaging

type Handler interface {
	Handle(msg Message) error
	NewMessage() Message
}
