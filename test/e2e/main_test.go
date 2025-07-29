package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"
	"sigs.k8s.io/e2e-framework/third_party/helm"
)

var (
	testenv              env.Environment
	kindClusterName      string
	namespace            = "sbombastic"
	workerImage          = "ghcr.io/rancher-sandbox/sbombastic/worker:latest"
	controllerImage      = "ghcr.io/rancher-sandbox/sbombastic/controller:latest"
	storageImage         = "ghcr.io/rancher-sandbox/sbombastic/storage:latest"
	certManagerNamespace = "cert-manager"
	certManagerVersion   = "v1.18.2"
)

func TestMain(m *testing.M) {
	cfg, _ := envconf.NewFromFlags()
	testenv = env.NewWithConfig(cfg)
	kindClusterName = envconf.RandomName("sbombastic-e2e-cluster", 32)

	testenv.Setup(
		envfuncs.CreateCluster(kind.NewProvider(), kindClusterName),
		envfuncs.CreateNamespace(namespace, envfuncs.WithLabels(map[string]string{
			"pod-security.kubernetes.io/enforce":         "restricted",
			"pod-security.kubernetes.io/enforce-version": "latest",
		})),
		envfuncs.LoadImageToCluster(kindClusterName, workerImage, "--verbose", "--mode", "direct"),
		envfuncs.LoadImageToCluster(kindClusterName, controllerImage, "--verbose", "--mode", "direct"),
		envfuncs.LoadImageToCluster(kindClusterName, storageImage, "--verbose", "--mode", "direct"),
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			manager := helm.New(cfg.KubeconfigFile())

			// Add the Jetstack Helm repository for cert-manager
			err := manager.RunRepo(helm.WithArgs(
				"add",
				"jetstack",
				"https://charts.jetstack.io",
				"--force-update"),
			)
			if err != nil {
				return ctx, fmt.Errorf("failed to add cert-manager helm repo: %w", err)
			}

			// Install cert-manager
			err = manager.RunInstall(
				helm.WithName("cert-manager"),
				helm.WithChart("jetstack/cert-manager"),
				helm.WithWait(),
				helm.WithArgs("--version", certManagerVersion),
				helm.WithArgs("--set", "installCRDs=true"),
				helm.WithNamespace(certManagerNamespace),
				helm.WithArgs("--create-namespace"),
				helm.WithTimeout("3m"))
			if err != nil {
				return ctx, fmt.Errorf("failed to install cert-manager: %w", err)
			}

			return ctx, nil
		},
	)

	testenv.Finish(
		envfuncs.ExportClusterLogs(kindClusterName, "./logs"),
		envfuncs.DestroyCluster(kindClusterName),
	)

	os.Exit(testenv.Run(m))
}
