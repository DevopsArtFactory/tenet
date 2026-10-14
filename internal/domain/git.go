package domain

// GitSource identifies the immutable repository revision used for reviewed inputs.
// Authentication tokens are never part of this metadata.
type GitSource struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
	Path       string `json:"path,omitempty"`
	Revision   string `json:"revision"`
}

func (s *GitSource) Clone() *GitSource {
	if s == nil {
		return nil
	}
	copy := *s
	return &copy
}
