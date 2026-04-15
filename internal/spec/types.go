package spec

// AccountSpec declares a target AWS account Tenet can manage.
type AccountSpec struct {
	Name            string   `json:"name"`
	AWSAccountID    string   `json:"awsAccountId"`
	RoleARN         string   `json:"roleArn"`
	ObserverRoleARN string   `json:"observerRoleArn,omitempty"`
	AllowedRegions  []string `json:"allowedRegions,omitempty"`
}

// EnvironmentSpec binds a logical environment to an account and region.
type EnvironmentSpec struct {
	Name       string `json:"name"`
	AccountRef string `json:"accountRef"`
	Region     string `json:"region"`
	NetworkRef string `json:"networkRef,omitempty"`
	PolicyRef  string `json:"policyRef,omitempty"`
}

// DeploymentSpec describes desired deployment intent.
type DeploymentSpec struct {
	Name           string       `json:"name"`
	AppRef         string       `json:"appRef"`
	EnvironmentRef string       `json:"environmentRef"`
	Target         TargetSpec   `json:"target"`
	Strategy       StrategySpec `json:"strategy"`
	Traffic        TrafficSpec  `json:"traffic"`
	Capacity       CapacitySpec `json:"capacity"`
}

// TargetType identifies a supported workload execution platform.
type TargetType string

const (
	TargetTypeEC2 TargetType = "ec2"
	TargetTypeECS TargetType = "ecs"
)

// TargetSpec selects the workload platform.
type TargetSpec struct {
	Type TargetType `json:"type"`
}

// StrategyType identifies rollout strategy.
type StrategyType string

const (
	StrategyTypeBlueGreen StrategyType = "blueGreen"
	StrategyTypeRolling   StrategyType = "rolling"
)

// StrategySpec configures rollout behavior.
type StrategySpec struct {
	Type              StrategyType `json:"type"`
	WarmupSeconds     int          `json:"warmupSeconds,omitempty"`
	MinHealthyPercent int          `json:"minHealthyPercent,omitempty"`
}

// TrafficSpec describes ingress expectations.
type TrafficSpec struct {
	Hostname string `json:"hostname,omitempty"`
	Port     int    `json:"port"`
}

// CapacitySpec describes desired scaling bounds.
type CapacitySpec struct {
	Desired int `json:"desired"`
	Min     int `json:"min"`
	Max     int `json:"max"`
}
