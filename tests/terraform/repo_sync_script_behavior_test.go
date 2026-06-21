package terraform_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRepoSyncScriptPreservesAnonymousPublicClone(t *testing.T) {
	result := runRenderedRepoSyncScript(t, repoSyncScriptCase{
		repoURL: "https://github.com/DevopsArtFactory/tenet-config.git",
		branch:  "main",
	})

	if result.exitCode != 0 {
		t.Fatalf("repo-sync script exited %d; stderr:\n%s\nstdout:\n%s", result.exitCode, result.stderr, result.stdout)
	}
	requireContains(t, "fake git log", result.gitLog,
		"cmd: clone --depth 1 --branch main https://github.com/DevopsArtFactory/tenet-config.git ",
		"cmd: -C "+result.targetDir+" fetch --depth 1 origin main",
		"cmd: -C "+result.targetDir+" reset --hard FETCH_HEAD",
		"askpass: unset",
	)
	requireNotContains(t, "fake git log", result.gitLog,
		"TENET_REPO_SYNC_TOKEN",
		"x-access-token",
		"ASKPASS_PASSWORD_OK",
	)
}

func TestRepoSyncScriptUsesAskpassForPrivateHTTPSWithoutLeakingToken(t *testing.T) {
	const token = "ghp_test_secret_token_value"
	result := runRenderedRepoSyncScript(t, repoSyncScriptCase{
		repoURL: "https://github.com/DevopsArtFactory/private-tenet-config.git",
		branch:  "release/2026-06",
		token:   token,
	})

	if result.exitCode != 0 {
		t.Fatalf("repo-sync script exited %d; stderr:\n%s\nstdout:\n%s", result.exitCode, result.stderr, result.stdout)
	}
	requireContains(t, "fake git log", result.gitLog,
		"cmd: clone --depth 1 --branch release/2026-06 https://github.com/DevopsArtFactory/private-tenet-config.git ",
		"askpass: set",
		"ASKPASS_USERNAME_OK",
		"ASKPASS_PASSWORD_OK",
		"terminal_prompt: 0",
	)
	requireNotContains(t, "fake git log", result.gitLog, token)
	requireNotContains(t, "repo-sync stdout", result.stdout, token)
	requireNotContains(t, "repo-sync stderr", result.stderr, token)
}

func TestRepoSyncScriptRejectsTokenForNonHTTPSRepoURL(t *testing.T) {
	const token = "ghp_test_secret_token_value"
	result := runRenderedRepoSyncScript(t, repoSyncScriptCase{
		repoURL: "git@github.com:DevopsArtFactory/private-tenet-config.git",
		branch:  "main",
		token:   token,
	})

	if result.exitCode == 0 {
		t.Fatalf("expected repo-sync script to reject token-backed non-HTTPS URL; stdout:\n%s\nstderr:\n%s", result.stdout, result.stderr)
	}
	requireContains(t, "repo-sync stderr", result.stderr, "TENET_REPO_SYNC_TOKEN requires an https:// repo_url")
	if strings.TrimSpace(result.gitLog) != "" {
		t.Fatalf("expected unsupported URL to fail before invoking git; git log:\n%s", result.gitLog)
	}
	requireNotContains(t, "repo-sync stderr", result.stderr, token)
	requireNotContains(t, "repo-sync stdout", result.stdout, token)
}

type repoSyncScriptCase struct {
	repoURL string
	branch  string
	token   string
}

type repoSyncScriptResult struct {
	exitCode  int
	stdout    string
	stderr    string
	gitLog    string
	targetDir string
}

func runRenderedRepoSyncScript(t *testing.T, tc repoSyncScriptCase) repoSyncScriptResult {
	t.Helper()

	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "workspace", "repo")
	gitLog := filepath.Join(tmp, "git.log")
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create fake bin dir: %v", err)
	}
	writeExecutable(t, filepath.Join(binDir, "git"), fakeGitScript(gitLog))
	writeExecutable(t, filepath.Join(binDir, "sleep"), "#!/bin/sh\nexit 0\n")

	script := renderRepoSyncTemplate(t, tc.repoURL, tc.branch, targetDir)
	scriptPath := filepath.Join(tmp, "repo-sync.sh")
	writeExecutable(t, scriptPath, script)

	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	if tc.token != "" {
		cmd.Env = append(cmd.Env, "TENET_REPO_SYNC_TOKEN="+tc.token)
	} else {
		cmd.Env = append(cmd.Env, "TENET_REPO_SYNC_TOKEN=")
	}
	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("run repo-sync script: %v", err)
		}
	}

	logBody, readErr := os.ReadFile(gitLog)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read fake git log: %v", readErr)
	}
	return repoSyncScriptResult{
		exitCode:  exitCode,
		stdout:    stdout.String(),
		stderr:    stderr.String(),
		gitLog:    string(logBody),
		targetDir: targetDir,
	}
}

func renderRepoSyncTemplate(t *testing.T, repoURL, branch, targetDir string) string {
	t.Helper()

	script := readRepoFile(t, "deploy/terraform/aws-central-control-plane/templates/repo-sync.sh.tftpl")
	replacements := map[string]string{
		"TARGET=/workspace/repo":      "TARGET=" + shellQuote(targetDir),
		"$${TENET_REPO_SYNC_TOKEN:-}": "${TENET_REPO_SYNC_TOKEN:-}",
		"${jsonencode(repo_url)}":     strconv.Quote(repoURL),
		"${jsonencode(repo_branch)}":  strconv.Quote(branch),
		"${sync_interval}":            "1",
		`  sleep "$SYNC_INTERVAL"`:    "  exit 0",
	}
	for old, replacement := range replacements {
		script = strings.ReplaceAll(script, old, replacement)
	}
	if strings.Contains(script, "${jsonencode(") || strings.Contains(script, "$${") || strings.Contains(script, "${sync_interval}") {
		t.Fatalf("repo-sync template still contains unrendered Terraform interpolation:\n%s", script)
	}
	return script
}

func fakeGitScript(logPath string) string {
	return fmt.Sprintf(`#!/bin/sh
set -eu
LOG=%s
{
  printf 'cmd:'
  for arg in "$@"; do
    printf ' %%s' "$arg"
  done
  printf '\n'
  if [ -n "${GIT_ASKPASS:-}" ]; then
    printf 'askpass: set\n'
    user="$($GIT_ASKPASS 'Username for https://github.com')"
    pass="$($GIT_ASKPASS 'Password for https://github.com')"
    [ "$user" = "x-access-token" ] && printf 'ASKPASS_USERNAME_OK\n'
    [ "$pass" = "${TENET_REPO_SYNC_TOKEN:-}" ] && printf 'ASKPASS_PASSWORD_OK\n'
    printf 'terminal_prompt: %%s\n' "${GIT_TERMINAL_PROMPT:-unset}"
  else
    printf 'askpass: unset\n'
  fi
} >>"$LOG"

last=
for arg in "$@"; do
  last=$arg
done
if [ "${1:-}" = "clone" ]; then
  mkdir -p "$last/.git"
fi
exit 0
`, shellQuote(logPath))
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
