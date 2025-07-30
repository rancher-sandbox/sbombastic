package v1alpha1

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	sbombasticv1alpha1 "github.com/rancher/sbombastic/api/v1alpha1"
)

func TestScanJobDefaulter_Default(t *testing.T) {
	scanJob := &sbombasticv1alpha1.ScanJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scan-job",
			Namespace: "default",
		},
		Spec: sbombasticv1alpha1.ScanJobSpec{
			Registry: "registry.example.com",
		},
	}

	defaulter := &ScanJobCustomDefaulter{}

	err := defaulter.Default(t.Context(), scanJob)
	require.NoError(t, err)

	timestampStr := scanJob.Annotations[sbombasticv1alpha1.CreationTimestampAnnotation]
	assert.NotEmpty(t, timestampStr)

	_, err = time.Parse(time.RFC3339Nano, timestampStr)
	require.NoError(t, err)
}

func TestScanJobCustomValidator_ValidateCreate(t *testing.T) {
	tests := []struct {
		name            string
		existingScanJob *sbombasticv1alpha1.ScanJob
		scanJob         *sbombasticv1alpha1.ScanJob
		expectedError   string
		expectedField   string
	}{
		{
			name:            "should admit creation when no existing jobs with same registry",
			existingScanJob: nil,
			scanJob: &sbombasticv1alpha1.ScanJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scan-job",
					Namespace: "default",
				},
				Spec: sbombasticv1alpha1.ScanJobSpec{
					Registry: "registry.example.com",
				},
			},
		},
		{
			name: "should deny creation when existing job with same registry is pending",
			existingScanJob: func() *sbombasticv1alpha1.ScanJob {
				job := &sbombasticv1alpha1.ScanJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-job",
						Namespace: "default",
					},
					Spec: sbombasticv1alpha1.ScanJobSpec{
						Registry: "registry.example.com",
					},
				}
				job.InitializeConditions()
				return job
			}(),
			scanJob: &sbombasticv1alpha1.ScanJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scan-job",
					Namespace: "default",
				},
				Spec: sbombasticv1alpha1.ScanJobSpec{
					Registry: "registry.example.com",
				},
			},
			expectedField: "spec.registry",
			expectedError: "is already running",
		},
		{
			name: "should deny creation when existing job with same registry is in progress",
			existingScanJob: func() *sbombasticv1alpha1.ScanJob {
				job := &sbombasticv1alpha1.ScanJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-job",
						Namespace: "default",
					},
					Spec: sbombasticv1alpha1.ScanJobSpec{
						Registry: "registry.example.com",
					},
				}
				job.InitializeConditions()
				job.MarkInProgress(sbombasticv1alpha1.ReasonImageScanInProgress, "Image scan in progress")
				return job
			}(),
			scanJob: &sbombasticv1alpha1.ScanJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scan-job",
					Namespace: "default",
				},
				Spec: sbombasticv1alpha1.ScanJobSpec{
					Registry: "registry.example.com",
				},
			},
			expectedField: "spec.registry",
			expectedError: "is already running",
		},
		{
			name: "should admit creation when existing job with same registry is completed",
			existingScanJob: func() *sbombasticv1alpha1.ScanJob {
				job := &sbombasticv1alpha1.ScanJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-job",
						Namespace: "default",
					},
					Spec: sbombasticv1alpha1.ScanJobSpec{
						Registry: "registry.example.com",
					},
				}
				job.InitializeConditions()
				job.MarkComplete(sbombasticv1alpha1.ReasonAllImagesScanned, "Done")
				return job
			}(),
			scanJob: &sbombasticv1alpha1.ScanJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scan-job",
					Namespace: "default",
				},
				Spec: sbombasticv1alpha1.ScanJobSpec{
					Registry: "registry.example.com",
				},
			},
		},
		{
			name: "should admit creation when existing job with same registry failed",
			existingScanJob: func() *sbombasticv1alpha1.ScanJob {
				job := &sbombasticv1alpha1.ScanJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-job",
						Namespace: "default",
					},
					Spec: sbombasticv1alpha1.ScanJobSpec{
						Registry: "registry.example.com",
					},
				}
				job.InitializeConditions()
				job.MarkFailed(sbombasticv1alpha1.ReasonInternalError, "Failed")
				return job
			}(),
			scanJob: &sbombasticv1alpha1.ScanJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scan-job",
					Namespace: "default",
				},
				Spec: sbombasticv1alpha1.ScanJobSpec{
					Registry: "registry.example.com",
				},
			},
		},
		{
			name: "should deny creation when name exceeds max length",
			scanJob: &sbombasticv1alpha1.ScanJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "this-name-is-way-too-long-and-exceeds-the-sixty-three-character-limit-that-is-set-for-kubernetes-labels",
					Namespace: "default",
				},
				Spec: sbombasticv1alpha1.ScanJobSpec{
					Registry: "registry.example.com",
				},
			},
			expectedField: "metadata.name",
			expectedError: "Too long: may not be more than 63 bytes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, sbombasticv1alpha1.AddToScheme(scheme))

			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			validator := ScanJobCustomValidator{client: client}

			if test.existingScanJob != nil {
				require.NoError(t, client.Create(t.Context(), test.existingScanJob))
			}

			warnings, err := validator.ValidateCreate(t.Context(), test.scanJob)

			if test.expectedError != "" {
				require.Error(t, err)
				statusErr, ok := err.(interface{ Status() metav1.Status })
				require.True(t, ok)
				details := statusErr.Status().Details
				require.NotNil(t, details)
				require.Len(t, details.Causes, 1)
				assert.Equal(t, test.expectedField, details.Causes[0].Field)
				assert.Contains(t, details.Causes[0].Message, test.expectedError)
			} else {
				require.NoError(t, err)
			}

			assert.Empty(t, warnings)
		})
	}
}

func TestScanJobCustomValidator_ValidateUpdate(t *testing.T) {
	oldObj := &sbombasticv1alpha1.ScanJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scan-job",
			Namespace: "default",
		},
		Spec: sbombasticv1alpha1.ScanJobSpec{
			Registry: "registry.example.com",
		},
	}

	newObj := &sbombasticv1alpha1.ScanJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scan-job",
			Namespace: "default",
		},
		Spec: sbombasticv1alpha1.ScanJobSpec{
			Registry: "new-registry.example.com",
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, sbombasticv1alpha1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	validator := ScanJobCustomValidator{client: client}

	warnings, err := validator.ValidateUpdate(t.Context(), oldObj, newObj)

	require.Error(t, err)
	statusErr, ok := err.(interface{ Status() metav1.Status })
	require.True(t, ok)
	details := statusErr.Status().Details
	require.NotNil(t, details)
	require.Len(t, details.Causes, 1)
	assert.Equal(t, "spec.registry", details.Causes[0].Field)
	assert.Contains(t, details.Causes[0].Message, "immutable")

	assert.Empty(t, warnings)
}
