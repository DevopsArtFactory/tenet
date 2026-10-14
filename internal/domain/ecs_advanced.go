package domain

// ECSAdvancedConfiguration is reviewed native traffic intent. It intentionally
// covers existing ALB routing and Lambda hooks; other routing/hook types are not
// implicitly inherited or converted to CodeDeploy.
type ECSAdvancedConfiguration struct {
	BakeTimeInMinutes *int32                  `json:"bakeTimeInMinutes" yaml:"bakeTimeInMinutes"`
	Canary            *ECSCanaryConfiguration `json:"canary,omitempty" yaml:"canary,omitempty"`
	Linear            *ECSLinearConfiguration `json:"linear,omitempty" yaml:"linear,omitempty"`
	Routing           ECSALBRouting           `json:"routing" yaml:"routing"`
	Alarms            *ECSDeploymentAlarms    `json:"alarms,omitempty" yaml:"alarms,omitempty"`
	LifecycleHooks    []ECSLambdaHook         `json:"lifecycleHooks,omitempty" yaml:"lifecycleHooks,omitempty"`
}
type ECSCanaryConfiguration struct {
	Percent           *float64 `json:"percent" yaml:"percent"`
	BakeTimeInMinutes *int32   `json:"bakeTimeInMinutes" yaml:"bakeTimeInMinutes"`
}
type ECSLinearConfiguration struct {
	StepPercent           *float64 `json:"stepPercent" yaml:"stepPercent"`
	StepBakeTimeInMinutes *int32   `json:"stepBakeTimeInMinutes" yaml:"stepBakeTimeInMinutes"`
}
type ECSALBRouting struct {
	TargetGroupARN            string `json:"targetGroupArn" yaml:"targetGroupArn"`
	AlternateTargetGroupARN   string `json:"alternateTargetGroupArn" yaml:"alternateTargetGroupArn"`
	ProductionListenerRuleARN string `json:"productionListenerRuleArn" yaml:"productionListenerRuleArn"`
	TestListenerRuleARN       string `json:"testListenerRuleArn,omitempty" yaml:"testListenerRuleArn,omitempty"`
	InfrastructureRoleARN     string `json:"infrastructureRoleArn" yaml:"infrastructureRoleArn"`
	ContainerPort             int32  `json:"containerPort" yaml:"containerPort"`
}
type ECSDeploymentAlarms struct {
	Enable   bool     `json:"enable" yaml:"enable"`
	Rollback bool     `json:"rollback" yaml:"rollback"`
	Names    []string `json:"names" yaml:"names"`
}
type ECSLambdaHook struct {
	FunctionARN string   `json:"functionArn" yaml:"functionArn"`
	RoleARN     string   `json:"roleArn" yaml:"roleArn"`
	Stages      []string `json:"stages" yaml:"stages"`
}

func (c *ECSAdvancedConfiguration) Clone() *ECSAdvancedConfiguration {
	if c == nil {
		return nil
	}
	copy := *c
	if c.BakeTimeInMinutes != nil {
		value := *c.BakeTimeInMinutes
		copy.BakeTimeInMinutes = &value
	}
	if c.Canary != nil {
		value := *c.Canary
		copy.Canary = &value
		if value.Percent != nil {
			v := *value.Percent
			copy.Canary.Percent = &v
		}
		if value.BakeTimeInMinutes != nil {
			v := *value.BakeTimeInMinutes
			copy.Canary.BakeTimeInMinutes = &v
		}
	}
	if c.Linear != nil {
		value := *c.Linear
		copy.Linear = &value
		if value.StepPercent != nil {
			v := *value.StepPercent
			copy.Linear.StepPercent = &v
		}
		if value.StepBakeTimeInMinutes != nil {
			v := *value.StepBakeTimeInMinutes
			copy.Linear.StepBakeTimeInMinutes = &v
		}
	}
	if c.Alarms != nil {
		value := *c.Alarms
		copy.Alarms = &value
		copy.Alarms.Names = append([]string(nil), c.Alarms.Names...)
	}
	copy.LifecycleHooks = append([]ECSLambdaHook(nil), c.LifecycleHooks...)
	for i := range copy.LifecycleHooks {
		copy.LifecycleHooks[i].Stages = append([]string(nil), c.LifecycleHooks[i].Stages...)
	}
	return &copy
}
