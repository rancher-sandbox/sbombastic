//nolint:wrapcheck // We want to return the errors from k8s.io/apiserver/pkg/storage as they are.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"

	"github.com/rancher/sbombastic/api/storage/v1alpha1"
)

// NewSBOMStore returns a store registry that will work against API services.
func NewSBOMStore(scheme *runtime.Scheme, optsGetter generic.RESTOptionsGetter, db *sqlx.DB) (*genericregistry.Store, error) {
	strategy := NewStrategy(scheme)

	store := &genericregistry.Store{
		NewFunc:                   func() runtime.Object { return &v1alpha1.SBOM{} },
		NewListFunc:               func() runtime.Object { return &v1alpha1.SBOMList{} },
		PredicateFunc:             MatchSBOM,
		DefaultQualifiedResource:  v1alpha1.Resource("sboms"),
		SingularQualifiedResource: v1alpha1.Resource("sbom"),
		Storage: genericregistry.DryRunnableStorage{
			Storage: &sbomStore{
				broadcaster: watch.NewBroadcaster(1000, watch.WaitIfChannelFull),
				db:          db,
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

var _ storage.Interface = &sbomStore{}

type sbomStore struct {
	broadcaster *watch.Broadcaster
	db          *sqlx.DB
}

// Returns Versioner associated with this interface.
func (s *sbomStore) Versioner() storage.Versioner {
	return storage.APIObjectVersioner{}
}

// Create adds a new object at a key unless it already exists. 'ttl' is time-to-live
// in seconds (0 means forever). If no error is returned and out is not nil, out will be
// set to the read value from database.
func (s *sbomStore) Create(ctx context.Context, key string, obj, out runtime.Object, _ uint64) error {
	name, namespace := extractNameAndNamespace(key)
	if name == "" || namespace == "" {
		return storage.NewInternalErrorf("invalid key: %s", key)
	}

	sbom, ok := obj.(*v1alpha1.SBOM)
	if !ok {
		return storage.NewInvalidObjError(key, fmt.Sprintf("unexpected object type: %T", obj))
	}

	if err := s.Versioner().UpdateObject(obj, 1); err != nil {
		return storage.NewInternalError(err.Error())
	}

	bytes, err := json.Marshal(sbom)
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	query, args, err := sq.Insert("sboms").
		Columns("name", "namespace", "object").
		Values(name, namespace, bytes).
		ToSql()
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	outSBOM, ok := out.(*v1alpha1.SBOM)
	if !ok {
		return storage.NewInvalidObjError(key, fmt.Sprintf("unexpected out object type: %T", out))
	}

	*outSBOM = *sbom

	if err := s.broadcaster.Action(watch.Added, sbom); err != nil {
		return storage.NewInternalError(err.Error())
	}

	return nil
}

// Delete removes the specified key and returns the value that existed at that spot.
// If key didn't exist, it will return NotFound storage error.
// If 'cachedExistingObject' is non-nil, it can be used as a suggestion about the
// current version of the object to avoid read operation from storage to get it.
// However, the implementations have to retry in case suggestion is stale.
func (s *sbomStore) Delete(
	ctx context.Context, key string, out runtime.Object, preconditions *storage.Preconditions,
	validateDeletion storage.ValidateObjectFunc, _ runtime.Object,
) error {
	name, namespace := extractNameAndNamespace(key)
	if name == "" || namespace == "" {
		return storage.NewInternalErrorf("invalid key: %s", key)
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return storage.NewInternalError(err.Error())
	}
	defer func() {
		if err := tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
			log.Printf("failed to rollback transaction: %v", err)
		}
	}()

	query, args, err := sq.Delete("sboms").
		Where(sq.Eq{"name": name, "namespace": namespace}).
		Suffix("RETURNING *").
		ToSql()
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	sbomRecord := &sbomSchema{}
	if err := tx.GetContext(ctx, sbomRecord, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.NewKeyNotFoundError(key, 0)
		}
		return storage.NewInternalError(err.Error())
	}

	if err := json.Unmarshal(sbomRecord.Object, out); err != nil {
		return storage.NewInternalError(err.Error())
	}

	if err := preconditions.Check(key, out); err != nil {
		return err
	}

	if err := validateDeletion(ctx, out); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return storage.NewInternalError(err.Error())
	}

	if err := s.broadcaster.Action(watch.Deleted, out); err != nil {
		return storage.NewInternalError(err.Error())
	}

	return nil
}

// Watch begins watching the specified key. Events are decoded into API objects,
// and any items selected by the options in 'opts' are sent down to returned watch.Interface.
// resourceVersion may be used to specify what version to begin watching,
// which should be the current resourceVersion, and no longer rv+1
// (e.g. reconnecting without missing any updates).
// If resource version is "0", this interface will get current object at given key
// and send it in an "ADDED" event, before watch starts.
func (s *sbomStore) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	if opts.ResourceVersion == "" {
		return s.broadcaster.Watch()
	}

	if opts.ResourceVersion == "0" {
		sbom := &v1alpha1.SBOM{}
		if err := s.Get(ctx, key, storage.GetOptions{}, sbom); err != nil {
			return nil, err
		}

		return s.broadcaster.WatchWithPrefix([]watch.Event{{Type: watch.Added, Object: sbom}})
	}

	sbomList := &v1alpha1.SBOMList{}
	if err := s.GetList(ctx, key, opts, sbomList); err != nil {
		return nil, err
	}
	var events []watch.Event
	for _, item := range sbomList.Items {
		events = append(events, watch.Event{Type: watch.Added, Object: &item})
	}

	return s.broadcaster.WatchWithPrefix(events)
}

// Get unmarshals object found at key into objPtr. On a not found error, will either
// return a zero object of the requested type, or an error, depending on 'opts.ignoreNotFound'.
// Treats empty responses and nil response nodes exactly like a not found error.
// The returned contents may be delayed, but it is guaranteed that they will
// match 'opts.ResourceVersion' according 'opts.ResourceVersionMatch'.
func (s *sbomStore) Get(ctx context.Context, key string, opts storage.GetOptions, objPtr runtime.Object) error {
	name, namespace := extractNameAndNamespace(key)
	if name == "" || namespace == "" {
		return storage.NewInternalErrorf("invalid key: %s", key)
	}

	query, args, err := sq.Select("*").
		From("sboms").
		Where(sq.Eq{"name": name, "namespace": namespace}).
		ToSql()
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	out, ok := objPtr.(*v1alpha1.SBOM)
	if !ok {
		return storage.NewInvalidObjError(key, fmt.Sprintf("unexpected out object type: %T", out))
	}

	sbomRecord := &sbomSchema{}
	if err := s.db.GetContext(ctx, sbomRecord, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if opts.IgnoreNotFound {
				*out = v1alpha1.SBOM{}

				return nil
			}

			return storage.NewKeyNotFoundError(key, 0)
		}
		return storage.NewInternalError(err.Error())
	}

	err = json.Unmarshal(sbomRecord.Object, out)
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	return nil
}

// GetList unmarshalls objects found at key into a *List api object (an object
// that satisfies runtime.IsList definition).
// The returned contents may be delayed, but it is guaranteed that they will
// match 'opts.ResourceVersion' according 'opts.ResourceVersionMatch'.
func (s *sbomStore) GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	sbomList, ok := listObj.(*v1alpha1.SBOMList)
	if !ok {
		return storage.NewInvalidObjError(key, fmt.Sprintf("unexpected out object type: %T", listObj))
	}

	queryBuilder := sq.Select("*").From("sboms")
	namespace := extractNamespace(key)
	if namespace != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"namespace": namespace})
	}
	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	var sbomRecords []sbomSchema
	if err := s.db.SelectContext(ctx, &sbomRecords, query, args...); err != nil {
		return storage.NewInternalError(err.Error())
	}

	for _, sbomRecord := range sbomRecords {
		sbom := &v1alpha1.SBOM{}
		if err := json.Unmarshal(sbomRecord.Object, sbom); err != nil {
			return storage.NewInternalError(err.Error())
		}

		if opts.Predicate.Label != nil {
			if !opts.Predicate.Label.Matches(labels.Set(sbom.GetLabels())) {
				continue
			}
		}
		sbomList.Items = append(sbomList.Items, *sbom)
	}

	return nil
}

// GuaranteedUpdate keeps calling 'tryUpdate()' to update key 'key' (of type 'destination')
// retrying the update until success if there is index conflict.
// Note that object passed to tryUpdate may change across invocations of tryUpdate() if
// other writers are simultaneously updating it, so tryUpdate() needs to take into account
// the current contents of the object when deciding how the update object should look.
// If the key doesn't exist, it will return NotFound storage error if ignoreNotFound=false
// else `destination` will be set to the zero value of it's type.
// If the eventual successful invocation of `tryUpdate` returns an output with the same serialized
// contents as the input, it won't perform any update, but instead set `destination` to an object with those
// contents.
// If 'cachedExistingObject' is non-nil, it can be used as a suggestion about the
// current version of the object to avoid read operation from storage to get it.
// However, the implementations have to retry in case suggestion is stale.
//
// Example:
//
// s := /* implementation of Interface */
// err := s.GuaranteedUpdate(
//
//	 "myKey", &MyType{}, true, preconditions,
//	 func(input runtime.Object, res ResponseMeta) (runtime.Object, *uint64, error) {
//	   // Before each invocation of the user defined function, "input" is reset to
//	   // current contents for "myKey" in database.
//	   curr := input.(*MyType)  // Guaranteed to succeed.
//
//	   // Make the modification
//	   curr.Counter++
//
//	   // Return the modified object - return an error to stop iterating. Return
//	   // a uint64 to alter the TTL on the object, or nil to keep it the same value.
//	   return cur, nil, nil
//	}, cachedExistingObject
//
// )
//
//nolint:gocognit,funlen // This functions can't be easily split into smaller parts.
func (s *sbomStore) GuaranteedUpdate(
	ctx context.Context,
	key string,
	destination runtime.Object,
	ignoreNotFound bool,
	preconditions *storage.Preconditions,
	tryUpdate storage.UpdateFunc,
	_ runtime.Object,
) error {
	out, ok := destination.(*v1alpha1.SBOM)
	if !ok {
		return storage.NewInvalidObjError(key, fmt.Sprintf("unexpected out object type: %T", destination))
	}

	name, namespace := extractNameAndNamespace(key)
	if name == "" || namespace == "" {
		return storage.NewInternalErrorf("invalid key: %s", key)
	}

	for {
		tx, err := s.db.BeginTxx(ctx, nil)
		if err != nil {
			return err
		}

		defer func() {
			if err := tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
				log.Printf("failed to rollback transaction: %v", err)
			}
		}()

		query, args, err := sq.Select("*").
			From("sboms").
			Where(sq.Eq{"name": name, "namespace": namespace}).
			ToSql()
		if err != nil {
			return storage.NewInternalError(err.Error())
		}

		sbomRecord := &sbomSchema{}
		err = tx.GetContext(ctx, sbomRecord, query, args...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				if !ignoreNotFound {
					return storage.NewKeyNotFoundError(key, 0)
				}

				*out = v1alpha1.SBOM{}

				return nil
			}
			return err
		}

		currentSBOM := &v1alpha1.SBOM{}
		err = json.Unmarshal(sbomRecord.Object, currentSBOM)
		if err != nil {
			return storage.NewInternalError(err.Error())
		}

		err = preconditions.Check(key, currentSBOM)
		if err != nil {
			return err
		}

		updatedSBOM, _, err := tryUpdate(currentSBOM, storage.ResponseMeta{})
		if err != nil {
			continue
		}

		version, err := s.Versioner().ObjectResourceVersion(currentSBOM)
		if err != nil {
			return storage.NewInternalError(err.Error())
		}
		if err := s.Versioner().UpdateObject(updatedSBOM, version+1); err != nil {
			return storage.NewInternalError(err.Error())
		}

		bytes, err := json.Marshal(updatedSBOM)
		if err != nil {
			return storage.NewInternalError(err.Error())
		}

		query, args, err = sq.Update("sboms").
			Set("object", bytes).
			Where(sq.Eq{"name": name, "namespace": namespace}).
			ToSql()
		if err != nil {
			return storage.NewInternalError(err.Error())
		}

		_, err = tx.ExecContext(ctx, query, args...)
		if err != nil {
			return storage.NewInternalError(err.Error())
		}

		sbom, ok := updatedSBOM.(*v1alpha1.SBOM)
		if !ok {
			return storage.NewInvalidObjError(key, fmt.Sprintf("unexpected updated object type: %T", updatedSBOM))
		}
		*out = *sbom

		if err := tx.Commit(); err != nil {
			return storage.NewInternalError(err.Error())
		}

		if err := s.broadcaster.Action(watch.Modified, updatedSBOM); err != nil {
			return storage.NewInternalError(err.Error())
		}

		break
	}

	return nil
}

// Count returns number of different entries under the key (generally being path prefix).
func (s *sbomStore) Count(key string) (int64, error) {
	namespace := extractNamespace(key)

	queryBuilder := sq.Select("COUNT(*)").From("sboms")
	if namespace != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"namespace": namespace})
	}

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return 0, storage.NewInternalError(err.Error())
	}

	var count int64
	if err := s.db.Get(&count, query, args...); err != nil {
		return 0, storage.NewInternalError(err.Error())
	}

	return count, nil
}

// ReadinessCheck checks if the storage is ready for accepting requests.
func (s *sbomStore) ReadinessCheck() error {
	return nil
}

// RequestWatchProgress requests the a watch stream progress status be sent in the
// watch response stream as soon as possible.
// Used for monitor watch progress even if watching resources with no changes.
//
// If watch is lagging, progress status might:
// * be pointing to stale resource version. Use etcd KV request to get linearizable resource version.
// * not be delivered at all. It's recommended to poll request progress periodically.
//
// Note: Only watches with matching context grpc metadata will be notified.
// https://github.com/kubernetes/kubernetes/blob/9325a57125e8502941d1b0c7379c4bb80a678d5c/vendor/go.etcd.io/etcd/client/v3/watch.go#L1037-L1042
//
// TODO: Remove when storage.Interface will be separate from etc3.store.
// Deprecated: Added temporarily to simplify exposing RequestProgress for watch cache.
func (s *sbomStore) RequestWatchProgress(_ context.Context) error {
	// As this is a deprecated method, we are not implementing it.
	return nil
}

// extractNameAndNamespace extracts the name and namespace from the key.
// Used for single object operations.
// Key format: /storage.sbombastic.rancher.io/<resource>/<namespace>/<name>
func extractNameAndNamespace(key string) (string, string) {
	key = strings.TrimPrefix(key, "/")
	parts := strings.Split(key, "/")
	if len(parts) == 4 {
		return parts[3], parts[2]
	}

	return "", ""
}

// extractNamespace extracts the namespace from the key.
// Used for list operations.
// Key format: /storage.sbombastic.rancher.io/<resource>/<namespace>
func extractNamespace(key string) string {
	key = strings.TrimPrefix(key, "/")
	parts := strings.Split(key, "/")

	if len(parts) == 3 {
		return parts[2]
	}

	return ""
}
