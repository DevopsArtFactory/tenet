package domain

import "testing"

func TestECSIntentCloneOwnsNestedTrafficPolicy(t *testing.T) {
	bake := int32(0)
	percent := 5.5
	original := &ECSDeploymentIntent{Advanced: &ECSAdvancedConfiguration{BakeTimeInMinutes: &bake, Canary: &ECSCanaryConfiguration{Percent: &percent, BakeTimeInMinutes: &bake}, Linear: &ECSLinearConfiguration{StepPercent: &percent, StepBakeTimeInMinutes: &bake}, Alarms: &ECSDeploymentAlarms{Names: []string{"health"}}, LifecycleHooks: []ECSLambdaHook{{Stages: []string{"POST_SCALE_UP"}}}}}
	copy := original.Clone()
	*copy.Advanced.BakeTimeInMinutes = 10
	*copy.Advanced.Canary.Percent = 50
	*copy.Advanced.Canary.BakeTimeInMinutes = 20
	*copy.Advanced.Linear.StepPercent = 30
	*copy.Advanced.Linear.StepBakeTimeInMinutes = 40
	copy.Advanced.Alarms.Names[0] = "mutated"
	copy.Advanced.LifecycleHooks[0].Stages[0] = "mutated"
	if *original.Advanced.BakeTimeInMinutes != 0 || *original.Advanced.Canary.Percent != 5.5 || *original.Advanced.Canary.BakeTimeInMinutes != 0 || *original.Advanced.Linear.StepPercent != 5.5 || *original.Advanced.Linear.StepBakeTimeInMinutes != 0 || original.Advanced.Alarms.Names[0] != "health" || original.Advanced.LifecycleHooks[0].Stages[0] != "POST_SCALE_UP" {
		t.Fatal("nested reviewed traffic configuration aliases caller storage")
	}
}
