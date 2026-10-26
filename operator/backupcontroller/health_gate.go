package backupcontroller

import (
	"context"
	"fmt"
	"reflect"

	k8upv1 "github.com/k8up-io/k8up/v2/api/v1"
	"github.com/k8up-io/k8up/v2/operator/cfg"
	"github.com/k8up-io/k8up/v2/operator/monitoring"
	"github.com/k8up-io/k8up/v2/restic/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gateOnRepositoryHealth records the health of the backup's repository in the ConditionRepositoryHealthy
// condition and reports whether the backup must be skipped.
// A backup is skipped when the last finished Check of its repository failed and either the Backup sets
// skipOnFailedCheck or the operator skips such backups globally. A skipped Backup is marked as finished.
func (r *BackupReconciler) gateOnRepositoryHealth(ctx context.Context, obj *k8upv1.Backup, repository string) (bool, error) {
	health, err := kubernetes.ReadRepositoryHealth(ctx, r.Kube, obj.Namespace, repository, cfg.Config.GetGlobalRepository())
	if err != nil {
		return false, fmt.Errorf("read repository health: %w", err)
	}

	status := obj.Status
	switch {
	case health.LastCheck == nil:
		status.SetCondition(k8upv1.ConditionRepositoryHealthy, k8upv1.ReasonNoCheckFound, metav1.ConditionUnknown, health.Message())
	case health.LastCheckFailed():
		status.SetCondition(k8upv1.ConditionRepositoryHealthy, k8upv1.ReasonLastCheckFailed, metav1.ConditionFalse, health.Message())
		monitoring.SetBackupRepositoryHealthy(obj.Namespace, false)
	default:
		status.SetCondition(k8upv1.ConditionRepositoryHealthy, k8upv1.ReasonLastCheckSucceeded, metav1.ConditionTrue, health.Message())
		monitoring.SetBackupRepositoryHealthy(obj.Namespace, true)
	}

	skip := health.LastCheckFailed() && (obj.Spec.SkipOnFailedCheck || cfg.Config.SkipBackupOnFailedCheck)
	if skip {
		status.SetCondition(k8upv1.ConditionCompleted, k8upv1.ReasonRepositoryUnhealthy, metav1.ConditionTrue,
			"the backup was skipped because the last check of its repository failed")
		status.SetFinished("the backup was skipped")
		monitoring.IncBackupsSkippedCounter(obj.Namespace)
	}

	if reflect.DeepEqual(status, obj.Status) {
		return skip, nil
	}
	obj.SetStatus(status)
	if err := r.Kube.Status().Update(ctx, obj); err != nil {
		return false, fmt.Errorf("backup status update failed: %w", err)
	}
	return skip, nil
}
