package domain

// Deployment is the desired deployment intent for one application and environment.
type Deployment struct {
	Name           string
	AppRef         string
	EnvironmentRef string
	Generation     int64
}
