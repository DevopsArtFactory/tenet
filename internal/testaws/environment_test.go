package testaws_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/testaws"
	"github.com/aws/aws-sdk-go-v2/config"
)

func TestMain(m *testing.M) {
	os.Exit(testaws.Run(m))
}

func assertIsolatedEnvironment(t *testing.T, environ []string) {
	t.Helper()
	actual := make(map[string]string)
	for _, entry := range environ {
		name, value, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "AWS_") {
			actual[name] = value
		}
	}
	want := map[string]string{
		"AWS_CONFIG_FILE":             os.DevNull,
		"AWS_SHARED_CREDENTIALS_FILE": os.DevNull,
		"AWS_EC2_METADATA_DISABLED":   "true",
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatal("test process retained ambient AWS settings or lost provider isolation")
	}
}

func TestAmbientAWSIsDiscarded(t *testing.T) {
	if os.Getenv("TENET_TESTAWS_CHILD") == "1" {
		assertIsolatedEnvironment(t, os.Environ())
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Region != "" {
			t.Fatal("inherited region or profile was loaded")
		}
		if _, err := cfg.Credentials.Retrieve(ctx); err == nil {
			t.Fatal("SDK recovered credentials from an ambient provider")
		}
		if ctx.Err() != nil {
			t.Fatal("credential lookup waited for an external provider")
		}
		return
	}

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "ambient provider must not be contacted", http.StatusForbidden)
	}))
	defer server.Close()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	credentialsPath := filepath.Join(dir, "credentials")
	if err := os.WriteFile(configPath, []byte("[default]\nregion = us-west-1\n[profile personal]\nregion = us-west-2\ncredential_process = must-not-execute-personal-credential-process\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credentialsPath, []byte("[default]\naws_access_key_id = fixture-file-key\naws_secret_access_key = fixture-file-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// All values are fixtures: a regression must never use real credentials or endpoints.
	environ := append(os.Environ(),
		"TENET_TESTAWS_CHILD=1",
		"AWS_ACCESS_KEY_ID=fixture-env-key", "AWS_SECRET_ACCESS_KEY=fixture-env-secret",
		"AWS_SESSION_TOKEN=fixture-session", "AWS_PROFILE=personal", "AWS_DEFAULT_PROFILE=personal",
		"AWS_REGION=us-east-1", "AWS_DEFAULT_REGION=us-east-2", "AWS_SDK_LOAD_CONFIG=true",
		"AWS_CONFIG_FILE="+configPath, "AWS_SHARED_CREDENTIALS_FILE="+credentialsPath,
		"AWS_ROLE_ARN=arn:aws:iam::123456789012:role/fixture", "AWS_ROLE_SESSION_NAME=fixture",
		"AWS_WEB_IDENTITY_TOKEN_FILE="+filepath.Join(dir, "must-not-read-token"),
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI=/fixture",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI="+server.URL,
		"AWS_CONTAINER_AUTHORIZATION_TOKEN=fixture-token",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE="+filepath.Join(dir, "must-not-read-container-token"),
		"AWS_EC2_METADATA_DISABLED=false", "AWS_EC2_METADATA_SERVICE_ENDPOINT="+server.URL,
		"AWS_ENDPOINT_URL="+server.URL, "AWS_ENDPOINT_URL_STS="+server.URL,
	)
	t.Run("go test suite", func(t *testing.T) {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestAmbientAWSIsDiscarded$")
		cmd.Env = environ
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated suite failed: %v\n%s", err, output)
		}
	})
	t.Run("local smoke shell", func(t *testing.T) {
		bash, err := exec.LookPath("bash")
		if err != nil {
			t.Skip("bash is required for smoke-local")
		}
		cmd := exec.CommandContext(t.Context(), bash, "-eu", "-c", `source "$1"; for name in ${!AWS_@}; do printf '%s=%s\n' "$name" "${!name}"; done`, "test-aws-env", "../../scripts/test-aws-env.sh")
		cmd.Env = environ
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("isolated smoke environment failed: %v\n%s", err, output)
		}
		assertIsolatedEnvironment(t, strings.Split(strings.TrimSpace(string(output)), "\n"))
	})
	if requests.Load() != 0 {
		t.Fatalf("ambient AWS providers received %d requests", requests.Load())
	}
}
