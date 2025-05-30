package messaging

import (
	"fmt"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

func newEmbeddedTestServer(storeDir string) (*server.Server, error) {
	opts := &server.Options{
		JetStream: true,
		StoreDir:  storeDir,
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
