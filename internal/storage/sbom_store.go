package storage

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/rancher/sbombastic/api/storage/v1alpha1"
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
func NewSBOMStore(scheme *runtime.Scheme, optsGetter generic.RESTOptionsGetter, db *sqlx.DB) (*registry.Store, error) {
	strategy := NewStrategy(scheme)

	newFunc := func() runtime.Object { return &v1alpha1.SBOM{} }
	newListFunc := func() runtime.Object { return &v1alpha1.SBOMList{} }

	store := &registry.Store{
		NewFunc:                   newFunc,
		NewListFunc:               newListFunc,
		PredicateFunc:             MatchSBOM,
		DefaultQualifiedResource:  v1alpha1.Resource("sboms"),
		SingularQualifiedResource: v1alpha1.Resource("sbom"),
		Storage: registry.DryRunnableStorage{
			Storage: &store{
				db:          db,
				broadcaster: watch.NewBroadcaster(1000, watch.WaitIfChannelFull),
				table:       "sboms",
				newFunc:     newFunc,
				newListFunc: newListFunc,
			},
		},
		CreateStrategy: strategy,
		UpdateStrategy: strategy,
		DeleteStrategy: strategy,

		// TODO: define table converter that exposes more than name/creation timestamp
		TableConvertor: rest.NewDefaultTableConvertor(v1alpha1.Resource("sboms")),
	}

	options := &generic.StoreOptions{RESTOptions: optsGetter, AttrFunc: GetAttrs}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, fmt.Errorf("unable to complete store with options: %w", err)
	}

	return store, nil
}
