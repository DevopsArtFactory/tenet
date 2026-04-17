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

func TestReadOnlyApplyOperatorContract(t *testing.T) {
	script := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`TENET_APPLY_STORE_PATH="$APPLY_STORE_PATH"`,
		`curl -fsS -X POST "http://$HOST:$PORT/v1/applies/plans/$plan_id"`,
		`curl -fsS "http://$HOST:$PORT/v1/applies/$apply_id"`,
		`assert apply_record['error'] == 'apply execution is disabled', apply_record`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local apply contract to include %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`POST /v1/applies/plans/{planId}`",
		"`GET /v1/applies/{applyId}`",
		"`TENET_APPLY_STORE_PATH`",
		"disabled by default for live execution",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README apply contract to include %q", fragment)
		}
	}

	uiDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"`POST /v1/applies/plans/{planId}`",
		"`GET /v1/applies/{applyId}`",
		"failed immutable contract with operator-facing error/details",
	} {
		if !strings.Contains(uiDoc, fragment) {
			t.Fatalf("expected overview UI doc apply contract to include %q", fragment)
		}
	}
}

func TestAuditLogOperatorContract(t *testing.T) {
	script := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`curl -fsS "http://$HOST:$PORT/v1/audit-logs"`,
		`assert any(entry['action'] == 'apply.requested' for entry in audit_logs['entries']), audit_logs`,
		`assert entry['actor']['available'] is False, entry`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local audit contract to include %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`GET /v1/audit-logs?limit={n}`",
		"actor information is unavailable because the immutable plan preview and apply request stores do not persist operator identity",
		"`/v1/audit-logs` is the preferred v1 route shape for this local timeline",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README audit contract to include %q", fragment)
		}
	}

	auditDoc := readRepoFile(t, "docs/architecture/tenet-v1-audit-log-api.md")
	for _, fragment := range []string{
		"`GET /v1/audit-logs`",
		"`limit` is optional and must be a positive integer when present",
		"`actor.available` is always `false` in this slice",
	} {
		if !strings.Contains(auditDoc, fragment) {
			t.Fatalf("expected audit architecture doc to include %q", fragment)
		}
	}
}
