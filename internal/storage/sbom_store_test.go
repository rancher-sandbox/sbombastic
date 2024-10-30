package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/utils/ptr"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/rancher/sbombastic/api/storage/v1alpha1"
)

const keyPrefix = "/storage.sbombastic.rancher.io/sboms"

type sbomStoreTestSuite struct {
	suite.Suite
	store       *sbomStore
	db          *sqlx.DB
	broadcaster *watch.Broadcaster
}

func (suite *sbomStoreTestSuite) SetupTest() {
	suite.db = sqlx.MustConnect("sqlite", ":memory:")

	suite.db.MustExec(CreateSBOMTableSQL)

	suite.broadcaster = watch.NewBroadcaster(1000, watch.WaitIfChannelFull)
	suite.store = &sbomStore{
		broadcaster: suite.broadcaster,
		db:          suite.db,
	}
}

func (suite *sbomStoreTestSuite) TearDownTest() {
	suite.db.Close()
	suite.broadcaster.Shutdown()
}

func TestSBOMStoreTestSuite(t *testing.T) {
	suite.Run(t, &sbomStoreTestSuite{})
}

func (suite *sbomStoreTestSuite) TestCreate() {
	sbom := &v1alpha1.SBOM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	key := keyPrefix + "/default/test"
	out := &v1alpha1.SBOM{}
	err := suite.store.Create(context.Background(), key, sbom, out, 0)
	suite.Require().NoError(err)

	suite.EqualValues(sbom, out)
	suite.Equal("1", out.ResourceVersion)
}

func (suite *sbomStoreTestSuite) TestDelete() {
	sbom := &v1alpha1.SBOM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	key := keyPrefix + "/default/test"

	tests := []struct {
		name             string
		preconditions    *storage.Preconditions
		validateDeletion storage.ValidateObjectFunc
		expectedError    error
	}{
		{
			name:          "happy path",
			preconditions: &storage.Preconditions{},
			validateDeletion: func(_ context.Context, _ runtime.Object) error {
				return nil
			},
			expectedError: nil,
		},
		{
			name:          "deletion fails with incorrect UID precondition",
			preconditions: &storage.Preconditions{UID: ptr.To(types.UID("incorrect-uid"))},
			validateDeletion: func(_ context.Context, _ runtime.Object) error {
				return nil
			},
			expectedError: storage.NewInvalidObjError(key, "Precondition failed: UID in precondition: incorrect-uid, UID in object meta: "),
		},
	}

	for _, test := range tests {
		suite.Run(test.name, func() {
			err := suite.store.Create(context.Background(), key, sbom, &v1alpha1.SBOM{}, 0)
			suite.Require().NoError(err)

			out := &v1alpha1.SBOM{}
			err = suite.store.Delete(context.Background(), key, out, test.preconditions, test.validateDeletion, nil)

			if test.expectedError != nil {
				suite.Require().Error(err)
				suite.Equal(test.expectedError.Error(), err.Error())
			} else {
				suite.Require().NoError(err)
				suite.Equal(sbom, out)

				err = suite.store.Get(context.Background(), key, storage.GetOptions{}, &v1alpha1.SBOM{})
				suite.True(storage.IsNotFound(err))
			}
		})
	}
}

func (suite *sbomStoreTestSuite) TestWatchEmptyResourceVersion() {
	key := keyPrefix + "/default/test"
	opts := storage.ListOptions{ResourceVersion: ""}

	watcher, err := suite.store.Watch(context.Background(), key, opts)
	suite.Require().NoError(err)

	suite.broadcaster.Shutdown()

	events := collectEvents(watcher)
	suite.Require().Empty(events)
}

func (suite *sbomStoreTestSuite) TestWatchResourceVersionZero() {
	key := keyPrefix + "/default/test"
	sbom := &v1alpha1.SBOM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}
	err := suite.store.Create(context.Background(), key, sbom, &v1alpha1.SBOM{}, 0)
	suite.Require().NoError(err)

	opts := storage.ListOptions{ResourceVersion: "0"}

	watcher, err := suite.store.Watch(context.Background(), key, opts)
	suite.Require().NoError(err)

	validateDeletion := func(_ context.Context, _ runtime.Object) error {
		return nil
	}
	err = suite.store.Delete(context.Background(), key, &v1alpha1.SBOM{}, &storage.Preconditions{}, validateDeletion, nil)
	suite.Require().NoError(err)

	suite.broadcaster.Shutdown()

	events := collectEvents(watcher)
	suite.Require().Len(events, 2)
	suite.Equal(watch.Added, events[0].Type)
	suite.Equal(sbom, events[0].Object)
	suite.Equal(watch.Deleted, events[1].Type)
	suite.Equal(sbom, events[1].Object)
}

func (suite *sbomStoreTestSuite) TestWatchSpecificResourceVersion() {
	key := keyPrefix + "/default"
	sbom := &v1alpha1.SBOM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}
	suite.Require().NoError(suite.store.Create(context.Background(), key+"/test", sbom, &v1alpha1.SBOM{}, 0))

	opts := storage.ListOptions{ResourceVersion: "1"}

	watcher, err := suite.store.Watch(context.Background(), key, opts)
	suite.Require().NoError(err)

	tryUpdate := func(input runtime.Object, _ storage.ResponseMeta) (runtime.Object, *uint64, error) {
		return input, ptr.To(uint64(0)), nil
	}
	updatedSBOM := &v1alpha1.SBOM{}
	err = suite.store.GuaranteedUpdate(context.Background(), key+"/test", updatedSBOM, false, &storage.Preconditions{}, tryUpdate, nil)
	suite.Require().NoError(err)

	suite.broadcaster.Shutdown()

	events := collectEvents(watcher)
	suite.Require().Len(events, 2)
	suite.Equal(watch.Added, events[0].Type)
	suite.Equal(sbom, events[0].Object)
	suite.Equal(watch.Modified, events[1].Type)
	suite.Equal(updatedSBOM, events[1].Object)
}

func (suite *sbomStoreTestSuite) TestWatchWithLabelSelector() {
	key := keyPrefix + "/default"
	sbom1 := &v1alpha1.SBOM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1",
			Namespace: "default",
			Labels: map[string]string{
				"sbombastic.rancher.io/test": "true",
			},
		},
	}
	err := suite.store.Create(context.Background(), key+"/test1", sbom1, &v1alpha1.SBOM{}, 0)
	suite.Require().NoError(err)

	sbom2 := &v1alpha1.SBOM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test2",
			Namespace: "default",
			Labels:    map[string]string{},
		},
	}
	err = suite.store.Create(context.Background(), key+"/test2", sbom2, &v1alpha1.SBOM{}, 0)
	suite.Require().NoError(err)

	opts := storage.ListOptions{
		ResourceVersion: "1",
		Predicate: storage.SelectionPredicate{
			Label: labels.SelectorFromSet(labels.Set{
				"sbombastic.rancher.io/test": "true",
			}),
		},
	}
	watcher, err := suite.store.Watch(context.Background(), key, opts)
	suite.Require().NoError(err)

	suite.broadcaster.Shutdown()

	events := collectEvents(watcher)
	suite.Require().Len(events, 1)
	suite.Equal(watch.Added, events[0].Type)
	suite.Equal(sbom1, events[0].Object)
}

// collectEvents reads events from the watcher and returns them in a slice.
func collectEvents(watcher watch.Interface) []watch.Event {
	var events []watch.Event
	for event := range watcher.ResultChan() {
		events = append(events, event)
	}
	return events
}

func (suite *sbomStoreTestSuite) TestGuaranteedUpdate() {
	sbom := &v1alpha1.SBOM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: v1alpha1.SBOMSpec{
			Data: runtime.RawExtension{
				Raw: []byte("{}"),
			},
		},
	}
	err := suite.store.Create(context.Background(), keyPrefix+"/default/test", sbom, &v1alpha1.SBOM{}, 0)
	suite.Require().NoError(err)

	tests := []struct {
		name                string
		key                 string
		ignoreNotFound      bool
		preconditions       *storage.Preconditions
		expectedUpdatedSBOM *v1alpha1.SBOM
		expectedError       error
	}{
		{
			name:          "happy path",
			key:           keyPrefix + "/default/test",
			preconditions: &storage.Preconditions{},
			expectedUpdatedSBOM: &v1alpha1.SBOM{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test",
					Namespace:       "default",
					UID:             "test-uid",
					ResourceVersion: "2",
				},
				Spec: v1alpha1.SBOMSpec{
					Data: runtime.RawExtension{
						Raw: []byte(`{"foo": "bar"}`),
					},
				},
			},
		},
		{
			name: "preconditions failed",
			key:  keyPrefix + "/default/test",
			preconditions: &storage.Preconditions{
				UID: ptr.To(types.UID("incorrect-uid")),
			},

			expectedError: storage.NewInvalidObjError(keyPrefix+"/default/test",
				"Precondition failed: UID in precondition: incorrect-uid, UID in object meta: test-uid"),
		},
		{
			name:          "not found",
			key:           keyPrefix + "/default/notfound",
			preconditions: &storage.Preconditions{},
			expectedError: storage.NewKeyNotFoundError(keyPrefix+"/default/notfound", 0),
		},
		{
			name:                "not found with ignore not found",
			key:                 keyPrefix + "/default/notfound",
			preconditions:       &storage.Preconditions{},
			ignoreNotFound:      true,
			expectedUpdatedSBOM: &v1alpha1.SBOM{},
		},
	}

	for _, test := range tests {
		suite.Run(test.name, func() {
			tryUpdate := func(input runtime.Object, _ storage.ResponseMeta) (runtime.Object, *uint64, error) {
				input.(*v1alpha1.SBOM).Spec.Data.Raw = []byte(`{"foo": "bar"}`)

				return input, ptr.To(uint64(0)), nil
			}
			updatedSBOM := &v1alpha1.SBOM{}
			err := suite.store.GuaranteedUpdate(context.Background(), test.key, updatedSBOM, test.ignoreNotFound, test.preconditions, tryUpdate, nil)

			if test.expectedError != nil {
				suite.Require().Error(err)
				suite.Require().Equal(test.expectedError.Error(), err.Error())
			} else {
				suite.Require().NoError(err)
				suite.Require().Equal(test.expectedUpdatedSBOM, updatedSBOM)
			}
		})
	}
}

func (suite *sbomStoreTestSuite) TestCount() {
	err := suite.store.Create(context.Background(), keyPrefix+"/default/test1", &v1alpha1.SBOM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1",
			Namespace: "default",
		},
	}, &v1alpha1.SBOM{}, 0)
	suite.Require().NoError(err)

	err = suite.store.Create(context.Background(), keyPrefix+"/default/test2", &v1alpha1.SBOM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test2",
			Namespace: "default",
		},
	}, &v1alpha1.SBOM{}, 0)
	suite.Require().NoError(err)

	err = suite.store.Create(context.Background(), keyPrefix+"/other/test4", &v1alpha1.SBOM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test4",
			Namespace: "other",
		},
	}, &v1alpha1.SBOM{}, 0)
	suite.Require().NoError(err)

	tests := []struct {
		name          string
		key           string
		expectedCount int64
	}{
		{
			name:          "count entries in default namespace",
			key:           keyPrefix + "/default",
			expectedCount: 2,
		},
		{
			name:          "count all entries",
			key:           keyPrefix,
			expectedCount: 3,
		},
	}

	for _, test := range tests {
		suite.Run(test.name, func() {
			count, err := suite.store.Count(test.key)
			suite.Require().NoError(err)
			suite.Require().Equal(test.expectedCount, count)
		})
	}
}
