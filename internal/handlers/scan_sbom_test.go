package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/owenrumney/go-sarif/v2/sarif"
	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/internal/messaging"
	"github.com/rancher/sbombastic/pkg/generated/clientset/versioned/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestScanSBOMHandler_Handle(t *testing.T) {
	spdxPath := filepath.Join("..", "..", "test", "fixtures", "golang-1.12-alpine.spdx.json")
	spdxData, err := os.ReadFile(spdxPath)
	require.NoError(t, err)

	sbom := &storagev1alpha1.SBOM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sbom",
			Namespace: "default",
		},
		Spec: storagev1alpha1.SBOMSpec{
			SPDX: runtime.RawExtension{Raw: spdxData},
		},
	}

	scheme := scheme.Scheme
	err = storagev1alpha1.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(sbom).
		Build()

	reportPath := filepath.Join("..", "..", "test", "fixtures", "golang-1.12-alpine.sarif.json")
	reportData, err := os.ReadFile(reportPath)
	require.NoError(t, err)

	expectedReport := &sarif.Report{}
	err = json.Unmarshal(reportData, expectedReport)
	require.NoError(t, err)

	handler := NewScanSBOMHandler(k8sClient, scheme, "/tmp", slog.Default())

	err = handler.Handle(&messaging.ScanSBOM{
		SBOMName:      sbom.Name,
		SBOMNamespace: sbom.Namespace,
	})
	require.NoError(t, err)

	vulnerabilityReport := &storagev1alpha1.VulnerabilityReport{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{
		Name:      sbom.Name,
		Namespace: sbom.Namespace,
	}, vulnerabilityReport)
	require.NoError(t, err)

	assert.Equal(t, sbom.GetImageMetadata(), vulnerabilityReport.GetImageMetadata())
	assert.Equal(t, sbom.UID, vulnerabilityReport.GetOwnerReferences()[0].UID)

	report := &sarif.Report{}
	err = json.Unmarshal(vulnerabilityReport.Spec.SARIF.Raw, report)
	require.NoError(t, err)

	// Filter out fields containing the file path from the comparison
	filter := cmp.FilterPath(func(path cmp.Path) bool {
		lastField := path.Last().String()
		return lastField == ".URI" || lastField == ".Text"
	}, cmp.Comparer(func(a, b *string) bool {
		if strings.Contains(*a, ".json") && strings.Contains(*b, ".json") {
			return true
		}

		return cmp.Equal(a, b)
	}))
	diff := cmp.Diff(expectedReport, report, filter)

	assert.Empty(t, diff)
}
