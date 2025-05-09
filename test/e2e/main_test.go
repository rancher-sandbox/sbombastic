package e2e

import (
	"os"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"
)

var (
	testenv         env.Environment
	kindClusterName string
	namespace       string
	workerImage     = "ghcr.io/rancher-sandbox/sbombastic/worker:e2e-test"
	controllerImage = "ghcr.io/rancher-sandbox/sbombastic/controller:e2e-test"
	storageImage    = "ghcr.io/rancher-sandbox/sbombastic/storage:e2e-test"
)

func TestMain(m *testing.M) {
	cfg, _ := envconf.NewFromFlags()
	testenv = env.NewWithConfig(cfg)
	namespace = envconf.RandomName("sbombastic-e2e-ns", 32)
	kindClusterName = envconf.RandomName("sbombastic-e2e-cluster", 32)

	testenv.Setup(
		envfuncs.CreateCluster(kind.NewProvider(), kindClusterName),
		envfuncs.CreateNamespace(namespace),
		envfuncs.LoadImageToCluster(kindClusterName, workerImage, "--verbose", "--mode", "direct"),
		envfuncs.LoadImageToCluster(kindClusterName, controllerImage, "--verbose", "--mode", "direct"),
		envfuncs.LoadImageToCluster(kindClusterName, storageImage, "--verbose", "--mode", "direct"),
	)

	testenv.Finish(
		envfuncs.DeleteNamespace(namespace),
		envfuncs.DestroyCluster(kindClusterName),
	)

	os.Exit(testenv.Run(m))
}
