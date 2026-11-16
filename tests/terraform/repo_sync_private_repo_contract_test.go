package terraform_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func readRepoFile(t *testing.T, relativePath string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(repoRoot(t), relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}

	return string(body)
}

func requireContains(t *testing.T, name, body string, fragments ...string) {
	t.Helper()

	for _, fragment := range fragments {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected %s to include %q", name, fragment)
		}
	}
}

func requireNotContains(t *testing.T, name, body string, fragments ...string) {
	t.Helper()

	for _, fragment := range fragments {
		if strings.Contains(body, fragment) {
			t.Fatalf("expected %s not to include stale fragment %q", name, fragment)
		}
	}
}

func TestAWSCentralControlPlaneRepoSyncPrivateRepoContract(t *testing.T) {
	variables := readRepoFile(t, "deploy/terraform/aws-central-control-plane/variables.tf")
	requireContains(t, "variables.tf", variables,
		`variable "repo_sync_token_secret_arn"`,
		`description = "Optional AWS Secrets Manager secret ARN containing an HTTPS token for the repo-sync sidecars. Leave null to keep anonymous public clone behavior."`,
		`default     = null`,
	)
	requireNotContains(t, "variables.tf", variables,
		"Private desired-state repo authentication is not wired into this module yet.",
	)

	main := readRepoFile(t, "deploy/terraform/aws-central-control-plane/main.tf")
	requireContains(t, "main.tf", main,
		`repo_sync_token_secrets = var.repo_sync_token_secret_arn == null ? [] : [`,
		`name      = "TENET_REPO_SYNC_TOKEN"`,
		`valueFrom = var.repo_sync_token_secret_arn`,
		`aws_iam_role_policy" "execution_repo_sync_secret"`,
		`count = var.repo_sync_token_secret_arn == null ? 0 : 1`,
		`Action   = ["secretsmanager:GetSecretValue"]`,
		`Resource = var.repo_sync_token_secret_arn`,
		`secrets = local.repo_sync_token_secrets`,
		`depends_on = [terraform_data.image_build, aws_iam_role_policy_attachment.execution, aws_iam_role_policy.execution_auth_secrets, aws_iam_role_policy.execution_repo_sync_secret`,
	)

	if strings.Count(main, `secrets = local.repo_sync_token_secrets`) != 2 {
		t.Fatalf("expected both repo-sync sidecars to receive the optional token secret; got %d injections", strings.Count(main, `secrets = local.repo_sync_token_secrets`))
	}

	script := readRepoFile(t, "deploy/terraform/aws-central-control-plane/templates/repo-sync.sh.tftpl")
	requireContains(t, "repo-sync.sh.tftpl", script,
		`if [ -n "$${TENET_REPO_SYNC_TOKEN:-}" ]; then`,
		`TENET_REPO_SYNC_TOKEN requires an https:// repo_url`,
		`GIT_ASKPASS=/tmp/tenet-git-askpass`,
		`git clone --depth 1 --branch "$REPO_BRANCH" "$REPO_URL" "$TARGET"`,
		`git -C "$TARGET" fetch --depth 1 origin "$REPO_BRANCH"`,
	)
	requireNotContains(t, "repo-sync.sh.tftpl", script,
		`https://$TENET_REPO_SYNC_TOKEN@`,
		`x-access-token:${TENET_REPO_SYNC_TOKEN}@`,
	)

	tfvars := readRepoFile(t, "deploy/terraform/aws-central-control-plane/terraform.tfvars.example")
	requireContains(t, "terraform.tfvars.example", tfvars,
		`# repo_sync_token_secret_arn = "arn:aws:secretsmanager:ap-northeast-2:123456789012:secret:tenet/repo-sync-token-AbCdEf"`,
	)
	requireNotContains(t, "terraform.tfvars.example", tfvars,
		"Private desired-state repo authentication is not wired into this module yet.",
	)

	readme := readRepoFile(t, "deploy/terraform/aws-central-control-plane/README.md")
	requireContains(t, "README.md", readme,
		"`repo_sync_token_secret_arn` is optional. Leave it unset for anonymously readable public HTTPS repos.",
		"When set, ECS injects the secret value as `TENET_REPO_SYNC_TOKEN` only into the two `repo-sync` sidecars, and the ECS execution role receives `secretsmanager:GetSecretValue` for that secret ARN.",
		"The repo-sync script keeps using the plain `repo_url` when no token is present, so public anonymous clones continue to work.",
	)
	requireNotContains(t, "README.md", readme,
		"Private desired-state repo authentication is not wired into this module yet.",
	)

	architectureDoc := readRepoFile(t, "docs/architecture/tenet-v1-aws-central-control-plane.md")
	requireContains(t, "tenet-v1-aws-central-control-plane.md", architectureDoc,
		"Set `repo_sync_token_secret_arn` to an AWS Secrets Manager secret ARN when the filesystem-mode desired-state repo is private.",
		"Leave `repo_sync_token_secret_arn` unset for public repos; the repo-sync sidecars keep using anonymous HTTPS clone/fetch.",
	)
	requireNotContains(t, "tenet-v1-aws-central-control-plane.md", architectureDoc,
		"Private desired-state repo authentication is not wired into this module yet.",
		"private desired-state repo authentication is not wired into this module yet",
	)

	operatorDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	requireContains(t, "tenet-v1-operator-contracts.md", operatorDoc,
		"The AWS central-control-plane Terraform bootstrap can optionally inject a Secrets Manager-backed `TENET_REPO_SYNC_TOKEN` into its two repo-sync sidecars with `repo_sync_token_secret_arn`.",
		"Leaving that ARN unset preserves anonymous public clone behavior.",
	)
}
