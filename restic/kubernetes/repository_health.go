package kubernetes

import (
	"context"
	"fmt"

	k8upv1 "github.com/k8up-io/k8up/v2/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RepositoryHealth is what the cluster knows about a restic repository, read from the
// Check and Snapshot resources in one namespace.
type RepositoryHealth struct {
	// LastCheck is the Check against the repository that finished most recently, nil if none has finished.
	LastCheck *k8upv1.Check
	// SnapshotCount is the number of Snapshot resources synced for the repository.
	SnapshotCount int
	// LatestSnapshot is when the newest Snapshot of the repository was taken, nil if there is none.
	LatestSnapshot *metav1.Time
}

// LastCheckFailed returns true if the last finished Check of the repository failed.
func (h RepositoryHealth) LastCheckFailed() bool {
	return h.LastCheck != nil && h.LastCheck.Status.HasFailed()
}

// Message describes the repository health for a status condition.
func (h RepositoryHealth) Message() string {
	snapshots := fmt.Sprintf("%d snapshots", h.SnapshotCount)
	if h.LatestSnapshot != nil {
		snapshots += fmt.Sprintf(", latest taken %s", h.LatestSnapshot.UTC().Format("2006-01-02T15:04:05Z"))
	}
	if h.LastCheck == nil {
		return fmt.Sprintf("no check of the repository has finished yet (%s)", snapshots)
	}
	result := "succeeded"
	if h.LastCheckFailed() {
		result = "failed"
	}
	return fmt.Sprintf("last check %q %s at %s (%s)", h.LastCheck.Name, result,
		h.LastCheck.FinishedAt().UTC().Format("2006-01-02T15:04:05Z"), snapshots)
}

// ReadRepositoryHealth reads the Checks and Snapshots of the given repository in the namespace.
// Checks without a backend run against the operator's global repository, given as defaultRepository.
func ReadRepositoryHealth(ctx context.Context, kube client.Reader, namespace, repository, defaultRepository string) (RepositoryHealth, error) {
	checks := &k8upv1.CheckList{}
	if err := kube.List(ctx, checks, client.InNamespace(namespace)); err != nil {
		return RepositoryHealth{}, fmt.Errorf("list checks: %w", err)
	}
	checks = filterChecksByRepo(checks, repository, defaultRepository)

	snapshots := &k8upv1.SnapshotList{}
	if err := kube.List(ctx, snapshots, client.InNamespace(namespace)); err != nil {
		return RepositoryHealth{}, fmt.Errorf("list snapshots: %w", err)
	}
	snapshots = filterByRepo(snapshots, repository)

	health := RepositoryHealth{
		LastCheck:     checks.LatestFinished(),
		SnapshotCount: len(snapshots.Items),
	}
	if latest := snapshots.Latest(); latest != nil {
		health.LatestSnapshot = latest.Spec.Date
	}
	return health, nil
}

// filterChecksByRepo will filter the list according to the given repository.
func filterChecksByRepo(list *k8upv1.CheckList, repo, defaultRepo string) *k8upv1.CheckList {
	filteredList := &k8upv1.CheckList{Items: []k8upv1.Check{}}

	for _, check := range list.Items {
		if check.RepositoryOrDefault(defaultRepo) == repo {
			filteredList.Items = append(filteredList.Items, check)
		}
	}

	return filteredList
}
