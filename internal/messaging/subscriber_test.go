package messaging

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

const testSubscriberSubject = "sbombastic.subscriber.test"

type testHandler struct {
	handleFunc func(message []byte) error
}

func (h *testHandler) Handle(_ context.Context, message []byte) error {
	return h.handleFunc(message)
}

func TestSubscriber_Run(t *testing.T) {
	opts := natstest.DefaultTestOptions
	opts.Port = -1 // Use a random port
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	ns := natstest.RunServer(&opts)
	defer ns.Shutdown()

	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	publisher, err := NewNatsPublisher(nc, slog.Default())
	require.NoError(t, err)

	err = publisher.CreateStream(t.Context(), jetstream.MemoryStorage)
	require.NoError(t, err, "failed to add stream")

	processed := make(chan []byte, 1)
	done := make(chan struct{})

	handleFunc := func(m []byte) error {
		processed <- m
		return nil
	}

	testHandler := &testHandler{handleFunc: handleFunc}
	handlers := HandlerRegistry{
		testSubscriberSubject: testHandler,
	}
	subscriber, err := NewNatsSubscriber(t.Context(), nc, "test-durable", handlers, slog.Default())
	require.NoError(t, err, "failed to create subscriber")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	message := []byte(`{"data":"test data"}`)
	err = publisher.Publish(t.Context(), testSubscriberSubject, message)
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
		handleFunc    func([]byte) error
		expectedError string
	}{
		{
			name: "valid message",
			msg: &nats.Msg{
				Subject: testSubscriberSubject,
				Data:    []byte("data"),
			},
			handleFunc: func(_ []byte) error {
				return nil
			},
			expectedError: "",
		},
		{
			name: "unregistered subject",
			msg: &nats.Msg{
				Subject: "unknown",
				Data:    []byte("data"),
			},
			expectedError: "no handler found for subject: unknown",
		},
		{
			name: "handler failure",
			msg: &nats.Msg{
				Subject: testSubscriberSubject,
				Data:    []byte("data"),
			},
			handleFunc: func(_ []byte) error {
				return errors.New("handler error")
			},
			expectedError: "failed to handle message on subject sbombastic.subscriber.test: handler error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handlers := HandlerRegistry{}
			handlers[testSubscriberSubject] = &testHandler{
				handleFunc: test.handleFunc,
			}

			subscriber := &NatsSubscriber{
				handlers: handlers,
				logger:   slog.Default(),
			}

			err := subscriber.processMessage(t.Context(), test.msg.Subject, test.msg.Data)

			if test.expectedError == "" {
				require.NoError(t, err, "expected no error, got: %v", err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.expectedError, "expected error message does not match")
			}
		})
	}
}
