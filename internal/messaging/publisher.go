package messaging

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	streamName        = "SBOMBASTIC"
	sbombasticSubject = "sbombastic.>"
)

type Publisher interface {
	// Publish publishes a message.
	Publish(ctx context.Context, subject string, message []byte) error
}

// NatsPublisher is an implementation of the Publisher interface that uses NATS JetStream to publish messages.
type NatsPublisher struct {
	js     jetstream.JetStream
	logger *slog.Logger
}

// NewNatsPublisher creates a new NatsPublisher instance with the provided NATS connection.
func NewNatsPublisher(nc *nats.Conn, logger *slog.Logger) (*NatsPublisher, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	publisher := &NatsPublisher{
		js:     js,
		logger: logger.With("component", "publisher"),
	}

	return publisher, nil
}

// Publish publishes a message.
func (p *NatsPublisher) Publish(ctx context.Context, subject string, message []byte) error {
	msg := &nats.Msg{
		Subject: subject,
		Data:    message,
	}
	if _, err := p.js.PublishMsg(ctx, msg); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	p.logger.DebugContext(ctx, "Message published", "subject", msg.Subject, "header", msg.Header, "message", string(msg.Data))

	return nil
}

// CreateStream adds a stream to the NATS JetStream context with the specified storage type.
func (p *NatsPublisher) CreateStream(ctx context.Context, storage jetstream.StorageType) error {
	// CreateStream is an idempotent operation, if the stream already exists, it will succeed without error.
	_, err := p.js.CreateStream(ctx, jetstream.StreamConfig{
		Name:      streamName,
		Retention: jetstream.WorkQueuePolicy,
		Subjects:  []string{sbombasticSubject},
		Storage:   storage,
	})
	if err != nil {
		return fmt.Errorf("failed to create JetStream stream: %w", err)
	}

	p.logger.DebugContext(ctx, "Stream created", "subjects", sbombasticSubject, "storage", storage)

	return nil
}
