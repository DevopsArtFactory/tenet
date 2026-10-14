package domain

// DeploymentArtifact captures either a binary artifact for EC2 or an image artifact for ECS.
type DeploymentArtifact struct {
	Binary string `json:"binary,omitempty"`
	Image  string `json:"image,omitempty"`
}

// DeploymentEC2LaunchTemplate captures normalized desired EC2 launch-template intent.
type DeploymentEC2LaunchTemplate struct {
	ImageID      string `json:"imageId,omitempty"`
	InstanceType string `json:"instanceType,omitempty"`
}

// DeploymentUserData captures normalized EC2 bootstrap source metadata.
type DeploymentUserData struct {
	Type string `json:"type,omitempty"`
	Path string `json:"path,omitempty"`
}

// DeploymentRolloutStrategy captures normalized rollout tuning preserved from the manifest.
type DeploymentRolloutStrategy struct {
	WarmupSeconds     int `json:"warmupSeconds,omitempty"`
	MinHealthyPercent int `json:"minHealthyPercent,omitempty"`
}

// DeploymentRolloutCapacity captures normalized rollout capacity bounds.
type DeploymentRolloutCapacity struct {
	Desired int `json:"desired,omitempty"`
	Min     int `json:"min,omitempty"`
	Max     int `json:"max,omitempty"`
}

// DeploymentRolloutStack captures stack-scoped rollout tuning preserved from goployer manifests.
type DeploymentRolloutStack struct {
	TerminationDelayRate       int `json:"terminationDelayRate,omitempty"`
	RollingUpdateInstanceCount int `json:"rollingUpdateInstanceCount,omitempty"`
}

// DeploymentRolloutConfig captures normalized rollout-shaping config preserved from the manifest.
type DeploymentRolloutConfig struct {
	Strategy DeploymentRolloutStrategy `json:"strategy,omitempty"`
	Capacity DeploymentRolloutCapacity `json:"capacity,omitempty"`
	Stacks   []DeploymentRolloutStack  `json:"stacks,omitempty"`
}

// Deployment is the desired deployment intent for one application and environment.
type Deployment struct {
	ECS               *ECSDeploymentIntent `json:"ecs,omitempty"`
	Name              string
	AppRef            string
	EnvironmentRef    string
	Generation        int64
	TargetType        string
	Strategy          string
	ExecutionMode     string
	DesiredArtifact   DeploymentArtifact
	EC2LaunchTemplate DeploymentEC2LaunchTemplate
	UserData          DeploymentUserData
	RolloutConfig     DeploymentRolloutConfig
}
