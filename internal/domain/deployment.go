package domain

// DeploymentArtifact captures either a binary artifact for EC2 or an image artifact for ECS.
type DeploymentArtifact struct {
	Binary string `json:"binary,omitempty"`
	Image  string `json:"image,omitempty"`
}

// Deployment is the desired deployment intent for one application and environment.
type Deployment struct {
	Name            string
	AppRef          string
	EnvironmentRef  string
	Generation      int64
	TargetType      string
	DesiredArtifact DeploymentArtifact
}
