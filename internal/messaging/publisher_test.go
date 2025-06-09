package messaging

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublisher_Publish(t *testing.T) {
	tmpDir := t.TempDir()
	ns, err := newEmbeddedTestServer(tmpDir)
	require.NoError(t, err)
	defer ns.Shutdown()

	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)

	publisher, err := NewNatsPublisher(nc, slog.Default())
	require.NoError(t, err)

	err = publisher.CreateStream(t.Context(), jetstream.MemoryStorage)
	require.NoError(t, err)

	msg := testMessage{
		Data: "test data",
	}

	err = publisher.Publish(t.Context(), msg)
	require.NoError(t, err)

	cons, err := publisher.js.CreateOrUpdateConsumer(t.Context(), streamName, jetstream.ConsumerConfig{})
	require.NoError(t, err)

	batch, err := cons.Fetch(1)
	require.NoError(t, err)
	require.NoError(t, batch.Error())

	var messages []jetstream.Msg
	for msg := range batch.Messages() {
		messages = append(messages, msg)
	}
	require.Len(t, messages, 1)

	receivedMsg := messages[0]
	assert.Equal(t, msg.MessageType(), receivedMsg.Headers().Get(MessageTypeHeader))

	var receivedData testMessage
	err = json.Unmarshal(receivedMsg.Data(), &receivedData)
	require.NoError(t, err)
	assert.Equal(t, msg.Data, receivedData.Data)
}
