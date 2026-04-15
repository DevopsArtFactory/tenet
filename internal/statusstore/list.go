package statusstore

import (
	"fmt"
	"sort"
)

var ErrNotFound = fmt.Errorf("status snapshot not found")

// List returns a stable deployment-name-sorted copy of snapshots.
func List(snapshots []Snapshot) []Snapshot {
	listed := append([]Snapshot(nil), snapshots...)
	sort.Slice(listed, func(i, j int) bool {
		return listed[i].DeploymentName < listed[j].DeploymentName
	})
	return listed
}

// Get returns one deployment status snapshot by deployment name.
func Get(snapshots []Snapshot, deploymentName string) (Snapshot, error) {
	for _, snapshot := range snapshots {
		if snapshot.DeploymentName == deploymentName {
			return snapshot, nil
		}
	}
	return Snapshot{}, fmt.Errorf("%w: %s", ErrNotFound, deploymentName)
}
