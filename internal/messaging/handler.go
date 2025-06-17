package messaging

import "context"

type Handler interface {
	Handle(ctx context.Context, message []byte) error
}
