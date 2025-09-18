package storage

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rancher/sbombastic/api/storage/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/generic/registry"
)

const CreateSBOMTableSQL = `
CREATE TABLE IF NOT EXISTS sboms (
    name VARCHAR(253) NOT NULL,
    namespace VARCHAR(253) NOT NULL,
    object JSONB NOT NULL,
    PRIMARY KEY (name, namespace)
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
				db:          db,
				broadcaster: watch.NewBroadcaster(1000, watch.WaitIfChannelFull),
				table:       "sboms",
				newFunc:     newFunc,
				newListFunc: newListFunc,
				logger:      logger.With("store", "sbom"),
			},
		},
		CreateStrategy: strategy,
		UpdateStrategy: strategy,
		DeleteStrategy: strategy,
		TableConvertor: &sbomTableConvertor{},
	}

	options := &generic.StoreOptions{RESTOptions: optsGetter, AttrFunc: getAttrs}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, fmt.Errorf("unable to complete store with options: %w", err)
	}

	return store, nil
}

type sbomTableConvertor struct{}

func (c *sbomTableConvertor) ConvertToTable(_ context.Context, obj runtime.Object, _ runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: imageMetadataTableColumns(),
		Rows:              []metav1.TableRow{},
	}

	// Handle both single object and list
	var sboms []v1alpha1.SBOM
	switch t := obj.(type) {
	case *v1alpha1.SBOMList:
		sboms = t.Items
	case *v1alpha1.SBOM:
		sboms = []v1alpha1.SBOM{*t}
	default:
		return nil, fmt.Errorf("unexpected type %T", obj)
	}

	for _, sbom := range sboms {
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: &sbom},
			Cells:  imageMetadataTableRowCells(sbom.Name, &sbom),
		}
		table.Rows = append(table.Rows, row)
	}

	return table, nil
}
