package messaging

import "context"

type Handler interface {
	Handle(ctx context.Context, message []byte) error
}

type FailureHandler interface {
	HandleFailure(ctx context.Context, message []byte, errorMessage string) error
}
