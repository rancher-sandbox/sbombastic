package v1alpha1

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/rancher/sbombastic/api/v1alpha1"
)

// SetupRegistryWebhookWithManager registers the webhook for Registry in the manager.
func SetupRegistryWebhookWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewWebhookManagedBy(mgr).For(&v1alpha1.Registry{}).
		WithValidator(&RegistryCustomValidator{
			logger: mgr.GetLogger().WithName("registry_validator"),
		}).
		Complete()
	if err != nil {
		return fmt.Errorf("failed to setup Registry webhook: %w", err)
	}
	return nil
}

// +kubebuilder:webhook:path=/validate-sbombastic-sbombastic-rancher-io-v1alpha1-registry,mutating=false,failurePolicy=fail,sideEffects=None,groups=sbombastic.sbombastic.rancher.io,resources=registries,verbs=create;update,versions=v1alpha1,name=vregistry-v1alpha1.kb.io,admissionReviewVersions=v1

type RegistryCustomValidator struct {
	logger logr.Logger
}

var _ webhook.CustomValidator = &RegistryCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Registry.
func (v *RegistryCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	registry, ok := obj.(*v1alpha1.Registry)
	if !ok {
		return nil, fmt.Errorf("expected a Registry object but got %T", obj)
	}
	v.logger.Info("Validation for Registry upon creation", "name", registry.GetName())

	var allErrs field.ErrorList

	if err := validateScanInterval(registry); err != nil {
		fieldPath := field.NewPath("spec").Child("scanInterval")
		allErrs = append(allErrs, field.Invalid(fieldPath, registry.Spec.ScanInterval, err.Error()))
	}

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			v1alpha1.GroupVersion.WithKind("Registry").GroupKind(),
			registry.Name,
			allErrs,
		)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Registry.
func (v *RegistryCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	registry, ok := newObj.(*v1alpha1.Registry)
	if !ok {
		return nil, fmt.Errorf("expected a Registry object for the newObj but got %T", newObj)
	}
	v.logger.Info("Validation for Registry upon update", "name", registry.GetName())

	var allErrs field.ErrorList

	if err := validateScanInterval(registry); err != nil {
		fieldPath := field.NewPath("spec").Child("scanInterval")
		allErrs = append(allErrs, field.Invalid(fieldPath, registry.Spec.ScanInterval, err.Error()))
	}

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			v1alpha1.GroupVersion.WithKind("Registry").GroupKind(),
			registry.Name,
			allErrs,
		)
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Registry.
func (v *RegistryCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	registry, ok := obj.(*v1alpha1.Registry)
	if !ok {
		return nil, fmt.Errorf("expected a Registry object but got %T", obj)
	}
	v.logger.Info("Validation for Registry upon deletion", "name", registry.GetName())

	return nil, nil
}

func validateScanInterval(registry *v1alpha1.Registry) error {
	if registry.Spec.ScanInterval == nil {
		return nil
	}
	if registry.Spec.ScanInterval.Duration < time.Minute {
		return errors.New("scanInterval must be at least 1 minute")
	}

	return nil
}
