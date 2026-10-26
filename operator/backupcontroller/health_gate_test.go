package backupcontroller

import (
	"context"
	"testing"
	"time"

	k8upv1 "github.com/k8up-io/k8up/v2/api/v1"
	"github.com/k8up-io/k8up/v2/operator/cfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBackupReconciler_gateOnRepositoryHealth(t *testing.T) {
	backend := &k8upv1.Backend{S3: &k8upv1.S3Spec{Endpoint: "https://endpoint", Bucket: "bucket"}}
	finishedCheck := func(reason k8upv1.ConditionReason) *k8upv1.Check {
		return &k8upv1.Check{
			ObjectMeta: metav1.ObjectMeta{Name: "weekly", Namespace: "app"},
			Spec:       k8upv1.CheckSpec{RunnableSpec: k8upv1.RunnableSpec{Backend: backend}},
			Status: k8upv1.Status{Conditions: []metav1.Condition{{
				Type:               k8upv1.ConditionCompleted.String(),
				Status:             metav1.ConditionTrue,
				Reason:             reason.String(),
				LastTransitionTime: metav1.NewTime(time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)),
			}}},
		}
	}

	tests := map[string]struct {
		givenCheck        *k8upv1.Check
		givenSkipOnFailed bool
		givenGlobalSkip   bool
		expectedSkip      bool
		expectedHealthy   metav1.ConditionStatus
		expectedReason    k8upv1.ConditionReason
		expectedFinished  bool
	}{
		"GivenNoCheck_ThenExpectUnknownAndNoSkip": {
			expectedHealthy: metav1.ConditionUnknown,
			expectedReason:  k8upv1.ReasonNoCheckFound,
		},
		"GivenSucceededCheck_WhenSkipEnabled_ThenExpectHealthyAndNoSkip": {
			givenCheck:        finishedCheck(k8upv1.ReasonSucceeded),
			givenSkipOnFailed: true,
			expectedHealthy:   metav1.ConditionTrue,
			expectedReason:    k8upv1.ReasonLastCheckSucceeded,
		},
		"GivenFailedCheck_WhenSkipDisabled_ThenExpectUnhealthyAndNoSkip": {
			givenCheck:      finishedCheck(k8upv1.ReasonFailed),
			expectedHealthy: metav1.ConditionFalse,
			expectedReason:  k8upv1.ReasonLastCheckFailed,
		},
		"GivenFailedCheck_WhenSkipOnFailedCheck_ThenExpectSkipped": {
			givenCheck:        finishedCheck(k8upv1.ReasonFailed),
			givenSkipOnFailed: true,
			expectedSkip:      true,
			expectedHealthy:   metav1.ConditionFalse,
			expectedReason:    k8upv1.ReasonLastCheckFailed,
			expectedFinished:  true,
		},
		"GivenFailedCheck_WhenGlobalSkip_ThenExpectSkipped": {
			givenCheck:       finishedCheck(k8upv1.ReasonFailed),
			givenGlobalSkip:  true,
			expectedSkip:     true,
			expectedHealthy:  metav1.ConditionFalse,
			expectedReason:   k8upv1.ReasonLastCheckFailed,
			expectedFinished: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			previousConfig := cfg.Config
			t.Cleanup(func() { cfg.Config = previousConfig })
			cfg.Config = &cfg.Configuration{SkipBackupOnFailedCheck: tt.givenGlobalSkip}

			backup := &k8upv1.Backup{
				ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "app"},
				Spec: k8upv1.BackupSpec{
					RunnableSpec:      k8upv1.RunnableSpec{Backend: backend},
					SkipOnFailedCheck: tt.givenSkipOnFailed,
				},
			}
			objects := []client.Object{backup}
			if tt.givenCheck != nil {
				objects = append(objects, tt.givenCheck)
			}
			scheme := runtime.NewScheme()
			require.NoError(t, k8upv1.AddToScheme(scheme))
			kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithStatusSubresource(backup).Build()
			r := &BackupReconciler{Kube: kube}

			skip, err := r.gateOnRepositoryHealth(context.Background(), backup, backend.String())
			require.NoError(t, err)
			assert.Equal(t, tt.expectedSkip, skip)

			stored := &k8upv1.Backup{}
			require.NoError(t, kube.Get(context.Background(), client.ObjectKeyFromObject(backup), stored))
			healthy := meta.FindStatusCondition(stored.Status.Conditions, k8upv1.ConditionRepositoryHealthy.String())
			require.NotNil(t, healthy)
			assert.Equal(t, tt.expectedHealthy, healthy.Status)
			assert.Equal(t, tt.expectedReason.String(), healthy.Reason)
			assert.Equal(t, tt.expectedFinished, stored.Status.HasFinished())
			if tt.expectedFinished {
				completed := meta.FindStatusCondition(stored.Status.Conditions, k8upv1.ConditionCompleted.String())
				assert.Equal(t, k8upv1.ReasonRepositoryUnhealthy.String(), completed.Reason)
			}
		})
	}
}
