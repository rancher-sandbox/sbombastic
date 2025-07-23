package messaging

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// HandlerRegistry is a map that associates subjects with their respective handlers.
type HandlerRegistry map[string]Handler

// NatsSubscriber is an implementation of a message subscriber that uses NATS JetStream to receive messages.
type NatsSubscriber struct {
	cons     jetstream.Consumer
	handlers HandlerRegistry
	logger   *slog.Logger
}

// NewNatsSubscriber creates a new NatsSubscriber instance with the provided NATS connection and durable subscription name.
func NewNatsSubscriber(ctx context.Context, nc *nats.Conn, durable string, handlers HandlerRegistry, logger *slog.Logger) (*NatsSubscriber, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	var subjects []string
	for subject := range handlers {
		subjects = append(subjects, subject)
	}

	cons, err := js.CreateOrUpdateConsumer(ctx,
		streamName,
		jetstream.ConsumerConfig{
			FilterSubjects: subjects,
			Durable:        durable,
		})
	if err != nil {
		return nil, fmt.Errorf("failed to create or update consumer: %w", err)
	}

	subscriber := &NatsSubscriber{
		cons:     cons,
		handlers: handlers,
		logger:   logger.With("component", "subscriber"),
	}

	return subscriber, nil
}

// Run starts the subscriber and processes messages in a loop until the context is done.
func (s *NatsSubscriber) Run(ctx context.Context) error {
	consContext, err := s.cons.Consume(
		func(msg jetstream.Msg) {
			s.logger.DebugContext(ctx, "Processing message", "subject", msg.Subject())

			if err := s.processMessage(ctx, msg.Subject(), msg.Data()); err != nil {
				// TODO: impelement error handling
				s.logger.ErrorContext(ctx, "Failed to process message",
					"subject", msg.Subject(),
					"headers", msg.Headers(),
					"error", err,
				)
			}

			if err := msg.Ack(); err != nil {
				s.logger.ErrorContext(ctx, "Failed to ack message",
					"subject", msg.Subject(),
					"error", err,
				)
			}
		},
	)
	if err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	s.logger.InfoContext(ctx, "Subscriber started, waiting for messages...")

	<-ctx.Done()

	s.logger.InfoContext(ctx, "Subscriber shutting down...")

	consContext.Stop()

	return nil
}

// processMessage handles individual message processing.
func (s *NatsSubscriber) processMessage(ctx context.Context, subject string, message []byte) error {
	handler, found := s.handlers[subject]
	if !found {
		return fmt.Errorf("no handler found for subject: %s", subject)
	}

	if err := handler.Handle(ctx, message); err != nil {
		return fmt.Errorf("failed to handle message on subject %s: %w", subject, err)
	}

	return nil
}
