package v1alpha1

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	sbombasticv1alpha1 "github.com/rancher/sbombastic/api/v1alpha1"
)

func SetupScanJobWebhookWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewWebhookManagedBy(mgr).
		For(&sbombasticv1alpha1.ScanJob{}).
		WithValidator(&ScanJobCustomValidator{
			client: mgr.GetClient(),
			logger: mgr.GetLogger().WithName("scanjob_validator"),
		}).
		WithDefaulter(&ScanJobCustomDefaulter{
			logger: mgr.GetLogger().WithName("scanjob_defaulter"),
		}).
		Complete()
	if err != nil {
		return fmt.Errorf("failed to setup ScanJob webhook: %w", err)
	}
	return nil
}

// +kubebuilder:webhook:path=/mutate-sbombastic-rancher-io-v1alpha1-scanjob,mutating=true,failurePolicy=fail,sideEffects=None,groups=sbombastic.rancher.io,resources=scanjobs,verbs=create;update,versions=v1alpha1,name=mscanjob.sbombastic.rancher.io,admissionReviewVersions=v1

type ScanJobCustomDefaulter struct {
	logger logr.Logger
}

var _ webhook.CustomDefaulter = &ScanJobCustomDefaulter{}

// Default mutates the object to set default values.
func (d *ScanJobCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	scanJob, ok := obj.(*sbombasticv1alpha1.ScanJob)
	if !ok {
		return fmt.Errorf("expected a ScanJob object but got %T", obj)
	}

	d.logger.Info("Defaulting ScanJob", "name", scanJob.GetName())

	if scanJob.Annotations == nil {
		scanJob.Annotations = make(map[string]string)
	}

	// Add creation timestamp annotation with nanosecond precision
	scanJob.Annotations[sbombasticv1alpha1.CreationTimestampAnnotation] = time.Now().Format(time.RFC3339Nano)

	return nil
}

// +kubebuilder:webhook:path=/validate-sbombastic-rancher-io-v1alpha1-scanjob,mutating=false,failurePolicy=fail,sideEffects=None,groups=sbombastic.rancher.io,resources=scanjobs,verbs=create;update,versions=v1alpha1,name=vscanjob.sbombastic.rancher.io,admissionReviewVersions=v1

type ScanJobCustomValidator struct {
	client client.Client
	logger logr.Logger
}

var _ webhook.CustomValidator = &ScanJobCustomValidator{}

// ValidateCreate validates the object on creation.
func (v *ScanJobCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	scanJob, ok := obj.(*sbombasticv1alpha1.ScanJob)
	if !ok {
		return nil, fmt.Errorf("expected a ScanJob object but got %T", obj)
	}
	v.logger.Info("Validation for ScanJob upon creation", "name", scanJob.GetName())

	var allErrs field.ErrorList
	fieldPath := field.NewPath("metadata").Child("name")
	// ScanJob names are limited to 63 characters because they are used as labels to identify VulnerabilityReports updated by the ScanJob.
	if len(scanJob.Name) > validation.LabelValueMaxLength {
		allErrs = append(allErrs, field.TooLong(fieldPath, scanJob.Name, validation.LabelValueMaxLength))
	}

	scanJobList := &sbombasticv1alpha1.ScanJobList{}
	if err := v.client.List(ctx, scanJobList, client.InNamespace(scanJob.GetNamespace())); err != nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("listing ScanJobs: %w", err))
	}

	for _, existingScanJob := range scanJobList.Items {
		// Check if the a ScanJob with the same registry is already running
		if existingScanJob.Spec.Registry == scanJob.Spec.Registry && (!existingScanJob.IsComplete() && !existingScanJob.IsFailed()) {
			fieldPath := field.NewPath("spec").Child("registry")
			allErrs = append(allErrs, field.Forbidden(fieldPath, fmt.Sprintf("a ScanJob for the registry %q is already running", scanJob.Spec.Registry)))
			break
		}
	}

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			sbombasticv1alpha1.GroupVersion.WithKind("ScanJob").GroupKind(),
			scanJob.Name,
			allErrs,
		)
	}
	return nil, nil
}

// ValidateUpdate validates the object on update.
func (v *ScanJobCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldJob, ok := oldObj.(*sbombasticv1alpha1.ScanJob)
	if !ok {
		return nil, fmt.Errorf("expected oldObj to be a ScanJob but got %T", oldObj)
	}
	newJob, ok := newObj.(*sbombasticv1alpha1.ScanJob)
	if !ok {
		return nil, fmt.Errorf("expected newObj to be a ScanJob but got %T", newObj)
	}
	v.logger.Info("Validation for ScanJob upon update", "name", newJob.GetName())

	if oldJob.Spec.Registry != newJob.Spec.Registry {
		fieldErr := field.Invalid(
			field.NewPath("spec").Child("registry"),
			newJob.Spec.Registry,
			"field is immutable")
		return nil, apierrors.NewInvalid(
			sbombasticv1alpha1.GroupVersion.WithKind("ScanJob").GroupKind(),
			newJob.Name,
			field.ErrorList{fieldErr},
		)
	}
	return nil, nil
}

// ValidateDelete validates the object on deletion.
func (v *ScanJobCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	scanJob, ok := obj.(*sbombasticv1alpha1.ScanJob)
	if !ok {
		return nil, fmt.Errorf("expected a ScanJob object but got %T", obj)
	}
	v.logger.Info("Validation for ScanJob upon deletion", "name", scanJob.GetName())
	return nil, nil
}
