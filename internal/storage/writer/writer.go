// This file defines a generic TableWriter interface to CRUD the PostgreSQL tables
// ImageWriter, SBOMWriter, and VulnerabilityReportWriter implement this interface.

package writer

import (
	"context"
)

// TableWriter is the minimal contract that the generic store requires
// in order to persist an object. Implementations map the call to a
// specific sqlc‑generated query for a concrete table.
type TableWriter interface {
	Create(ctx context.Context, name, namespace string, raw []byte) (int64, error)
	Delete(ctx context.Context, name, namespace string) ([]byte, error)
	Get(ctx context.Context, name, namespace string) (string, error)
	List(ctx context.Context, namespace string) ([]string, error)
	Update(ctx context.Context, name, namespace string, raw []byte) (int64, error)
	Count(ctx context.Context, namespace string) (int64, error)
}
