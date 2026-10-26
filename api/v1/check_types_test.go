package v1

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func newFinishedCheck(name string, reason ConditionReason, finishedAt time.Time) Check {
	return Check{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: Status{Conditions: []metav1.Condition{{
			Type:               ConditionCompleted.String(),
			Status:             metav1.ConditionTrue,
			Reason:             reason.String(),
			LastTransitionTime: metav1.NewTime(finishedAt),
		}}},
	}
}

func TestCheck_RepositoryOrDefault(t *testing.T) {
	check := Check{}
	assert.Equal(t, "s3:https://global/bucket", check.RepositoryOrDefault("s3:https://global/bucket"))

	check.Spec.Backend = &Backend{S3: &S3Spec{Endpoint: "https://endpoint", Bucket: "bucket"}}
	assert.Equal(t, "s3:https://endpoint/bucket", check.RepositoryOrDefault("s3:https://global/bucket"))
}

func TestCheck_FinishedAt(t *testing.T) {
	finishedAt := time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)

	running := Check{}
	running.Status.SetStarted("running")
	assert.Nil(t, running.FinishedAt())

	failed := newFinishedCheck("failed", ReasonFailed, finishedAt)
	assert.Equal(t, finishedAt, failed.FinishedAt().Time)
}

func TestCheckList_LatestFinished(t *testing.T) {
	base := time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)
	running := Check{ObjectMeta: metav1.ObjectMeta{Name: "running"}}
	running.Status.SetStarted("running")

	tests := map[string]struct {
		givenChecks  []Check
		expectedName *string
	}{
		"GivenNoChecks_ThenExpectNil": {},
		"GivenOnlyRunningCheck_ThenExpectNil": {
			givenChecks: []Check{running},
		},
		"GivenFinishedChecks_ThenExpectMostRecent": {
			givenChecks: []Check{
				newFinishedCheck("old", ReasonSucceeded, base),
				newFinishedCheck("new", ReasonFailed, base.Add(24*time.Hour)),
				running,
			},
			expectedName: ptr.To("new"),
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			latest := (&CheckList{Items: tt.givenChecks}).LatestFinished()
			if tt.expectedName == nil {
				assert.Nil(t, latest)
				return
			}
			assert.Equal(t, *tt.expectedName, latest.Name)
		})
	}
}

func TestSnapshotList_Latest(t *testing.T) {
	base := time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)
	list := &SnapshotList{Items: []Snapshot{
		{ObjectMeta: metav1.ObjectMeta{Name: "old"}, Spec: SnapshotSpec{Date: &metav1.Time{Time: base}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "undated"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "new"}, Spec: SnapshotSpec{Date: &metav1.Time{Time: base.Add(time.Hour)}}},
	}}
	assert.Equal(t, "new", list.Latest().Name)
	assert.Nil(t, (&SnapshotList{}).Latest())
}
