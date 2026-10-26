package kubernetes

import (
	"context"
	"testing"
	"time"

	k8upv1 "github.com/k8up-io/k8up/v2/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	healthNamespace  = "app"
	healthRepository = "s3:https://endpoint/bucket"
	globalRepository = "s3:https://global/bucket"
)

func newCheck(name string, backend *k8upv1.Backend, reason k8upv1.ConditionReason, finishedAt time.Time) *k8upv1.Check {
	return &k8upv1.Check{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: healthNamespace},
		Spec:       k8upv1.CheckSpec{RunnableSpec: k8upv1.RunnableSpec{Backend: backend}},
		Status: k8upv1.Status{Conditions: []metav1.Condition{{
			Type:               k8upv1.ConditionCompleted.String(),
			Status:             metav1.ConditionTrue,
			Reason:             reason.String(),
			LastTransitionTime: metav1.NewTime(finishedAt),
		}}},
	}
}

func newSnapshot(name, repository string, date time.Time) *k8upv1.Snapshot {
	return &k8upv1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: healthNamespace},
		Spec: k8upv1.SnapshotSpec{
			ID:         ptr.To(name),
			Date:       &metav1.Time{Time: date},
			Repository: ptr.To(repository),
		},
	}
}

func TestReadRepositoryHealth(t *testing.T) {
	base := time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)
	backend := &k8upv1.Backend{S3: &k8upv1.S3Spec{Endpoint: "https://endpoint", Bucket: "bucket"}}
	otherBackend := &k8upv1.Backend{S3: &k8upv1.S3Spec{Endpoint: "https://endpoint", Bucket: "other"}}

	tests := map[string]struct {
		givenRepository       string
		givenObjects          []client.Object
		expectedLastCheck     *string
		expectedFailed        bool
		expectedSnapshotCount int
		expectedLatest        *time.Time
	}{
		"GivenNoResources_ThenExpectNoCheckAndNoSnapshots": {
			givenRepository: healthRepository,
		},
		"GivenFailedCheckAfterSucceededCheck_ThenExpectFailed": {
			givenRepository: healthRepository,
			givenObjects: []client.Object{
				newCheck("weekly-1", backend, k8upv1.ReasonSucceeded, base),
				newCheck("weekly-2", backend, k8upv1.ReasonFailed, base.Add(7*24*time.Hour)),
				newSnapshot("aaaaaaaa", healthRepository, base),
				newSnapshot("bbbbbbbb", healthRepository, base.Add(time.Hour)),
			},
			expectedLastCheck:     ptr.To("weekly-2"),
			expectedFailed:        true,
			expectedSnapshotCount: 2,
			expectedLatest:        ptr.To(base.Add(time.Hour)),
		},
		"GivenChecksOfOtherRepository_ThenExpectThemIgnored": {
			givenRepository: healthRepository,
			givenObjects: []client.Object{
				newCheck("ours", backend, k8upv1.ReasonSucceeded, base),
				newCheck("theirs", otherBackend, k8upv1.ReasonFailed, base.Add(time.Hour)),
				newSnapshot("cccccccc", "s3:https://endpoint/other", base),
			},
			expectedLastCheck: ptr.To("ours"),
		},
		"GivenCheckWithoutBackend_WhenGlobalRepository_ThenExpectItMatched": {
			givenRepository: globalRepository,
			givenObjects: []client.Object{
				newCheck("global", nil, k8upv1.ReasonFailed, base),
			},
			expectedLastCheck: ptr.To("global"),
			expectedFailed:    true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, k8upv1.AddToScheme(scheme))
			kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.givenObjects...).Build()

			health, err := ReadRepositoryHealth(context.Background(), kube, healthNamespace, tt.givenRepository, globalRepository)
			require.NoError(t, err)

			if tt.expectedLastCheck == nil {
				assert.Nil(t, health.LastCheck)
			} else {
				require.NotNil(t, health.LastCheck)
				assert.Equal(t, *tt.expectedLastCheck, health.LastCheck.Name)
			}
			assert.Equal(t, tt.expectedFailed, health.LastCheckFailed())
			assert.Equal(t, tt.expectedSnapshotCount, health.SnapshotCount)
			if tt.expectedLatest == nil {
				assert.Nil(t, health.LatestSnapshot)
			} else {
				assert.True(t, tt.expectedLatest.Equal(health.LatestSnapshot.Time))
			}
		})
	}
}

func TestRepositoryHealth_Message(t *testing.T) {
	base := time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)
	assert.Equal(t, "no check of the repository has finished yet (0 snapshots)", RepositoryHealth{}.Message())

	health := RepositoryHealth{
		LastCheck:      newCheck("weekly", nil, k8upv1.ReasonFailed, base),
		SnapshotCount:  3,
		LatestSnapshot: &metav1.Time{Time: base.Add(-time.Hour)},
	}
	assert.Equal(t, `last check "weekly" failed at 2026-10-20T03:00:00Z (3 snapshots, latest taken 2026-10-20T02:00:00Z)`, health.Message())
}
