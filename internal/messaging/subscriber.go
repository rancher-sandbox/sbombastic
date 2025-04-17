package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

type HandlerRegistry map[string]Handler

type Subscriber struct {
	sub      *nats.Subscription
	handlers HandlerRegistry
	logger   *slog.Logger
}

func NewSubscriber(sub *nats.Subscription, handlers HandlerRegistry, logger *slog.Logger) *Subscriber {
	return &Subscriber{
		sub:      sub,
		handlers: handlers,
		logger:   logger.With("component", "subscriber"),
	}
}

func (s *Subscriber) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			s.logger.InfoContext(ctx, "Subscriber shutting down...")

			return nil
		default:
			msgs, err := s.sub.Fetch(1, nats.MaxWait(5*time.Second))
			if err != nil {
				if errors.Is(err, nats.ErrTimeout) {
					continue
				}

				return fmt.Errorf("failed to fetch message: %w", err)
			}

			for _, msg := range msgs {
				s.logger.DebugContext(ctx, "Processing message", "message", msg)
				if processMsgErr := s.processMessage(msg); processMsgErr != nil {
					s.logger.ErrorContext(ctx, "Failed to process message",
						"subject", msg.Subject,
						"header", msg.Header,
						"data", msg.Data,
						"error", processMsgErr,
					)
				}

				if ackErr := msg.Ack(); ackErr != nil {
					return fmt.Errorf("failed to ack message: %w", ackErr)
				}
			}
		}
	}
}

// processMessage handles individual message processing.
func (s *Subscriber) processMessage(msg *nats.Msg) error {
	msgType := msg.Header.Get(MessageTypeHeader)
	if msgType == "" {
		return fmt.Errorf("malformed message: missing type header, header: %v", msg.Header)
	}

	handler, found := s.handlers[msgType]
	if !found {
		return fmt.Errorf("no handler found for message type: %s", msgType)
	}

	message := handler.NewMessage()
	if err := json.Unmarshal(msg.Data, message); err != nil {
		return fmt.Errorf("failed to unmarshal message of type %s: %w", msgType, err)
	}

	if err := handler.Handle(message); err != nil {
		return fmt.Errorf("failed to handle message of type %s: %w", msgType, err)
	}

	return nil
}
