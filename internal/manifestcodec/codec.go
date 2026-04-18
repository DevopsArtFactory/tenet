package manifestcodec

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Decode applies Tenet's manifest decoding rules based on file extension.
func Decode(name string, content []byte, out any) error {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json":
		return json.Unmarshal(content, out)
	case ".yaml", ".yml":
		return yaml.Unmarshal(content, out)
	default:
		return fmt.Errorf("unsupported manifest extension %q", filepath.Ext(name))
	}
}

// IsManifestFile reports whether name has a supported manifest extension.
func IsManifestFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json", ".yaml", ".yml":
		return true
	default:
		return false
	}
}
