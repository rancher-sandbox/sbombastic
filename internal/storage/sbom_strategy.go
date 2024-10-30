package storage

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/names"

	"github.com/rancher/sbombastic/api/storage/v1alpha1"
)

// NewStrategy creates and returns a sbomStrategy instance
func NewStrategy(typer runtime.ObjectTyper) sbomStrategy { //nolint:revive // Unexported return is fine here
	return sbomStrategy{typer, names.SimpleNameGenerator}
}

// GetAttrs returns labels.Set, fields.Set, and error in case the given runtime.Object is not a SBOM
func GetAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	apiserver, ok := obj.(*v1alpha1.SBOM)
	if !ok {
		return nil, nil, fmt.Errorf("given object is not a SBOM, got: %T", obj)
	}
	return labels.Set(apiserver.ObjectMeta.Labels), SelectableFields(apiserver), nil
}

// MatchSBOM is the filter used by the generic etcd backend to watch events
// from etcd to clients of the apiserver only interested in specific labels/fields.
func MatchSBOM(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{
		Label:    label,
		Field:    field,
		GetAttrs: GetAttrs,
	}
}

// SelectableFields returns a field set that represents the object.
func SelectableFields(obj *v1alpha1.SBOM) fields.Set {
	return generic.ObjectMetaFieldsSet(&obj.ObjectMeta, true)
}

type sbomStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

func (sbomStrategy) NamespaceScoped() bool {
	return true
}

func (sbomStrategy) PrepareForCreate(_ context.Context, _ runtime.Object) {
}

func (sbomStrategy) PrepareForUpdate(_ context.Context, _, _ runtime.Object) {
}

func (sbomStrategy) Validate(_ context.Context, _ runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

// WarningsOnCreate returns warnings for the creation of the given object.
func (sbomStrategy) WarningsOnCreate(_ context.Context, _ runtime.Object) []string {
	return nil
}

func (sbomStrategy) AllowCreateOnUpdate() bool {
	return false
}

func (sbomStrategy) AllowUnconditionalUpdate() bool {
	return false
}

func (sbomStrategy) Canonicalize(_ runtime.Object) {
}

func (sbomStrategy) ValidateUpdate(_ context.Context, _, _ runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

// WarningsOnUpdate returns warnings for the given update.
func (sbomStrategy) WarningsOnUpdate(_ context.Context, _, _ runtime.Object) []string {
	return nil
}
