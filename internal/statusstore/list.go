package statusstore

import "sort"

// List returns a stable deployment-name-sorted copy of snapshots.
func List(snapshots []Snapshot) []Snapshot {
	listed := append([]Snapshot(nil), snapshots...)
	sort.Slice(listed, func(i, j int) bool {
		return listed[i].DeploymentName < listed[j].DeploymentName
	})
	return listed
}
