// Package testaws isolates test processes from ambient AWS configuration.
// Import it only from tests; production credential discovery is unchanged.
package testaws

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// Run starts a suite without inherited AWS credentials, profiles, endpoints or
// metadata providers. Tests may explicitly configure mock providers with t.Setenv.
// Emulator tests load their own files from TENET_EMULATOR_STATE.
func Run(m *testing.M) int {
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "AWS_") {
			if err := os.Unsetenv(name); err != nil {
				fmt.Fprintln(os.Stderr, "isolate AWS test environment:", err)
				return 1
			}
		}
	}
	for name, value := range map[string]string{
		"AWS_CONFIG_FILE":             os.DevNull,
		"AWS_SHARED_CREDENTIALS_FILE": os.DevNull,
		"AWS_EC2_METADATA_DISABLED":   "true",
	} {
		if err := os.Setenv(name, value); err != nil {
			fmt.Fprintln(os.Stderr, "isolate AWS test environment:", err)
			return 1
		}
	}
	return m.Run()
}
