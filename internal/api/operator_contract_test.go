package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRootForContractTest(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func readRepoFile(t *testing.T, relativePath string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(repoRootForContractTest(t), relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}

	return string(body)
}

func TestDockerUpOperatorContract(t *testing.T) {
	makefile := readRepoFile(t, "Makefile")
	if !strings.Contains(makefile, "./scripts/docker-up.sh $(DOCKER_COMPOSE)") {
		t.Fatal("expected Makefile docker-up target to route through scripts/docker-up.sh")
	}

	script := readRepoFile(t, "scripts/docker-up.sh")
	for _, fragment := range []string{
		`port api "$PORT"`,
		`lsof -nP -iTCP:"$PORT" -sTCP:LISTEN`,
		"localhost:8080 is already occupied by something other than this Tenet compose stack",
		"lsof -nP -iTCP:8080 -sTCP:LISTEN",
		"make docker-down",
		"docker compose up --build -d",
		`exec "${compose_cmd[@]}" up --build -d`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected docker-up preflight script to include %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"If `make docker-up` exits before starting the stack, `localhost:8080` is already owned by something outside this Compose project.",
		"`lsof -nP -iTCP:8080 -sTCP:LISTEN`",
		"`make docker-down`",
		"`docker compose up --build -d`",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README docker troubleshooting to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"The `make docker-up` preflight intentionally fails fast when `localhost:8080` is already occupied by something other than this Compose stack.",
		"`lsof -nP -iTCP:8080 -sTCP:LISTEN`",
		"`make docker-down`",
		"`docker compose up --build -d`",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime doc troubleshooting to include %q", fragment)
		}
	}
}
