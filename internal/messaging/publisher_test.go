package messaging

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testMessage struct {
	Data string `json:"data"`
}

func (m testMessage) MessageType() string {
	return "test-type"
}

func TestPublisher_Publish(t *testing.T) {
	ns, err := NewServer()
	require.NoError(t, err)
	defer ns.Shutdown()

	js, err := NewJetStreamContext(ns)
	require.NoError(t, err)

	publisher := NewPublisher(js)

	msg := testMessage{
		Data: "test data",
	}

	err = publisher.Publish(msg)
	require.NoError(t, err)

	sub, err := js.SubscribeSync(SbombasticSubject)
	require.NoError(t, err)
	defer func() {
		err := sub.Unsubscribe()
		require.NoError(t, err)
	}()

	receivedMsg, err := sub.NextMsg(2 * time.Second)
	require.NoError(t, err)

	assert.Equal(t, msg.MessageType(), receivedMsg.Header.Get(MessageTypeHeader))

	var receivedData testMessage
	err = json.Unmarshal(receivedMsg.Data, &receivedData)
	require.NoError(t, err)
	assert.Equal(t, msg.Data, receivedData.Data)
}
