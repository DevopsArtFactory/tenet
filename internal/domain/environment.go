package domain

// Environment binds a logical environment to placement information.
type Environment struct {
	Name       string
	AccountRef string
	Region     string
	NetworkRef string
	PolicyRef  string
}
