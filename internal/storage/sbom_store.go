package storage

import (
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/internal/storage/writer"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

const CreateSBOMTableSQL = `
CREATE TABLE IF NOT EXISTS sboms (
    id INTEGER PRIMARY KEY,
    name VARCHAR(253) NOT NULL,
    namespace VARCHAR(253) NOT NULL,
    object TEXT NOT NULL,
    UNIQUE(name, namespace)
);
`

// NewSBOMStore returns a store registry that will work against API services.
func NewSBOMStore(
	scheme *runtime.Scheme,
	optsGetter generic.RESTOptionsGetter,
	db *pgxpool.Pool,
	logger *slog.Logger,
) (*registry.Store, error) {
	strategy := newSBOMStrategy(scheme)

	newFunc := func() runtime.Object { return &v1alpha1.SBOM{} }
	newListFunc := func() runtime.Object { return &v1alpha1.SBOMList{} }

	store := &registry.Store{
		NewFunc:                   newFunc,
		NewListFunc:               newListFunc,
		PredicateFunc:             matcher,
		DefaultQualifiedResource:  v1alpha1.Resource("sboms"),
		SingularQualifiedResource: v1alpha1.Resource("sbom"),
		Storage: registry.DryRunnableStorage{
			Storage: &store{
				broadcaster: watch.NewBroadcaster(1000, watch.WaitIfChannelFull),
				writer:      writer.NewSbomWriter(db),
				newFunc:     newFunc,
				newListFunc: newListFunc,
				logger:      logger.With("store", "sbom"),
			},
		},
		CreateStrategy: strategy,
		UpdateStrategy: strategy,
		DeleteStrategy: strategy,

		// TODO: define table converter that exposes more than name/creation timestamp
		TableConvertor: rest.NewDefaultTableConvertor(v1alpha1.Resource("sboms")),
	}

	options := &generic.StoreOptions{RESTOptions: optsGetter, AttrFunc: getAttrs}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, fmt.Errorf("unable to complete store with options: %w", err)
	}

	return store, nil
}
