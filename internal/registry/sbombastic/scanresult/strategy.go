/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scanresult

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

// NewStrategy creates and returns a flunderStrategy instance
func NewStrategy(typer runtime.ObjectTyper) scanresultStrategy {
	return scanresultStrategy{typer, names.SimpleNameGenerator}
}

// GetAttrs returns labels.Set, fields.Set, and error in case the given runtime.Object is not a Flunder
func GetAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	apiserver, ok := obj.(*v1alpha1.ScanResult)
	if !ok {
		return nil, nil, fmt.Errorf("given object is not a Flunder")
	}
	return labels.Set(apiserver.ObjectMeta.Labels), SelectableFields(apiserver), nil
}

// MatchFlunder is the filter used by the generic etcd backend to watch events
// from etcd to clients of the apiserver only interested in specific labels/fields.
func MatchFlunder(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{
		Label:    label,
		Field:    field,
		GetAttrs: GetAttrs,
	}
}

// SelectableFields returns a field set that represents the object.
func SelectableFields(obj *v1alpha1.ScanResult) fields.Set {
	return generic.ObjectMetaFieldsSet(&obj.ObjectMeta, true)
}

type scanresultStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

func (scanresultStrategy) NamespaceScoped() bool {
	return true
}

func (scanresultStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
}

func (scanresultStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
}

func (scanresultStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	// flunder := obj.(*wardle.Flunder)
	// return validation.ValidateFlunder(flunder)

	return field.ErrorList{}
}

// WarningsOnCreate returns warnings for the creation of the given object.
func (scanresultStrategy) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string {
	return nil
}

func (scanresultStrategy) AllowCreateOnUpdate() bool {
	return false
}

func (scanresultStrategy) AllowUnconditionalUpdate() bool {
	return false
}

func (scanresultStrategy) Canonicalize(obj runtime.Object) {
}

func (scanresultStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

// WarningsOnUpdate returns warnings for the given update.
func (scanresultStrategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string {
	return nil
}
