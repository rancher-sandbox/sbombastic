package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nats-io/nats.go"
	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/cmdutil"
	"github.com/rancher/sbombastic/internal/handlers"
	"github.com/rancher/sbombastic/internal/handlers/registry"
	"github.com/rancher/sbombastic/internal/messaging"
	"github.com/rancher/sbombastic/pkg/generated/clientset/versioned/scheme"
)

func main() { //nolint:funlen // This function is intentionally long to keep the main logic together.
	var natsURL string
	var natsCert string
	var natsKey string
	var natsCA string
	var logLevel string
	var runDir string

	flag.StringVar(&natsURL, "nats-url", "localhost:4222", "The URL of the NATS server")
	flag.StringVar(&natsCert, "nats-cert", "/nats/tls/tls.crt", "The path to the NATS client certificate.")
	flag.StringVar(&natsKey, "nats-key", "/nats/tls/tls.key", "The path to the NATS client key.")
	flag.StringVar(&natsCA, "nats-ca", "/nats/tls/ca.crt", "The path to the NATS CA certificate.")
	flag.StringVar(&runDir, "run-dir", "/var/run/worker", "Directory to store temporary files")
	flag.StringVar(&logLevel, "log-level", slog.LevelInfo.String(), "Log level")
	flag.Parse()

	slogLevel, err := cmdutil.ParseLogLevel(logLevel)
	if err != nil {
		//nolint:sloglint // Use the global logger since the logger is not yet initialized
		slog.Error(
			"error initializing the logger",
			"error",
			err,
		)
		os.Exit(1)
	}
	opts := slog.HandlerOptions{
		Level: slogLevel,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &opts)).With("component", "worker")
	logger.Info("Starting worker")

	ctx, cancel := context.WithCancel(context.Background())
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalChan
		cancel()
	}()

	config := ctrl.GetConfigOrDie()
	scheme := scheme.Scheme
	if err = v1alpha1.AddToScheme(scheme); err != nil {
		logger.Error("Error adding v1alpha1 to scheme", "error", err)
		os.Exit(1)
	}
	if err = storagev1alpha1.AddToScheme(scheme); err != nil {
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

	nc, err := nats.Connect(natsURL,
		nats.RetryOnFailedConnect(true),
		nats.RootCAs(natsCA),
		nats.ClientCert(natsCert, natsKey),
	)
	if err != nil {
		logger.Error("Unable to connect to NATS server", "error", err, "natsURL", natsURL)
		os.Exit(1)
	}

	publisher, err := messaging.NewNatsPublisher(ctx, nc, logger)
	if err != nil {
		logger.Error("Error creating NATS publisher", "error", err)
		os.Exit(1)
	}

	registry := messaging.HandlerRegistry{
		handlers.CreateCatalogSubject: handlers.NewCreateCatalogHandler(registryClientFactory, k8sClient, scheme, publisher, logger),
		handlers.GenerateSBOMSubject:  handlers.NewGenerateSBOMHandler(k8sClient, scheme, runDir, publisher, logger),
		handlers.ScanSBOMSubject:      handlers.NewScanSBOMHandler(k8sClient, scheme, runDir, logger),
	}
	failureHandler := handlers.NewScanJobFailureHandler(k8sClient, logger)

	subscriber, err := messaging.NewNatsSubscriber(ctx, nc, "worker", registry, failureHandler, logger)
	if err != nil {
		logger.Error("Error creating NATS subscriber", "error", err)
		os.Exit(1)
	}

	err = subscriber.Run(ctx)
	if err != nil {
		logger.Error("Error running worker subscriber", "error", err)
		os.Exit(1)
	}
}
