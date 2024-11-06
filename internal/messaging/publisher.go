package messaging

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

const MessageTypeHeader = "MessageType"

//go:generate go run github.com/vektra/mockery/v2@v2.46.2 --name Publisher
type Publisher interface {
	Publish(message Message) error
}

type publisher struct {
	js nats.JetStreamContext
}

func NewPublisher(js nats.JetStreamContext) Publisher {
	return &publisher{
		js: js,
	}
}

func (p *publisher) Publish(message Message) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message to JSON: %w", err)
	}

	header := make(nats.Header)
	header.Add(MessageTypeHeader, message.MessageType())

	msg := &nats.Msg{
		Subject: sbombasticSubject,
		Data:    data,
		Header:  header,
	}

	if _, err := p.js.PublishMsg(msg); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}
