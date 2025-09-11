package storage

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmoiron/sqlx"
	"github.com/rancher/sbombastic/api/storage/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/generic/registry"
)

const CreateImageTableSQL = `
CREATE TABLE IF NOT EXISTS images (
    id INTEGER PRIMARY KEY,
    name VARCHAR(253) NOT NULL,
    namespace VARCHAR(253) NOT NULL,
    object TEXT NOT NULL,
    UNIQUE(name, namespace)
);
`

// NewImageStore returns a store registry that will work against API services.
func NewImageStore(
	scheme *runtime.Scheme,
	optsGetter generic.RESTOptionsGetter,
	db *sqlx.DB,
	logger *slog.Logger,
) (*registry.Store, error) {
	strategy := newImageStrategy(scheme)

	newFunc := func() runtime.Object { return &v1alpha1.Image{} }
	newListFunc := func() runtime.Object { return &v1alpha1.ImageList{} }

	store := &registry.Store{
		NewFunc:                   newFunc,
		NewListFunc:               newListFunc,
		PredicateFunc:             matcher,
		DefaultQualifiedResource:  v1alpha1.Resource("images"),
		SingularQualifiedResource: v1alpha1.Resource("image"),
		Storage: registry.DryRunnableStorage{
			Storage: &store{
				db:          db,
				broadcaster: watch.NewBroadcaster(1000, watch.WaitIfChannelFull),
				table:       "images",
				newFunc:     newFunc,
				newListFunc: newListFunc,
				logger:      logger.With("store", "image"),
			},
		},
		CreateStrategy: strategy,
		UpdateStrategy: strategy,
		DeleteStrategy: strategy,
		TableConvertor: &imageTableConvertor{},
	}

	options := &generic.StoreOptions{RESTOptions: optsGetter, AttrFunc: getAttrs}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, fmt.Errorf("unable to complete store with options: %w", err)
	}

	return store, nil
}

type imageTableConvertor struct{}

func (c *imageTableConvertor) ConvertToTable(_ context.Context, obj runtime.Object, _ runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: imageMetadataTableColumns(),
		Rows:              []metav1.TableRow{},
	}

	// Handle both single object and list
	var images []v1alpha1.Image
	switch t := obj.(type) {
	case *v1alpha1.ImageList:
		images = t.Items
	case *v1alpha1.Image:
		images = []v1alpha1.Image{*t}
	default:
		return nil, fmt.Errorf("unexpected type %T", obj)
	}

	for _, image := range images {
		row := metav1.TableRow{
			Object: runtime.RawExtension{Object: &image},
			Cells:  imageMetadataTableRowCells(image.Name, &image),
		}
		table.Rows = append(table.Rows, row)
	}

	return table, nil
}
