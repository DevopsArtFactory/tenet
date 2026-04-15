package domain

// DeploymentArtifact captures either a binary artifact for EC2 or an image artifact for ECS.
type DeploymentArtifact struct {
	Binary string `json:"binary,omitempty"`
	Image  string `json:"image,omitempty"`
}

// DeploymentUserData captures normalized EC2 bootstrap source metadata.
type DeploymentUserData struct {
	Type string `json:"type,omitempty"`
	Path string `json:"path,omitempty"`
}

// Deployment is the desired deployment intent for one application and environment.
type Deployment struct {
	Name            string
	AppRef          string
	EnvironmentRef  string
	Generation      int64
	TargetType      string
	Strategy        string
	ExecutionMode   string
	DesiredArtifact DeploymentArtifact
	UserData        DeploymentUserData
}
