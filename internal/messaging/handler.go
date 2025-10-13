package messaging

import "context"

// Message represents a message received by the handler.
// It provides access to the message data and allows marking the message as in-progress
// to extend the acknowledgement timeout during long-running operations.
type Message interface {
	Data() []byte
	InProgress() error
}

// Handler processes messages.
type Handler interface {
	Handle(ctx context.Context, message Message) error
}

// FailureHandler handles messages that failed processing after exhausting retries.
type FailureHandler interface {
	HandleFailure(ctx context.Context, message Message, errorMessage string) error
}
