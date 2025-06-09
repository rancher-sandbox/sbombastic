package messaging

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
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

	ns, err := newEmbeddedTestServer(tmpDir)
	require.NoError(t, err)
	defer ns.Shutdown()

	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	publisher, err := NewNatsPublisher(nc, slog.Default())
	require.NoError(t, err)

	err = publisher.CreateStream(t.Context(), jetstream.MemoryStorage)
	require.NoError(t, err, "failed to add stream")

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
	subscriber, err := NewNatsSubscriber(t.Context(), nc, "test-durable", handlers, slog.Default())
	require.NoError(t, err, "failed to create subscriber")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	message := &testMessage{Data: "data"}
	err = publisher.Publish(t.Context(), message)
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

			subscriber := &NatsSubscriber{
				handlers: handlers,
				logger:   slog.Default(),
			}

			err := subscriber.processMessage(test.msg.Header.Get(MessageTypeHeader), test.msg.Data)

			if test.expectedError == "" {
				require.NoError(t, err, "expected no error, got: %v", err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.expectedError, "expected error message does not match")
			}
		})
	}
}
