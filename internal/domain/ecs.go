package domain

// ECSDeploymentIntent is the reviewed, secret-free input for future ECS execution.
// Runtime/role/network settings are inherited from the pinned base task definition.
type ECSDeploymentIntent struct {
	RollbackOnFailure     bool   `json:"rollbackOnFailure,omitempty"`
	AWSAccountID          string `json:"awsAccountId"`
	Region                string `json:"region"`
	DeploymentController  string `json:"deploymentController"`
	DeploymentStrategy    string `json:"deploymentStrategy"`
	ClusterARN            string `json:"clusterArn"`
	ServiceARN            string `json:"serviceArn"`
	BaseTaskDefinitionARN string `json:"baseTaskDefinitionArn"`
	ContainerName         string `json:"containerName"`
	Image                 string `json:"image"`
	DesiredCount          int    `json:"desiredCount"`
	MinimumHealthyPercent int    `json:"minimumHealthyPercent"`
	MaximumPercent        int    `json:"maximumPercent"`
}

// Clone keeps immutable projections independent of mutable caller-owned pointers.
func (e *ECSDeploymentIntent) Clone() *ECSDeploymentIntent {
	if e == nil {
		return nil
	}
	copied := *e
	return &copied
}
