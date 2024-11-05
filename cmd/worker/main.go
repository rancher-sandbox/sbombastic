package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rancher/sbombastic/internal/messaging"
	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("failed to create logger: %v", err))
	}
	defer logger.Sync() //nolint: errcheck // flushes buffer, ignore error

	sub, err := messaging.NewSubscription("nats://controller-nats.sbombastic.svc.cluster.local",
		"worker")
	if err != nil {
		logger.Fatal("Error creating subscription", zap.Error(err))
	}

	handlers := messaging.HandlerRegistry{}
	subscriber := messaging.NewSubscriber(sub, handlers, logger)

	ctx, cancel := context.WithCancel(context.Background())

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalChan
		cancel()
	}()

	go func() {
		err := subscriber.Run(ctx)
		if err != nil {
			logger.Fatal("Error running worker subscriber", zap.Error(err))
		}
	}()
}
