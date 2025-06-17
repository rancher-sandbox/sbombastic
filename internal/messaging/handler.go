package messaging

type Handler interface {
	Handle(msg []byte) error
}
