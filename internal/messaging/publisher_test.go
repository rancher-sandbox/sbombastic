package messaging

import (
	"log/slog"
	"testing"

	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPublisherSubject = "sbomscanner.publisher.test"

func TestPublisher_Publish(t *testing.T) {
	opts := natstest.DefaultTestOptions
	opts.Port = -1 // Use a random port
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	ns := natstest.RunServer(&opts)
	defer ns.Shutdown()

	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)

	publisher, err := NewNatsPublisher(t.Context(), nc, slog.Default())
	require.NoError(t, err)

	message := []byte(`{"data":"test data"}`)
	err = publisher.Publish(t.Context(), testPublisherSubject, "id", message)
	require.NoError(t, err)

	// Send a duplicate message with the same ID to test idempotency
	messageDup := []byte(`{"data":"test data duplicate"}`)
	err = publisher.Publish(t.Context(), testPublisherSubject, "id", messageDup)
	require.NoError(t, err)

	cons, err := publisher.js.CreateOrUpdateConsumer(t.Context(), streamName, jetstream.ConsumerConfig{})
	require.NoError(t, err)

	batch, err := cons.FetchNoWait(10) // Fetch max 10 messages, but we expect only 1
	require.NoError(t, err)
	require.NoError(t, batch.Error())

	var messages []jetstream.Msg
	for msg := range batch.Messages() {
		messages = append(messages, msg)
	}
	require.Len(t, messages, 1)

	receivedMessage := messages[0]
	assert.Equal(t, testPublisherSubject, receivedMessage.Subject())
	assert.Equal(t, message, receivedMessage.Data())
}
