package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

type testHandler struct {
	handleFunc func(Message) error
}

func (h *testHandler) Handle(message Message) error {
	return h.handleFunc(message)
}

func (h *testHandler) NewMessage() Message {
	return &testMessage{}
}

func TestSubscriber_Run(t *testing.T) {
	tmpDir := t.TempDir()

	ns, err := NewServer(tmpDir)
	require.NoError(t, err)
	defer ns.Shutdown()

	js, err := NewJetStreamContext(ns)
	require.NoError(t, err)

	err = AddStream(js, nats.MemoryStorage)
	require.NoError(t, err)

	sub, err := NewSubscription(ns.ClientURL(), "test-sub")
	require.NoError(t, err)

	message := &testMessage{Data: "data"}
	header := nats.Header{MessageTypeHeader: {"test-type"}}

	processed := make(chan Message, 1)
	done := make(chan struct{})

	handleFunc := func(m Message) error {
		processed <- m
		return nil
	}

	testHandler := &testHandler{handleFunc: handleFunc}
	handlers := HandlerRegistry{
		"test-type": testHandler,
	}
	subscriber := NewSubscriber(sub, handlers, slog.Default())

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	data, err := json.Marshal(message)
	require.NoError(t, err, "failed to marshal testMessage data")

	msg := &nats.Msg{
		Subject: sbombasticSubject,
		Data:    data,
		Header:  header,
	}

	_, err = js.PublishMsg(msg)
	require.NoError(t, err, "failed to publish message")

	go func() {
		err = subscriber.Run(ctx)
		close(done)
	}()

	select {
	case processedMessage := <-processed:
		require.Equal(t, message, processedMessage, "unexpected message")
	case <-time.After(2 * time.Second):
		require.Fail(t, "timed out waiting for message to be processed")
	}

	cancel()
	<-done

	require.NoError(t, err, "unexpected subscriber error")
}

func TestProcessMessage(t *testing.T) {
	tests := []struct {
		name          string
		msg           *nats.Msg
		handleFunc    func(Message) error
		expectedError string
	}{
		{
			name: "valid message",
			msg: &nats.Msg{
				Subject: sbombasticSubject,
				Data:    []byte(`{"data":"valid"}`),
				Header:  nats.Header{MessageTypeHeader: {"test-type"}},
			},
			handleFunc: func(_ Message) error {
				return nil
			},
			expectedError: "",
		},
		{
			name: "missing type header",
			msg: &nats.Msg{
				Subject: sbombasticSubject,
				Data:    []byte(`{"data":"valid"}`),
				Header:  nats.Header{},
			},
			expectedError: "malformed message: missing type header",
		},
		{
			name: "unknown message type",
			msg: &nats.Msg{
				Subject: sbombasticSubject,
				Data:    []byte(`{"data":"valid"}`),
				Header:  nats.Header{MessageTypeHeader: {"unknown-type"}},
			},
			expectedError: "no handler found for message type: unknown-type",
		},
		{
			name: "invalid message format",
			msg: &nats.Msg{
				Subject: sbombasticSubject,
				Data:    []byte(`{"invalid-json"`),
				Header:  nats.Header{MessageTypeHeader: {"test-type"}},
			},
			expectedError: "failed to unmarshal message of type test-type",
		},
		{
			name: "handler failure",
			msg: &nats.Msg{
				Subject: sbombasticSubject,
				Data:    []byte(`{"data":"valid"}`),
				Header:  nats.Header{MessageTypeHeader: {"test-type"}},
			},
			handleFunc: func(_ Message) error {
				return errors.New("handler error")
			},
			expectedError: "failed to handle message of type test-type: handler error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handlers := HandlerRegistry{}
			handlers["test-type"] = &testHandler{
				handleFunc: test.handleFunc,
			}

			subscriber := &Subscriber{
				handlers: handlers,
				logger:   slog.Default(),
			}

			err := subscriber.processMessage(test.msg)

			if test.expectedError == "" {
				require.NoError(t, err, "expected no error, got: %v", err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.expectedError, "expected error message does not match")
			}
		})
	}
}
