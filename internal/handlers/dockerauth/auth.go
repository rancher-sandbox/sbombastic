package dockerauth

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/rancher/sbombastic/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// secretTypeDockerConfigJSON contains a dockercfg file that follows the
	// same format rules as ~/.docker/config.json
	// https://github.com/kubernetes/kubernetes/blob/master/pkg/apis/core/types.go#L6545
	secretTypeDockerConfigJSON = "kubernetes.io/dockerconfigjson" //nolint:gosec // this is not an hardcoded credential
	// dockerConfigJSONKey is the key of the required data for
	// SecretTypeDockerConfigJson secrets
	// https://github.com/kubernetes/kubernetes/blob/master/pkg/apis/core/types.go#L6547
	dockerConfigJSONKey = ".dockerconfigjson"
)

// SetupDockerAuthForRegistry retrieve the Secret listed in the Registry resource
// and creates the dockerconfig file.
func SetupDockerAuthForRegistry(ctx context.Context, k8sClient client.Client, registry *v1alpha1.Registry) error {
	authSecret := &corev1.Secret{}
	err := k8sClient.Get(ctx, k8stypes.NamespacedName{
		Name:      registry.Spec.AuthSecret,
		Namespace: registry.Namespace,
	}, authSecret)
	if err != nil {
		return fmt.Errorf("cannot get Secret %s: %w", registry.Spec.AuthSecret, err)
	}

	secretData := authSecret.Data[dockerConfigJSONKey]
	dockerConfig, err := createDockerConfigJSON(registry.Spec.URI, secretData)
	if err != nil {
		return fmt.Errorf("cannot create dockerconfig file: %w", err)
	}

	err = os.Setenv("DOCKER_CONFIG", dockerConfig)
	if err != nil {
		return fmt.Errorf("cannot set DOCKER_CONFIG env: %w", err)
	}
	return nil
}

// createDockerConfigJSON creates the config.json file used by docker / trivy to
// get credentials to connect to the registry.
func createDockerConfigJSON(serverAddress string, data []byte) (string, error) {
	cf, err := config.LoadFromReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to load docker config: %w", err)
	}
	creds := cf.GetCredentialsStore(serverAddress)
	if serverAddress == name.DefaultRegistry {
		serverAddress = authn.DefaultAuthKey
	}
	authConfig, err := creds.Get(serverAddress)
	if err != nil {
		return "", fmt.Errorf("failed to get credentials from store: %w", err)
	}
	dockerConfig, err := os.MkdirTemp("/tmp", "dockerconfig-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary dockerconfig dir: %w", err)
	}
	cf.Filename = path.Join(dockerConfig, "config.json")
	if err := creds.Store(types.AuthConfig{
		ServerAddress: serverAddress,
		Username:      authConfig.Username,
		Password:      authConfig.Password,
	}); err != nil {
		return "", fmt.Errorf("failed to store credentials: %w", err)
	}
	if err := cf.Save(); err != nil {
		return "", fmt.Errorf("failed to save docker config: %w", err)
	}
	return dockerConfig, nil
}
