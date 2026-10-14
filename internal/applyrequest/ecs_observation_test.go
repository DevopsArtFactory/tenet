package applyrequest

import "testing"

func TestNativeObservationCloneAndAuditTrackTrafficWithoutTimestampNoise(t *testing.T) {
	percent := 0.0
	original := &ECSRolloutObservation{State: "progressing", Native: &ECSNativeRolloutObservation{Status: "IN_PROGRESS", Stage: "PRODUCTION_TRAFFIC_SHIFT", TargetConfiguredPercent: &percent, TriggeredAlarms: []string{"health"}}}
	copy := original.Clone()
	copy.ObservedAt = "new attempt"
	copy.Polls = 10
	if observationChanged(original, copy) {
		t.Fatal("timestamp-only native poll added an audit transition")
	}
	*copy.Native.TargetConfiguredPercent = 5.5
	copy.Native.TriggeredAlarms[0] = "changed"
	if *original.Native.TargetConfiguredPercent != 0 || original.Native.TriggeredAlarms[0] != "health" {
		t.Fatal("native evidence clone aliases mutable traffic/alarm data")
	}
	if !observationChanged(original, copy) {
		t.Fatal("actual native traffic progress omitted audit transition")
	}
}
