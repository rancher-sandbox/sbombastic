//nolint:wrapcheck // We want to return the errors from k8s.io/apiserver/pkg/storage as they are.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"

	"github.com/jmoiron/sqlx"
	"github.com/rancher/sbombastic/api/storage/v1alpha1"
)

// NewSBOMStorage returns a store registry that will work against API services.
func NewSBOMStorage(scheme *runtime.Scheme, optsGetter generic.RESTOptionsGetter, db *sqlx.DB) (*genericregistry.Store, error) {
	strategy := NewStrategy(scheme)

	store := &genericregistry.Store{
		NewFunc:                   func() runtime.Object { return &v1alpha1.SBOM{} },
		NewListFunc:               func() runtime.Object { return &v1alpha1.SBOMList{} },
		PredicateFunc:             MatchFlunder,
		DefaultQualifiedResource:  v1alpha1.Resource("sboms"),
		SingularQualifiedResource: v1alpha1.Resource("sbom"),
		Storage: genericregistry.DryRunnableStorage{Codec: nil, Storage: &sbomStorage{
			db: db,
		}},
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

var _ storage.Interface = &sbomStorage{}

type sbomStorage struct {
	db *sqlx.DB
}

func (s *sbomStorage) Versioner() storage.Versioner {
	return storage.APIObjectVersioner{}
}

func (s *sbomStorage) Create(ctx context.Context, key string, obj, out runtime.Object, _ uint64) error {
	sbom, ok := obj.(*v1alpha1.SBOM)
	if !ok {
		return storage.NewInvalidObjError(key, fmt.Sprintf("unexpected object type: %T", obj))
	}

	bytes, err := json.Marshal(sbom)
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	sbomRecord := sbomSchema{
		Key:    key,
		Object: bytes,
	}

	_, err = s.db.NamedExecContext(ctx, "INSERT INTO sbom (key, object) VALUES (:key, :object)", &sbomRecord)
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	outSBOM, ok := out.(*v1alpha1.SBOM)
	if !ok {
		return storage.NewInvalidObjError(key, fmt.Sprintf("unexpected out object type: %T", out))
	}

	*outSBOM = *sbom

	return nil
}

func (s *sbomStorage) Delete(
	ctx context.Context, key string, out runtime.Object, preconditions *storage.Preconditions,
	_ storage.ValidateObjectFunc, _ runtime.Object,
) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return storage.NewInternalError(err.Error())
	}
	defer tx.Rollback()

	var sbomRecord sbomSchema
	if err := tx.GetContext(ctx, &sbomRecord, "DELETE FROM sbom WHERE key = ? RETURNING key, object", key); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.NewKeyNotFoundError(key, 0)
		}
		return storage.NewInternalError(err.Error())
	}

	var sbom v1alpha1.SBOM
	if err := json.Unmarshal(sbomRecord.Object, &sbom); err != nil {
		return storage.NewInternalError(err.Error())
	}

	if err := preconditions.Check(key, &sbom); err != nil {
		return err
	}

	err = json.Unmarshal(sbomRecord.Object, out)
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	if err := tx.Commit(); err != nil {
		return storage.NewInternalError(err.Error())
	}

	return nil
}

func (s *sbomStorage) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	log.Printf("Watch() called: key=%s\n", key)
	return watch.NewEmptyWatch(), nil
}

func (s *sbomStorage) Get(ctx context.Context, key string, _ storage.GetOptions, objPtr runtime.Object) error {
	log.Printf("Get() called: key=%s\n", key)

	var sbomRecord sbomSchema
	err := s.db.GetContext(ctx, &sbomRecord, "SELECT * FROM sbom WHERE  key = ?", key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.NewKeyNotFoundError(key, 0)
		}

		return storage.NewInternalError(err.Error())
	}

	err = json.Unmarshal(sbomRecord.Object, objPtr)
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	return nil
}

func (s *sbomStorage) GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	log.Printf("GetList() called: key=%s\n", key)

	sbomList, ok := listObj.(*v1alpha1.SBOMList)
	if !ok {
		return storage.NewInternalError(fmt.Sprintf("unexpected object type: %T", listObj))
	}

	var sbomRecords []sbomSchema
	if err := s.db.SelectContext(ctx, &sbomRecords, "SELECT * FROM sbom"); err != nil {
		return storage.NewInternalError(err.Error())
	}

	for _, sbomRecord := range sbomRecords {
		var sbom v1alpha1.SBOM
		if err := json.Unmarshal(sbomRecord.Object, &sbom); err != nil {
			return storage.NewInternalError(err.Error())
		}

		sbomList.Items = append(sbomList.Items, sbom)
	}

	return nil
}

func (s *sbomStorage) GuaranteedUpdate(
	ctx context.Context,
	key string,
	destination runtime.Object,
	ignoreNotFound bool,
	preconditions *storage.Preconditions,
	tryUpdate storage.UpdateFunc,
	cachedExistingObject runtime.Object,
) error {
	var existingSBOM v1alpha1.SBOM

	for {
		if err := s.Get(ctx, key, storage.GetOptions{}, &existingSBOM); err != nil {
			if storage.IsNotFound(err) && ignoreNotFound {
				return nil
			}
			if !ignoreNotFound {
				return err
			}
		}

		if err := preconditions.Check(key, &existingSBOM); err != nil {
			return err
		}

		if err := tryUpdate(&existingSBOM); err != nil {
			continue
		}
	}
}

func (s *sbomStorage) Count(key string) (int64, error) {
	var count int64
	if err := s.db.Get(&count, "SELECT COUNT(*) FROM sbom WHERE key LIKE ?", key+"%"); err != nil {
		return 0, storage.NewInternalError(err.Error())
	}

	return count, nil
}

func (s *sbomStorage) ReadinessCheck() error {
	log.Println("ReadinessCheck() called")
	return nil
}

func (s *sbomStorage) RequestWatchProgress(ctx context.Context) error {
	log.Println("RequestWatchProgress() called")
	return nil
}
