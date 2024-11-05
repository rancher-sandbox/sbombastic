package messaging

import (
	"fmt"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

const sbombasticSubject = "sbombastic"

func NewServer() (*server.Server, error) {
	opts := &server.Options{
		JetStream: true,
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create NATS server: %w", err)
	}
	ns.ConfigureLogger()

	go ns.Start()

	if !ns.ReadyForConnections(20 * time.Second) {
		return nil, fmt.Errorf("NATS server not ready for connections: %w", err)
	}

	return ns, nil
}

func NewJetStreamContext(ns *server.Server) (nats.JetStreamContext, error) {
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS server: %w", err)
	}

	js, err := nc.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	return js, nil
}

func AddStream(js nats.JetStreamContext, storage nats.StorageType) error {
	_, err := js.AddStream(&nats.StreamConfig{
		Name: "SBOMBASTIC",
		// We use WorkQueuePolicy to ensure that each message is removed once it is processed.
		Retention: nats.WorkQueuePolicy,
		Subjects:  []string{sbombasticSubject},
		Storage:   storage,
	})
	if err != nil {
		return fmt.Errorf("failed to add JetStream stream: %w", err)
	}

	return nil
}

func NewSubscription(url, durable string) (*nats.Subscription, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS server: %w", err)
	}

	js, err := nc.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	sub, err := js.PullSubscribe(sbombasticSubject, durable, nats.InactiveThreshold(24*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to JetStream stream: %w", err)
	}

	return sub, nil
}
