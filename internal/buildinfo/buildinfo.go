package buildinfo

import "runtime"

// Info captures build and runtime metadata exposed by the API.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Go      string `json:"go"`
}

// Current returns the active build information.
func Current(version, commit string) Info {
	return Info{
		Version: version,
		Commit:  commit,
		Go:      runtime.Version(),
	}
}
