/*
Copyright 2024.

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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegistryAnnotation stores a snapshot of the Registry targeted by the ScanJob.
const RegistryAnnotation = "sbombastic.rancher.io/registry"

// ScanJobSpec defines the desired state of ScanJob.
type ScanJobSpec struct {
	// Registry is the registry in the same namespace to scan.
	// +kubebuilder:validation:Required
	Registry string `json:"registry"`
}

const (
	ConditionTypeScheduled  = "Scheduled"
	ConditionTypeInProgress = "InProgress"
	ConditionTypeComplete   = "Complete"
	ConditionTypeFailed     = "Failed"
)

const (
	ReasonPending                   = "Pending"
	ReasonScheduled                 = "Scheduled"
	ReasonInProgress                = "InProgress"
	ReasonCatalogCreationInProgress = "CatalogCreationInProgress"
	ReasonSBOMGenerationInProgress  = "SBOMGenerationInProgress"
	ReasonImageScanInProgress       = "ImageScanInProgress"
	ReasonComplete                  = "Complete"
	ReasonFailed                    = "Failed"
	ReasonNoImagesToScan            = "NoImagesToScan"
	ReasonAllImagesScanned          = "AllImagesScanned"
	ReasonRegistryNotFound          = "RegistryNotFound"
	ReasonInternalError             = "InternalError"
)

const (
	messagePending    = "ScanJob is pending"
	messageScheduled  = "ScanJob is scheduled"
	messageInProgress = "ScanJob is in progress"
	messageCompleted  = "ScanJob completed successfully"
	messageFailed     = "ScanJob failed"
)

// ScanJobStatus defines the observed state of ScanJob.
type ScanJobStatus struct {
	// Conditions represent the latest available observations of ScanJob state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ImagesCount is the number of images in the registry.
	ImagesCount int `json:"imagesCount,omitempty"`

	// ScannedImagesCount is the number of images that have been scanned.
	ScannedImagesCount int `json:"scannedImagesCount,omitempty"`

	// StartTime is when the job started processing.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the job completed or failed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ScanJob is the Schema for the scanjobs API.
type ScanJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScanJobSpec   `json:"spec,omitempty"`
	Status ScanJobStatus `json:"status,omitempty"`
}

// InitializeConditions initializes status fields and conditions.
func (s *ScanJob) InitializeConditions() {
	s.Status.Conditions = []metav1.Condition{}

	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeScheduled,
		Status:             metav1.ConditionUnknown,
		Reason:             ReasonPending,
		Message:            messagePending,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeInProgress,
		Status:             metav1.ConditionUnknown,
		Reason:             ReasonPending,
		Message:            messagePending,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeComplete,
		Status:             metav1.ConditionUnknown,
		Reason:             ReasonPending,
		Message:            messagePending,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeFailed,
		Status:             metav1.ConditionUnknown,
		Reason:             ReasonPending,
		Message:            messagePending,
		ObservedGeneration: s.Generation,
	})
}

// MarkScheduled marks the job as scheduled.
func (s *ScanJob) MarkScheduled(reason, message string) {
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeScheduled,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeInProgress,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonScheduled,
		Message:            messageScheduled,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeComplete,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonScheduled,
		Message:            messageScheduled,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeFailed,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonScheduled,
		Message:            messageScheduled,
		ObservedGeneration: s.Generation,
	})
}

// MarkInProgress marks the job as in progress.
func (s *ScanJob) MarkInProgress(reason, message string) {
	now := metav1.Now()
	s.Status.StartTime = &now

	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeScheduled,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonInProgress,
		Message:            messageInProgress,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeInProgress,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeComplete,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonInProgress,
		Message:            messageInProgress,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeFailed,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonInProgress,
		Message:            messageInProgress,
		ObservedGeneration: s.Generation,
	})
}

// MarkComplete marks the job as complete.
func (s *ScanJob) MarkComplete(reason, message string) {
	now := metav1.Now()
	s.Status.CompletionTime = &now

	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeScheduled,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonComplete,
		Message:            messageCompleted,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeInProgress,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonComplete,
		Message:            messageCompleted,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeComplete,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeFailed,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonComplete,
		Message:            messageCompleted,
		ObservedGeneration: s.Generation,
	})
}

// MarkFailed marks the job as failed.
func (s *ScanJob) MarkFailed(reason, message string) {
	now := metav1.Now()
	s.Status.CompletionTime = &now

	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeScheduled,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonFailed,
		Message:            messageFailed,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeInProgress,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonFailed,
		Message:            messageFailed,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeComplete,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonFailed,
		Message:            messageFailed,
		ObservedGeneration: s.Generation,
	})
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeFailed,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: s.Generation,
	})
}

// IsPending returns true if the job is not in any other state.
func (s *ScanJob) IsPending() bool {
	return !s.IsScheduled() && !s.IsInProgress() && !s.IsComplete() && !s.IsFailed()
}

// IsScheduled returns true if the job is scheduled.
func (s *ScanJob) IsScheduled() bool {
	scheduledCond := meta.FindStatusCondition(s.Status.Conditions, ConditionTypeScheduled)
	if scheduledCond == nil {
		return false
	}
	return scheduledCond.Status == metav1.ConditionTrue
}

// IsInProgress returns true if the job is currently in progress.
func (s *ScanJob) IsInProgress() bool {
	inProgressCond := meta.FindStatusCondition(s.Status.Conditions, ConditionTypeInProgress)
	if inProgressCond == nil {
		return false
	}
	return inProgressCond.Status == metav1.ConditionTrue
}

// IsComplete returns true if the job has completed successfully.
func (s *ScanJob) IsComplete() bool {
	completeCond := meta.FindStatusCondition(s.Status.Conditions, ConditionTypeComplete)
	if completeCond == nil {
		return false
	}
	return completeCond.Status == metav1.ConditionTrue
}

// IsFailed returns true if the job has failed.
func (s *ScanJob) IsFailed() bool {
	failedCond := meta.FindStatusCondition(s.Status.Conditions, ConditionTypeFailed)
	if failedCond == nil {
		return false
	}
	return failedCond.Status == metav1.ConditionTrue
}

// +kubebuilder:object:root=true

// ScanJobList contains a list of ScanJob.
type ScanJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScanJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScanJob{}, &ScanJobList{})
}
