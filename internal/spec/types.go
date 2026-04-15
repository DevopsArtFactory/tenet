package spec

// AccountSpec declares a target AWS account Tenet can manage.
type AccountSpec struct {
	Name            string   `json:"name" yaml:"name"`
	AWSAccountID    string   `json:"awsAccountId" yaml:"awsAccountId"`
	RoleARN         string   `json:"roleArn" yaml:"roleArn"`
	ObserverRoleARN string   `json:"observerRoleArn,omitempty" yaml:"observerRoleArn,omitempty"`
	AllowedRegions  []string `json:"allowedRegions,omitempty" yaml:"allowedRegions,omitempty"`
}

// EnvironmentSpec binds a logical environment to an account and region.
type EnvironmentSpec struct {
	Name       string `json:"name" yaml:"name"`
	AccountRef string `json:"accountRef" yaml:"accountRef"`
	Region     string `json:"region" yaml:"region"`
	NetworkRef string `json:"networkRef,omitempty" yaml:"networkRef,omitempty"`
	PolicyRef  string `json:"policyRef,omitempty" yaml:"policyRef,omitempty"`
}

// DeploymentSpec describes desired deployment intent.
type DeploymentSpec struct {
	Name           string       `json:"name" yaml:"name"`
	AppRef         string       `json:"appRef,omitempty" yaml:"appRef,omitempty"`
	EnvironmentRef string       `json:"environmentRef,omitempty" yaml:"environmentRef,omitempty"`
	Target         TargetSpec   `json:"target,omitempty" yaml:"target,omitempty"`
	Strategy       StrategySpec `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	Traffic        TrafficSpec  `json:"traffic,omitempty" yaml:"traffic,omitempty"`
	Capacity       CapacitySpec `json:"capacity,omitempty" yaml:"capacity,omitempty"`
	Binary         string       `json:"binary,omitempty" yaml:"binary,omitempty"`
	Image          string       `json:"image,omitempty" yaml:"image,omitempty"`
	UserData       UserDataSpec `json:"userdata,omitempty" yaml:"userdata,omitempty"`
	Tags           []string     `json:"tags,omitempty" yaml:"tags,omitempty"`
	Stacks         []StackSpec  `json:"stacks,omitempty" yaml:"stacks,omitempty"`
}

// TargetType identifies a supported workload execution platform.
type TargetType string

const (
	TargetTypeEC2 TargetType = "ec2"
	TargetTypeECS TargetType = "ecs"
)

// TargetSpec selects the workload platform.
type TargetSpec struct {
	Type TargetType `json:"type,omitempty" yaml:"type,omitempty"`
}

// StrategyType identifies rollout strategy.
type StrategyType string

const (
	StrategyTypeBlueGreen StrategyType = "blueGreen"
	StrategyTypeRolling   StrategyType = "rolling"
	StrategyTypeCanary    StrategyType = "canary"
	StrategyTypeLinear    StrategyType = "linear"
)

// StrategySpec configures rollout behavior.
type StrategySpec struct {
	Type              StrategyType `json:"type,omitempty" yaml:"type,omitempty"`
	WarmupSeconds     int          `json:"warmupSeconds,omitempty" yaml:"warmupSeconds,omitempty"`
	MinHealthyPercent int          `json:"minHealthyPercent,omitempty" yaml:"minHealthyPercent,omitempty"`
}

// TrafficSpec describes ingress expectations.
type TrafficSpec struct {
	Hostname string `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	Port     int    `json:"port,omitempty" yaml:"port,omitempty"`
}

// CapacitySpec describes desired scaling bounds.
type CapacitySpec struct {
	Desired int `json:"desired,omitempty" yaml:"desired,omitempty"`
	Min     int `json:"min,omitempty" yaml:"min,omitempty"`
	Max     int `json:"max,omitempty" yaml:"max,omitempty"`
}

// UserDataSpec references user-data content for an instance-oriented deployment.
type UserDataSpec struct {
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
	Path string `json:"path,omitempty" yaml:"path,omitempty"`
}

// StackSpec is the goployer-compatible stack declaration Tenet can ingest.
type StackSpec struct {
	Stack                      string       `json:"stack,omitempty" yaml:"stack,omitempty"`
	Account                    string       `json:"account,omitempty" yaml:"account,omitempty"`
	Env                        string       `json:"env,omitempty" yaml:"env,omitempty"`
	ReplacementType            string       `json:"replacement_type,omitempty" yaml:"replacement_type,omitempty"`
	TerminationDelayRate       int          `json:"termination_delay_rate,omitempty" yaml:"termination_delay_rate,omitempty"`
	RollingUpdateInstanceCount int          `json:"rolling_update_instance_count,omitempty" yaml:"rolling_update_instance_count,omitempty"`
	UserData                   UserDataSpec `json:"userdata,omitempty" yaml:"userdata,omitempty"`
	Capacity                   CapacitySpec `json:"capacity,omitempty" yaml:"capacity,omitempty"`
	Regions                    []RegionSpec `json:"regions,omitempty" yaml:"regions,omitempty"`
}

// RegionSpec is the goployer-compatible region declaration Tenet currently ingests.
type RegionSpec struct {
	Region       string `json:"region,omitempty" yaml:"region,omitempty"`
	InstanceType string `json:"instance_type,omitempty" yaml:"instance_type,omitempty"`
	AMIID        string `json:"ami_id,omitempty" yaml:"ami_id,omitempty"`
}

// NormalizedAppRef returns the deployment appRef or falls back to the deployment name.
func (d DeploymentSpec) NormalizedAppRef() string {
	if d.AppRef != "" {
		return d.AppRef
	}
	return d.Name
}

// NormalizedEnvironmentRef returns an explicit environmentRef or falls back to the first stack env.
func (d DeploymentSpec) NormalizedEnvironmentRef() string {
	if d.EnvironmentRef != "" {
		return d.EnvironmentRef
	}
	if len(d.Stacks) > 0 {
		return d.Stacks[0].Env
	}
	return ""
}

// NormalizedTargetType returns an explicit target type or infers one from the deployment shape.
func (d DeploymentSpec) NormalizedTargetType() TargetType {
	if d.Target.Type != "" {
		return d.Target.Type
	}
	if d.Image != "" {
		return TargetTypeECS
	}
	if d.Binary != "" || len(d.Stacks) > 0 {
		return TargetTypeEC2
	}
	return ""
}

// NormalizedStrategy returns an explicit strategy or maps the first stack replacement type.
func (d DeploymentSpec) NormalizedStrategy() StrategyType {
	if d.Strategy.Type != "" {
		return d.Strategy.Type
	}
	if len(d.Stacks) == 0 {
		return ""
	}
	switch d.Stacks[0].ReplacementType {
	case "blueGreen", "bluegreen", "blue_green":
		return StrategyTypeBlueGreen
	case "rolling", "rollingUpdate", "rolling_update":
		return StrategyTypeRolling
	case "canary":
		return StrategyTypeCanary
	case "linear":
		return StrategyTypeLinear
	default:
		return ""
	}
}

// NormalizedCapacity returns the top-level capacity or falls back to the first stack capacity.
func (d DeploymentSpec) NormalizedCapacity() CapacitySpec {
	if d.Capacity.Desired != 0 || d.Capacity.Min != 0 || d.Capacity.Max != 0 {
		return d.Capacity
	}
	if len(d.Stacks) > 0 {
		return d.Stacks[0].Capacity
	}
	return CapacitySpec{}
}
