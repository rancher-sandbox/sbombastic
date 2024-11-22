//nolint:wrapcheck // We want to return the errors from k8s.io/apiserver/pkg/storage as they are.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/storage"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
)

// objectSchema is the schema of an object in the database.
// Note: the struct fields must be exported in order to work.
type objectSchema struct {
	ID        int    `db:"id"`
	Name      string `db:"name"`
	Namespace string `db:"namespace"`
	Object    []byte `db:"object"`
}

var _ storage.Interface = &store{}

type store struct {
	db          *sqlx.DB
	broadcaster *watch.Broadcaster
	table       string
	newFunc     func() runtime.Object
	newListFunc func() runtime.Object
	logger      *slog.Logger
}

// Returns Versioner associated with this interface.
func (s *store) Versioner() storage.Versioner {
	return storage.APIObjectVersioner{}
}

// Create adds a new object at a key unless it already exists. 'ttl' is time-to-live
// in seconds (0 means forever). If no error is returned and out is not nil, out will be
// set to the read value from database.
func (s *store) Create(ctx context.Context, key string, obj, out runtime.Object, _ uint64) error {
	s.logger.DebugContext(ctx, "Creating object", "key", key, "object", obj)

	name, namespace := extractNameAndNamespace(key)
	if name == "" || namespace == "" {
		return storage.NewInternalErrorf("invalid key: %s", key)
	}

	if err := s.Versioner().UpdateObject(obj, 1); err != nil {
		return storage.NewInternalError(err.Error())
	}

	bytes, err := json.Marshal(obj)
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	query, args, err := sq.Insert(s.table).
		Columns("name", "namespace", "object").
		Values(name, namespace, bytes).
		Suffix("ON CONFLICT DO NOTHING").
		ToSql()
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	if rowsAffected == 0 {
		return storage.NewKeyExistsError(key, 0)
	}

	if err := s.broadcaster.Action(watch.Added, obj); err != nil {
		return storage.NewInternalError(err.Error())
	}

	if out != nil {
		if err := setValue(obj, out); err != nil {
			return err
		}
	}

	return nil
}

// Delete removes the specified key and returns the value that existed at that spot.
// If key didn't exist, it will return NotFound storage error.
// If 'cachedExistingObject' is non-nil, it can be used as a suggestion about the
// current version of the object to avoid read operation from storage to get it.
// However, the implementations have to retry in case suggestion is stale.
func (s *store) Delete(
	ctx context.Context, key string, out runtime.Object, preconditions *storage.Preconditions,
	validateDeletion storage.ValidateObjectFunc, _ runtime.Object,
) error {
	s.logger.DebugContext(ctx, "Deleting object", "key", key)

	name, namespace := extractNameAndNamespace(key)
	if name == "" || namespace == "" {
		return storage.NewInternalErrorf("invalid key: %s", key)
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return storage.NewInternalError(err.Error())
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			s.logger.ErrorContext(ctx, "failed to rollback transaction", "error", err)
		}
	}()

	query, args, err := sq.Delete(s.table).
		Where(sq.Eq{"name": name, "namespace": namespace}).
		Suffix("RETURNING *").
		ToSql()
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	objectRecord := &objectSchema{}
	if err := tx.GetContext(ctx, objectRecord, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.NewKeyNotFoundError(key, 0)
		}
		return storage.NewInternalError(err.Error())
	}

	if err := json.Unmarshal(objectRecord.Object, out); err != nil {
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
func (s *store) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	s.logger.DebugContext(ctx, "Watching object", "key", key, "options", opts)

	if opts.ResourceVersion == "" {
		return s.broadcaster.Watch()
	}

	if opts.ResourceVersion == "0" {
		obj := s.newFunc()
		if err := s.Get(ctx, key, storage.GetOptions{}, obj); err != nil {
			return nil, err
		}

		return s.broadcaster.WatchWithPrefix([]watch.Event{{Type: watch.Added, Object: obj}})
	}

	listObj := s.newListFunc()
	if err := s.GetList(ctx, key, opts, listObj); err != nil {
		return nil, err
	}

	itemsValue, err := getItems(listObj)
	if err != nil {
		return nil, err
	}

	var events []watch.Event
	for i := range itemsValue.Len() {
		// Cast the item address to a runtime.Object
		item, ok := itemsValue.Index(i).Addr().Interface().(runtime.Object)
		if !ok {
			return nil, storage.NewInternalErrorf("unexpected item type: %T", itemsValue.Index(i).Addr().Interface())
		}

		events = append(events, watch.Event{
			Type:   watch.Added,
			Object: item,
		})
	}

	return s.broadcaster.WatchWithPrefix(events)
}

// Get unmarshals object found at key into objPtr. On a not found error, will either
// return a zero object of the requested type, or an error, depending on 'opts.ignoreNotFound'.
// Treats empty responses and nil response nodes exactly like a not found error.
// The returned contents may be delayed, but it is guaranteed that they will
// match 'opts.ResourceVersion' according 'opts.ResourceVersionMatch'.
func (s *store) Get(ctx context.Context, key string, opts storage.GetOptions, objPtr runtime.Object) error {
	s.logger.DebugContext(ctx, "Getting object", "key", key, "options", opts)

	name, namespace := extractNameAndNamespace(key)
	if name == "" || namespace == "" {
		return storage.NewInternalErrorf("invalid key: %s", key)
	}

	if err := runtime.SetZeroValue(objPtr); err != nil {
		return storage.NewInternalErrorf("unable to set objPtr zero value: %v", err)
	}

	query, args, err := sq.Select("*").
		From(s.table).
		Where(sq.Eq{"name": name, "namespace": namespace}).
		ToSql()
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	objectRecord := &objectSchema{}
	if err := s.db.GetContext(ctx, objectRecord, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if opts.IgnoreNotFound {
				return nil
			}

			return storage.NewKeyNotFoundError(key, 0)
		}
		return storage.NewInternalError(err.Error())
	}

	err = json.Unmarshal(objectRecord.Object, objPtr)
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	return nil
}

// GetList unmarshalls objects found at key into a *List api object (an object
// that satisfies runtime.IsList definition).
// The returned contents may be delayed, but it is guaranteed that they will
// match 'opts.ResourceVersion' according 'opts.ResourceVersionMatch'.
func (s *store) GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	s.logger.DebugContext(ctx, "Getting list", "key", key, "options", opts)

	queryBuilder := sq.Select("*").From(s.table)
	namespace := extractNamespace(key)
	if namespace != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"namespace": namespace})
	}
	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	var objectRecords []objectSchema
	if err := s.db.SelectContext(ctx, &objectRecords, query, args...); err != nil {
		return storage.NewInternalError(err.Error())
	}

	itemsValue, err := getItems(listObj)
	if err != nil {
		return err
	}

	for _, objectRecord := range objectRecords {
		obj := s.newFunc()
		if err := json.Unmarshal(objectRecord.Object, obj); err != nil {
			return storage.NewInternalError(err.Error())
		}

		ok, err := opts.Predicate.Matches(obj)
		if err != nil {
			return storage.NewInternalError(err.Error())
		}
		if !ok {
			continue
		}

		// Append the object to the items slice
		itemsValue.Set(reflect.Append(itemsValue, reflect.ValueOf(obj).Elem()))
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
func (s *store) GuaranteedUpdate(
	ctx context.Context,
	key string,
	destination runtime.Object,
	ignoreNotFound bool,
	preconditions *storage.Preconditions,
	tryUpdate storage.UpdateFunc,
	_ runtime.Object,
) error {
	s.logger.DebugContext(ctx, "Guaranteed update", "key", key)

	name, namespace := extractNameAndNamespace(key)
	if name == "" || namespace == "" {
		return storage.NewInternalErrorf("invalid key: %s", key)
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			s.logger.ErrorContext(ctx, "failed to rollback transaction", "error", err)
		}
	}()

	for {
		query, args, err := sq.Select("*").
			From(s.table).
			Where(sq.Eq{"name": name, "namespace": namespace}).
			ToSql()
		if err != nil {
			return storage.NewInternalError(err.Error())
		}

		if err := runtime.SetZeroValue(destination); err != nil {
			return storage.NewInternalErrorf("unable to set destination to zero value: %v", err)
		}

		objectRecord := &objectSchema{}
		err = tx.GetContext(ctx, objectRecord, query, args...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				if !ignoreNotFound {
					return storage.NewKeyNotFoundError(key, 0)
				}

				return nil
			}
			return err
		}

		obj := s.newFunc()
		err = json.Unmarshal(objectRecord.Object, obj)
		if err != nil {
			return storage.NewInternalError(err.Error())
		}

		err = preconditions.Check(key, obj)
		if err != nil {
			return err
		}

		updatedObj, _, err := tryUpdate(obj, storage.ResponseMeta{})
		if err != nil {
			if apierrors.IsConflict(err) && strings.Contains(err.Error(), registry.OptimisticLockErrorMsg) {
				s.logger.DebugContext(ctx, "Optimistic lock conflict", "key", key, "error", err)

				// retry update on optimistic lock conflict
				continue
			}

			return err
		}

		version, err := s.Versioner().ObjectResourceVersion(obj)
		if err != nil {
			return storage.NewInternalError(err.Error())
		}
		if err := s.Versioner().UpdateObject(updatedObj, version+1); err != nil {
			return storage.NewInternalError(err.Error())
		}

		bytes, err := json.Marshal(destination)
		if err != nil {
			return storage.NewInternalError(err.Error())
		}

		query, args, err = sq.Update(s.table).
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

		if err := tx.Commit(); err != nil {
			return storage.NewInternalError(err.Error())
		}

		if err := s.broadcaster.Action(watch.Modified, updatedObj); err != nil {
			return storage.NewInternalError(err.Error())
		}

		if err := setValue(updatedObj, destination); err != nil {
			return err
		}

		break
	}

	return nil
}

// Count returns number of different entries under the key (generally being path prefix).
func (s *store) Count(key string) (int64, error) {
	s.logger.Debug("Counting objects", "key", key)

	namespace := extractNamespace(key)

	queryBuilder := sq.Select("COUNT(*)").From(s.table)
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
func (s *store) ReadinessCheck() error {
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
func (s *store) RequestWatchProgress(_ context.Context) error {
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

// setValue sets the value of 'dest' to the value of 'source' after converting them to pointers.
func setValue(source, dest runtime.Object) error {
	destValue, err := conversion.EnforcePtr(dest)
	if err != nil {
		return storage.NewInternalErrorf("unable to convert destination object to pointer: %v", err)
	}

	sourceValue, err := conversion.EnforcePtr(source)
	if err != nil {
		return storage.NewInternalErrorf("unable to convert source object to pointer: %v", err)
	}

	destValue.Set(sourceValue)
	return nil
}

// getItems retrieves the items slice pointer from the provided ObjectList.
func getItems(listObj runtime.Object) (reflect.Value, error) {
	// Access the items field of the list object using reflection
	itemsPtr, err := meta.GetItemsPtr(listObj)
	if err != nil {
		return reflect.Value{}, storage.NewInternalErrorf("unable to get items pointer: %v", err)
	}

	itemsValue, err := conversion.EnforcePtr(itemsPtr)
	if err != nil || itemsValue.Kind() != reflect.Slice {
		return reflect.Value{}, storage.NewInternalErrorf("need pointer to slice: %v", err)
	}

	return itemsValue, nil
}
