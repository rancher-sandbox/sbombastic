package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/handlers"
	"github.com/rancher/sbombastic/internal/handlers/registry"
	"github.com/rancher/sbombastic/internal/messaging"
	"github.com/rancher/sbombastic/pkg/generated/clientset/versioned/scheme"
)

func main() {
	// TODO: add CLI flags for log level
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(fmt.Sprintf("failed to create logger: %v", err))
	}
	defer logger.Sync() //nolint: errcheck // flushes buffer, ignore error

	logger.Info("Starting worker")

	// TODO: add CLI flags for NATS server address
	sub, err := messaging.NewSubscription("nats://controller-nats.sbombastic.svc.cluster.local",
		"worker")
	if err != nil {
		logger.Fatal("Error creating subscription", zap.Error(err))
	}

	config := ctrl.GetConfigOrDie()
	scheme := scheme.Scheme
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		logger.Fatal("Error adding v1alpha1 to scheme", zap.Error(err))
	}
	if err := storagev1alpha1.AddToScheme(scheme); err != nil {
		logger.Fatal("Error adding storagev1alpha1 to scheme", zap.Error(err))
	}
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		logger.Fatal("Error creating k8s client", zap.Error(err))
	}
	registryClientFactory := func(transport http.RoundTripper) registry.Client {
		return registry.NewClient(transport, logger)
	}

	handlers := messaging.HandlerRegistry{
		messaging.CreateCatalogType: handlers.NewCreateCatalogHandler(registryClientFactory, k8sClient, scheme, logger),
		messaging.GenerateSBOMType:  handlers.NewGenerateSBOMHandler(k8sClient, "/var/run/worker", logger),
		messaging.ScanSBOMType:      handlers.NewScanSBOMHandler(k8sClient, "/var/run/worker", logger),
	}
	subscriber := messaging.NewSubscriber(sub, handlers, logger)

	ctx, cancel := context.WithCancel(context.Background())
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalChan
		cancel()
	}()

	err = subscriber.Run(ctx)
	if err != nil {
		logger.Fatal("Error running worker subscriber", zap.Error(err))
	}
}
