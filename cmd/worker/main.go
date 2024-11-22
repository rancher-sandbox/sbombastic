package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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
	opts := slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &opts)).With("component", "worker")
	logger.Info("Starting worker")

	// TODO: add CLI flags for NATS server address
	sub, err := messaging.NewSubscription("nats://controller-nats.sbombastic.svc.cluster.local",
		"worker")
	if err != nil {
		logger.Error("Error creating subscription", "error", err)
		os.Exit(1)
	}

	config := ctrl.GetConfigOrDie()
	scheme := scheme.Scheme
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		logger.Error("Error adding v1alpha1 to scheme", "error", err)
		os.Exit(1)
	}
	if err := storagev1alpha1.AddToScheme(scheme); err != nil {
		logger.Error("Error adding storagev1alpha1 to scheme", "error", err)
		os.Exit(1)
	}
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		logger.Error("Error creating k8s client", "error", err)
		os.Exit(1)
	}
	registryClientFactory := func(transport http.RoundTripper) registry.Client {
		return registry.NewClient(transport, logger)
	}

	handlers := messaging.HandlerRegistry{
		messaging.CreateCatalogType: handlers.NewCreateCatalogHandler(registryClientFactory, k8sClient, scheme, logger),
		messaging.GenerateSBOMType:  handlers.NewGenerateSBOMHandler(k8sClient, scheme, "/var/run/worker", logger),
		messaging.ScanSBOMType:      handlers.NewScanSBOMHandler(k8sClient, scheme, "/var/run/worker", logger),
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
		logger.Error("Error running worker subscriber", "error", err)
		os.Exit(1)
	}
}
