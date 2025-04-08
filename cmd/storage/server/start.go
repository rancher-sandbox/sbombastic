/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/featuregate"
	baseversion "k8s.io/component-base/version"
	netutils "k8s.io/utils/net"

	"github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/internal/apiserver"
	informers "github.com/rancher/sbombastic/pkg/generated/informers/externalversions"
	sampleopenapi "github.com/rancher/sbombastic/pkg/generated/openapi"
)

// WardleServerOptions contains state for master/api server
type WardleServerOptions struct {
	RecommendedOptions    *genericoptions.RecommendedOptions
	SharedInformerFactory informers.SharedInformerFactory

	AlternateDNS []string

	DB     *sqlx.DB
	Logger *slog.Logger
}

func WardleVersionToKubeVersion(ver *version.Version) *version.Version {
	if ver.Major() != 1 {
		return nil
	}
	kubeVer := baseversion.DefaultKubeEffectiveVersion().BinaryVersion()
	// "1.2" maps to kubeVer
	minor := ver.Minor()
	if minor > math.MaxInt32 {
		panic("minor version is too large")
	}

	offset := int(minor) - 2
	mappedVer := kubeVer.OffsetMinor(offset)
	if mappedVer.GreaterThan(kubeVer) {
		return kubeVer
	}
	return mappedVer
}

// NewWardleServerOptions returns a new WardleServerOptions
func NewWardleServerOptions(db *sqlx.DB, logger *slog.Logger) *WardleServerOptions {
	o := &WardleServerOptions{
		RecommendedOptions: genericoptions.NewRecommendedOptions(
			"/registry/sbombastic.rancher.io",
			apiserver.Codecs.LegacyCodec(v1alpha1.SchemeGroupVersion),
		),
		DB:     db,
		Logger: logger,
	}

	// Disable etcd
	o.RecommendedOptions.Etcd = nil
	// Disable admission plugins
	o.RecommendedOptions.Admission = nil
	// Disable priority and fairness as it is not compatible with old versions of Kubernetes
	o.RecommendedOptions.Features.EnablePriorityAndFairness = false
	return o
}

// NewCommandStartWardleServer provides a CLI handler for 'start master' command
// with a default WardleServerOptions.
func NewCommandStartWardleServer(ctx context.Context, defaults *WardleServerOptions) *cobra.Command {
	o := *defaults
	cmd := &cobra.Command{
		Short: "Launch a wardle API server",
		Long:  "Launch a wardle API server",
		PersistentPreRunE: func(*cobra.Command, []string) error {
			return featuregate.DefaultComponentGlobalsRegistry.Set()
		},
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Validate(args); err != nil {
				return err
			}
			if err := o.RunWardleServer(c.Context()); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.SetContext(ctx)

	flags := cmd.Flags()
	o.RecommendedOptions.AddFlags(flags)

	// The following lines demonstrate how to configure version compatibility and feature gates
	// for the "Wardle" component, as an example of KEP-4330.

	// Create an effective version object for the "Wardle" component.
	// This initializes the binary version, the emulation version and the minimum compatibility version.
	//
	// Note:
	// - The binary version represents the actual version of the running source code.
	// - The emulation version is the version whose capabilities are being emulated by the binary.
	// - The minimum compatibility version specifies the minimum version that the component remains compatible with.
	//
	// Refer to KEP-4330 for more details: https://github.com/kubernetes/enhancements/blob/master/keps/sig-architecture/4330-compatibility-versions
	defaultWardleVersion := "1.2"
	// Register the "Wardle" component with the global component registry,
	// associating it with its effective version and feature gate configuration.
	// Will skip if the component has been registered, like in the integration test.
	_, _ = featuregate.DefaultComponentGlobalsRegistry.ComponentGlobalsOrRegister(
		apiserver.WardleComponentName, baseversion.NewEffectiveVersion(defaultWardleVersion),
		featuregate.NewVersionedFeatureGate(version.MustParse(defaultWardleVersion)))

	// Register the default kube component if not already present in the global registry.
	_, _ = featuregate.DefaultComponentGlobalsRegistry.ComponentGlobalsOrRegister(featuregate.DefaultKubeComponent,
		baseversion.NewEffectiveVersion(baseversion.DefaultKubeBinaryVersion), utilfeature.DefaultMutableFeatureGate)

	// Set the emulation version mapping from the "Wardle" component to the kube component.
	// This ensures that the emulation version of the latter is determined by the emulation version of the former.
	utilruntime.Must(
		featuregate.DefaultComponentGlobalsRegistry.SetEmulationVersionMapping(
			apiserver.WardleComponentName,
			featuregate.DefaultKubeComponent,
			WardleVersionToKubeVersion,
		),
	)

	featuregate.DefaultComponentGlobalsRegistry.AddFlags(flags)

	return cmd
}

// Validate validates WardleServerOptions
func (o *WardleServerOptions) Validate(_ []string) error {
	errors := []error{}
	errors = append(errors, o.RecommendedOptions.Validate()...)
	errors = append(errors, featuregate.DefaultComponentGlobalsRegistry.Validate()...)
	return utilerrors.NewAggregate(errors)
}

// Complete fills in fields required to have valid data
func (o *WardleServerOptions) Complete() error {
	return nil
}

// Config returns config for the api server given WardleServerOptions
func (o *WardleServerOptions) Config() (*apiserver.Config, error) {
	// TODO have a "real" external address
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", o.AlternateDNS, []net.IP{netutils.ParseIPSloppy("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("error creating self-signed certificates: %w", err)
	}

	serverConfig := genericapiserver.NewRecommendedConfig(apiserver.Codecs)

	serverConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(
		sampleopenapi.GetOpenAPIDefinitions,
		openapi.NewDefinitionNamer(apiserver.Scheme),
	)
	serverConfig.OpenAPIConfig.Info.Title = "Wardle"
	serverConfig.OpenAPIConfig.Info.Version = "0.1"

	serverConfig.OpenAPIV3Config = genericapiserver.DefaultOpenAPIV3Config(
		sampleopenapi.GetOpenAPIDefinitions,
		openapi.NewDefinitionNamer(apiserver.Scheme),
	)
	serverConfig.OpenAPIV3Config.Info.Title = "Wardle"
	serverConfig.OpenAPIV3Config.Info.Version = "0.1"

	serverConfig.FeatureGate = featuregate.DefaultComponentGlobalsRegistry.FeatureGateFor(
		featuregate.DefaultKubeComponent,
	)
	serverConfig.EffectiveVersion = featuregate.DefaultComponentGlobalsRegistry.EffectiveVersionFor(
		apiserver.WardleComponentName,
	)

	// As we don't have a real etcd, we need to set a dummy storage factory
	serverConfig.RESTOptionsGetter = &genericoptions.StorageFactoryRestOptionsFactory{
		StorageFactory: &genericoptions.SimpleStorageFactory{},
	}

	if err := o.RecommendedOptions.ApplyTo(serverConfig); err != nil {
		return nil, fmt.Errorf("error applying options to server config: %w", err)
	}

	config := &apiserver.Config{
		GenericConfig: serverConfig,
		ExtraConfig:   apiserver.ExtraConfig{},
	}
	return config, nil
}

// RunWardleServer starts a new WardleServer given WardleServerOptions
func (o *WardleServerOptions) RunWardleServer(ctx context.Context) error {
	config, err := o.Config()
	if err != nil {
		return err
	}

	server, err := config.Complete().New(o.DB, o.Logger)
	if err != nil {
		return fmt.Errorf("error creating server: %w", err)
	}

	if err = server.GenericAPIServer.PrepareRun().RunWithContext(ctx); err != nil {
		return fmt.Errorf("error while running server: %w", err)
	}

	return nil
}
