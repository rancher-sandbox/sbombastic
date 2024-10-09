package messaging

import (
	"fmt"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

const SbombasticSubject = "sbombastic"

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

	_, err = js.AddStream(&nats.StreamConfig{
		Name: "SBOMBASTIC",
		// We use WorkQueuePolicy to ensure that each message is removed once it is processed.
		Retention: nats.WorkQueuePolicy,
		Subjects:  []string{SbombasticSubject},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add JetStream stream: %w", err)
	}

	return js, nil
}
