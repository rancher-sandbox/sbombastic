package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/cmdutil"
	"github.com/rancher/sbombastic/internal/handlers"
	"github.com/rancher/sbombastic/internal/handlers/registry"
	"github.com/rancher/sbombastic/internal/messaging"
	"github.com/rancher/sbombastic/pkg/generated/clientset/versioned/scheme"
)

func main() {
	var natsURL string
	var logLevel string
	var runDir string

	flag.StringVar(&natsURL, "nats-url", "localhost:4222", "The URL of the NATS server")
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

	var deployNamespace string
	if deployNamespace, err = getDeployNamespace(logger); err != nil {
		os.Exit(1)
	}

	sub, err := messaging.NewSubscription(natsURL, "worker")
	if err != nil {
		logger.Error("Error creating subscription", "error", err)
		os.Exit(1)
	}

	scheme, k8sClient, clientSet, err := newK8sClientScheme(logger)
	if err != nil {
		os.Exit(1)
	}
	coordinationClient := clientSet.CoordinationV1()
	registryClientFactory := func(transport http.RoundTripper) registry.Client {
		return registry.NewClient(transport, logger)
	}

	handlers := messaging.HandlerRegistry{
		messaging.CreateCatalogType: handlers.NewCreateCatalogHandler(registryClientFactory, k8sClient,
			func(leaseName, leaseNamespace, identity string) resourcelock.Interface {
				return &resourcelock.LeaseLock{
					LeaseMeta: metav1.ObjectMeta{
						Name:      leaseName,
						Namespace: leaseNamespace,
					},
					Client: coordinationClient,
					LockConfig: resourcelock.ResourceLockConfig{
						Identity: identity,
					},
				}
			}, scheme, logger, deployNamespace),
		messaging.GenerateSBOMType: handlers.NewGenerateSBOMHandler(k8sClient, scheme, runDir, logger),
		messaging.ScanSBOMType:     handlers.NewScanSBOMHandler(k8sClient, scheme, runDir, logger),
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

func getDeployNamespace(logger *slog.Logger) (string, error) {
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err == nil {
		return string(data), nil
	}
	logger.Error("Failed to open namespace file", "error", err)
	return "", err
}

func newK8sClientScheme(logger *slog.Logger) (*runtime.Scheme, client.Client, *kubernetes.Clientset, error) {
	var err error

	config := ctrl.GetConfigOrDie()
	scheme := scheme.Scheme
	if err = v1alpha1.AddToScheme(scheme); err != nil {
		logger.Error("Error adding v1alpha1 to scheme", "error", err)
		return nil, nil, nil, err
	}
	if err = storagev1alpha1.AddToScheme(scheme); err != nil {
		logger.Error("Error adding storagev1alpha1 to scheme", "error", err)
		os.Exit(1)
	}
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		logger.Error("Error creating k8s client", "error", err)
		return nil, nil, nil, err
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error("Error creating k8s client", "error", err)
		return nil, nil, nil, err
	}

	return scheme, k8sClient, clientSet, nil
}
