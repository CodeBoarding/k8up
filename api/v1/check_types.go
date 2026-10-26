package v1

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CheckSpec defines the desired state of Check. It needs to contain the repository
// information.
type CheckSpec struct {
	RunnableSpec `json:",inline"`

	// PromURL sets a prometheus push URL where the backup container send metrics to
	// +optional
	PromURL string `json:"promURL,omitempty"`

	// ClusterName sets the kubernetes cluster name to send to pushgateway for grouping metrics
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// KeepJobs amount of jobs to keep for later analysis.
	//
	// Deprecated: Use FailedJobsHistoryLimit and SuccessfulJobsHistoryLimit respectively.
	// +optional
	KeepJobs *int `json:"keepJobs,omitempty"`
	// FailedJobsHistoryLimit amount of failed jobs to keep for later analysis.
	// KeepJobs is used property is not specified.
	// +optional
	FailedJobsHistoryLimit *int `json:"failedJobsHistoryLimit,omitempty"`
	// SuccessfulJobsHistoryLimit amount of successful jobs to keep for later analysis.
	// KeepJobs is used property is not specified.
	// +optional
	SuccessfulJobsHistoryLimit *int `json:"successfulJobsHistoryLimit,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Schedule Ref",type="string",JSONPath=`.metadata.ownerReferences[?(@.kind == "Schedule")].name`,description="Reference to Schedule"
// +kubebuilder:printcolumn:name="Completion",type="string",JSONPath=`.status.conditions[?(@.type == "Completed")].reason`,description="Status of Completion"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Check is the Schema for the checks API
type Check struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CheckSpec `json:"spec,omitempty"`
	Status Status    `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CheckList contains a list of Check
type CheckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Check `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Check{}, &CheckList{})
}

func (c *Check) GetType() JobType {
	return CheckType
}

// GetStatus retrieves the Status property
func (c *Check) GetStatus() Status {
	return c.Status
}

// SetStatus sets the Status property
func (c *Check) SetStatus(status Status) {
	c.Status = status
}

// GetResources returns the resource requirements
func (c *Check) GetResources() corev1.ResourceRequirements {
	return c.Spec.Resources
}

// GetPodSecurityContext returns the pod security context
func (c *Check) GetPodSecurityContext() *corev1.PodSecurityContext {
	return c.Spec.PodSecurityContext
}

// GetActiveDeadlineSeconds implements JobObject
func (c *Check) GetActiveDeadlineSeconds() *int64 {
	return c.Spec.ActiveDeadlineSeconds
}

// GetFailedJobsHistoryLimit returns failed jobs history limit.
// Returns KeepJobs if unspecified.
func (c *Check) GetFailedJobsHistoryLimit() *int {
	if c.Spec.FailedJobsHistoryLimit != nil {
		return c.Spec.FailedJobsHistoryLimit
	}
	return c.Spec.KeepJobs
}

// GetSuccessfulJobsHistoryLimit returns successful jobs history limit.
// Returns KeepJobs if unspecified.
func (c *Check) GetSuccessfulJobsHistoryLimit() *int {
	if c.Spec.SuccessfulJobsHistoryLimit != nil {
		return c.Spec.SuccessfulJobsHistoryLimit
	}
	return c.Spec.KeepJobs
}

func (b *Check) GetPodConfig(ctx context.Context, c client.Client) (*PodConfig, error) {
	if b.Spec.PodConfigRef == nil {
		return nil, nil
	}
	return NewPodConfig(ctx, b.Spec.PodConfigRef.Name, b.GetNamespace(), c)
}

// GetJobObjects returns a sortable list of jobs
func (c *CheckList) GetJobObjects() JobObjectList {
	items := make(JobObjectList, len(c.Items))
	for i := range c.Items {
		items[i] = &c.Items[i]
	}
	return items
}

// RepositoryOrDefault returns the restic repository this Check runs against.
// A Check without a backend runs against the operator's global repository, given as defaultRepository.
func (c *Check) RepositoryOrDefault(defaultRepository string) string {
	if c.Spec.Backend == nil {
		return defaultRepository
	}
	return c.Spec.Backend.String()
}

// FinishedAt returns when the Check completed, successfully or not, or nil if it has not finished yet.
func (c *Check) FinishedAt() *metav1.Time {
	if !c.Status.HasFinished() {
		return nil
	}
	completedCond := meta.FindStatusCondition(c.Status.Conditions, ConditionCompleted.String())
	return &completedCond.LastTransitionTime
}

// LatestFinished returns the Check that finished most recently, or nil if none of them has finished.
func (c *CheckList) LatestFinished() *Check {
	var latest *Check
	for i := range c.Items {
		finishedAt := c.Items[i].FinishedAt()
		if finishedAt == nil {
			continue
		}
		if latest == nil || finishedAt.After(latest.FinishedAt().Time) {
			latest = &c.Items[i]
		}
	}
	return latest
}

// GetDeepCopy returns a deep copy
func (in *CheckSchedule) GetDeepCopy() ScheduleSpecInterface {
	return in.DeepCopy()
}

// GetRunnableSpec returns a pointer to RunnableSpec
func (in *CheckSchedule) GetRunnableSpec() *RunnableSpec {
	return &in.RunnableSpec
}

// GetSchedule returns the schedule definition
func (in *CheckSchedule) GetSchedule() ScheduleDefinition {
	return in.Schedule
}

var (
	CheckKind = reflect.TypeOf(Check{}).Name()
)
