package domain

import "testing"

func TestAssignManagedDriftIDsDerivesStableOpaqueIDs(t *testing.T) {
	items := []ManagedDriftItem{
		{Field: "runtimeContext.capacity.desired", Desired: "6", Actual: "4", Severity: ManagedDriftSeverityHigh, ResourceRef: &ManagedDriftResourceRef{ResourceType: "ecsService", ResourceName: "payments-api"}},
		{Field: "runtimeContext.capacity.desired", Desired: "6", Actual: "4", Severity: ManagedDriftSeverityHigh, ResourceRef: &ManagedDriftResourceRef{ResourceType: "ecsService", ResourceName: "payments-api"}},
		{Field: "runtimeContext.capacity.min", Desired: "4", Actual: "2", Severity: ManagedDriftSeverityMedium},
	}

	first := AssignManagedDriftIDs(items)
	second := AssignManagedDriftIDs(items)

	if len(first) != len(items) || len(second) != len(items) {
		t.Fatalf("expected assigned rows to preserve length, got %d and %d", len(first), len(second))
	}
	for i := range first {
		if first[i].DriftID == "" || first[i].DriftID[:6] != "drift_" {
			t.Fatalf("expected drift_%d id, got %#v", i, first[i].DriftID)
		}
		if first[i].DriftID != second[i].DriftID {
			t.Fatalf("expected stable id at row %d, got %q then %q", i, first[i].DriftID, second[i].DriftID)
		}
	}
	if first[0].DriftID == first[1].DriftID {
		t.Fatalf("expected duplicate rows to receive ordinal-distinct ids, got %q", first[0].DriftID)
	}
	if items[0].DriftID != "" {
		t.Fatalf("expected input slice to remain unmodified, got %#v", items[0])
	}
	first[0].ResourceRef.ResourceName = "mutated"
	if items[0].ResourceRef.ResourceName != "payments-api" {
		t.Fatalf("expected resource refs to be cloned defensively, got %#v", items[0].ResourceRef)
	}
}

func TestAssignManagedDriftIDsChangesWhenStableInputsChange(t *testing.T) {
	base := AssignManagedDriftIDs([]ManagedDriftItem{{Field: "artifact", Desired: "v2", Actual: "v1", Severity: ManagedDriftSeverityHigh}})
	changed := AssignManagedDriftIDs([]ManagedDriftItem{{Field: "artifact", Desired: "v3", Actual: "v1", Severity: ManagedDriftSeverityHigh}})

	if base[0].DriftID == changed[0].DriftID {
		t.Fatalf("expected changed stable inputs to change drift id, got %q", base[0].DriftID)
	}
}

func TestAssignManagedDriftIDsPreservesExistingIDs(t *testing.T) {
	items := []ManagedDriftItem{
		{DriftID: "  drift_existing  ", Field: "artifact", Desired: "v2", Actual: "v1", Severity: ManagedDriftSeverityHigh},
		{Field: "artifact", Desired: "v2", Actual: "v1", Severity: ManagedDriftSeverityHigh},
	}

	assigned := AssignManagedDriftIDs(items)

	if assigned[0].DriftID != "drift_existing" {
		t.Fatalf("expected existing persisted id to be preserved and normalized, got %q", assigned[0].DriftID)
	}
	if assigned[1].DriftID == "" || assigned[1].DriftID == assigned[0].DriftID {
		t.Fatalf("expected missing duplicate id to be generated distinctly, got %#v", assigned)
	}
}
