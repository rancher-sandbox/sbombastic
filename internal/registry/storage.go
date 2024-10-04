package registry

import (
	"context"
	"errors"
	"fmt"
	"log"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"

	"github.com/rancher/sbombastic/api/storage/v1alpha1"
)

var _ storage.Interface = &MyStorage{}

var store *v1alpha1.ScanResult

// MyStorage implements the Interface and logs calls to each method.
type MyStorage struct{}

// Versioner returns the versioner associated with the interface
func (s *MyStorage) Versioner() storage.Versioner {
	log.Println("Versioner() called")
	return storage.APIObjectVersioner{}
}

// Create logs the creation of an object
func (s *MyStorage) Create(ctx context.Context, key string, obj, out runtime.Object, ttl uint64) error {
	log.Printf("Create() called: key=%s, ttl=%d\n %v %v", key, ttl, obj, out)
	// resourceVersion should not be set on create
	if version, err := s.Versioner().ObjectResourceVersion(obj); err == nil && version != 0 {
		msg := "resourceVersion should not be set on objects to be created"
		log.Println(msg)
		return errors.New(msg)
	}
	store = obj.(*v1alpha1.ScanResult)
	store.DeepCopyInto(out.(*v1alpha1.ScanResult))
	return nil
}

func preprocess(obj *v1alpha1.ScanResult) *v1alpha1.ScanResult {
	return obj
}

// Delete logs the deletion of an object by key
func (s *MyStorage) Delete(
	ctx context.Context, key string, out runtime.Object, preconditions *storage.Preconditions,
	validateDeletion storage.ValidateObjectFunc, cachedExistingObject runtime.Object,
) error {
	log.Printf("Delete() called: key=%s\n", key)
	return nil
}

// Watch logs the start of a watch on a key
func (s *MyStorage) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	log.Printf("Watch() called: key=%s\n", key)
	return watch.NewEmptyWatch(), nil
}

// Get logs the retrieval of an object by key
func (s *MyStorage) Get(ctx context.Context, key string, opts storage.GetOptions, objPtr runtime.Object) error {
	log.Printf("Get() called: key=%s opts=%v\n", key, opts)
	if store == nil {
		return storage.NewKeyNotFoundError(key, 0)
	}
	// Type assert objPtr to the specific type (e.g., *v1alpha1.ScanResult)
	if result, ok := objPtr.(*v1alpha1.ScanResult); ok {
		store.DeepCopyInto(result)
	} else {
		return fmt.Errorf("expected objPtr to be *v1alpha1.ScanResult, got %T", objPtr)
	}

	return nil
}

// GetList logs the retrieval of a list of objects
func (s *MyStorage) GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	log.Printf("GetList() called: key=%s\n", key)
	return nil
}

// GuaranteedUpdate logs the update of an object with retry logic
func (s *MyStorage) GuaranteedUpdate(
	ctx context.Context, key string, destination runtime.Object, ignoreNotFound bool,
	preconditions *storage.Preconditions, tryUpdate storage.UpdateFunc, cachedExistingObject runtime.Object,
) error {
	log.Printf("GuaranteedUpdate() called: key=%s, ignoreNotFound=%v\n", key, ignoreNotFound)
	return nil
}

// Count logs the counting of entries under a key
func (s *MyStorage) Count(key string) (int64, error) {
	log.Printf("Count() called: key=%s\n", key)
	return 1, nil
}

// ReadinessCheck logs the readiness check
func (s *MyStorage) ReadinessCheck() error {
	log.Println("ReadinessCheck() called")
	return nil
}

// RequestWatchProgress logs a request to watch progress
func (s *MyStorage) RequestWatchProgress(ctx context.Context) error {
	log.Println("RequestWatchProgress() called")
	return nil
}
