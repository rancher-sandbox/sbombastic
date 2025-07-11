package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

const testSubscriberSubject = "sbombastic.subscriber.test"

type testHandler struct {
	handleFunc func(message []byte) error
}

func (h *testHandler) Handle(_ context.Context, message []byte) error {
	return h.handleFunc(message)
}

type testFailureHandler struct {
	handleFailureFunc func(message []byte, errorMessage string) error
}

func (h *testFailureHandler) HandleFailure(_ context.Context, message []byte, errorMessage string) error {
	return h.handleFailureFunc(message, errorMessage)
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

	publisher, err := NewNatsPublisher(t.Context(), nc, slog.Default())
	require.NoError(t, err)

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
	subscriber, err := NewNatsSubscriber(t.Context(), nc, "test-durable", handlers, nil, slog.Default())
	require.NoError(t, err, "failed to create subscriber")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	message := []byte(`{"data":"test data"}`)
	err = publisher.Publish(t.Context(), testSubscriberSubject, "id", message)
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

func TestSubscriber_Run_WithFailure(t *testing.T) {
	opts := natstest.DefaultTestOptions
	opts.Port = -1 // Use a random port
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	ns := natstest.RunServer(&opts)
	defer ns.Shutdown()

	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	publisher, err := NewNatsPublisher(t.Context(), nc, slog.Default())
	require.NoError(t, err)

	failureHandled := make(chan struct{}, 1)
	done := make(chan struct{})

	handleFunc := func(_ []byte) error {
		return errors.New("processing failed")
	}

	failureHandleFunc := func(message []byte, errorMessage string) error {
		require.Contains(t, string(message), "test-scanjob")
		require.Equal(t, fmt.Sprintf("failed to handle message on subject %s: processing failed", testSubscriberSubject), errorMessage)

		failureHandled <- struct{}{}
		return nil
	}

	testHandler := &testHandler{handleFunc: handleFunc}
	testFailureHandler := &testFailureHandler{handleFailureFunc: failureHandleFunc}

	handlers := HandlerRegistry{
		testSubscriberSubject: testHandler,
	}

	subscriber, err := NewNatsSubscriber(t.Context(), nc, "test-durable-failure", handlers, testFailureHandler, slog.Default())
	require.NoError(t, err, "failed to create subscriber")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	message := []byte(`{"scanjob":{"name":"test-scanjob","namespace":"default"}}`)
	err = publisher.Publish(t.Context(), testSubscriberSubject, "id", message)
	require.NoError(t, err, "failed to publish message")

	go func() {
		err = subscriber.Run(ctx)
		close(done)
	}()

	select {
	case <-failureHandled:
	case <-time.After(2 * time.Second):
		require.Fail(t, "timed out waiting for failure handler to be called")
	}

	cancel()
	<-done
	require.NoError(t, err, "unexpected subscriber error")
}

func TestSubscriber_handleMessage(t *testing.T) {
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

			err := subscriber.handleMessage(t.Context(), test.msg.Subject, test.msg.Data)

			if test.expectedError == "" {
				require.NoError(t, err, "expected no error, got: %v", err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.expectedError, "expected error message does not match")
			}
		})
	}
}
