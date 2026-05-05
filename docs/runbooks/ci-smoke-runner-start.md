# CI Smoke runner-start failures

Use this runbook when a GitHub Actions `CI Smoke` run fails before Tenet repository commands start. These failures are different from product, test, or smoke regressions because GitHub never assigns a runner to the job.

## Recognize the pattern

A runner-start failure typically has all of these signals:

- the failing jobs show `runner_id=0`
- the failing jobs have `steps=[]`
- `gh run view <run-id> --log-failed` returns no repository logs, often `log not found`
- check-run annotations mention an account-level condition, for example: `The job was not started because recent account payments have failed or your spending limit needs to be increased. Please check the 'Billing & plans' section in your settings`

When those signals are present, do not treat the red check as evidence that `go test ./...`, `make smoke-local`, or `make smoke-docker` failed. No Tenet command ran.

## Collect evidence

From the repository checkout, capture the workflow, job step, and annotation evidence before commenting on a PR or issue:

```bash
gh run view <run-id> --json databaseId,name,status,conclusion,headSha,url,jobs \
  --jq '{id:.databaseId,name,status,conclusion,headSha,url,jobs:[.jobs[]|{name,status,conclusion,runner_id,steps}]}'

gh run view <run-id> --log-failed
```

Capture the check-run annotations too, because GitHub often puts the account-level billing or spending-limit reason there instead of in downloadable logs:

```bash
SHA=$(gh run view <run-id> --json headSha --jq .headSha)
gh api "repos/:owner/:repo/commits/${SHA}/check-runs" \
  --jq '.check_runs[] | select(.name == "Fast smoke" or .name == "Docker smoke scope") | {id,name,status,conclusion,details_url}'

for id in $(gh api "repos/:owner/:repo/commits/${SHA}/check-runs" \
  --jq '.check_runs[] | select(.name == "Fast smoke" or .name == "Docker smoke scope") | .id'); do
  gh api "repos/:owner/:repo/check-runs/${id}/annotations" \
    --jq '.[] | {path,start_line,annotation_level,message}'
done
```

If the run is attached to a PR, also inspect the PR check rollup so reviewers can see that the failure happened before the lane reached repository commands:

```bash
gh pr view <pr-number> --json statusCheckRollup \
  --jq '.statusCheckRollup[] | select(.workflowName == "CI Smoke") | {name,status,conclusion,detailsUrl}'
```

## Triage decision

- If `runner_id=0`, `steps=[]`, and an account-level billing/spending-limit annotation is present, classify the incident as external GitHub Actions runner availability. Track it on issue #850 or the current runner-availability issue instead of opening a product-fix PR.
- If the job has normal steps and logs, diagnose it as an ordinary CI failure: read the failed command output, reproduce locally, and fix the repo-side regression.

## Temporary PR evidence

While runner availability is blocked, PR comments should be explicit that local verification is a temporary substitute for the missing hosted signal. Include concrete commands and results, for example:

```text
CI Smoke failed before repository commands ran: Fast smoke and Docker smoke scope have runner_id=0 and steps=[].
Local verification on this branch is green:
- unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./...
- unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; make smoke-local
- unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; make smoke-docker   # when the slice touches Docker-smoke scope
```

## Recovery after billing/spending-limit is fixed

After the GitHub account condition is cleared:

1. Choose the rerun target that proves the signal you are trying to restore:

   ```bash
   gh run list --workflow "CI Smoke" --branch main --limit 5
   ```

   - For post-merge release readiness, rerun the newest failed `CI Smoke` run on `main`. Do not use an old PR rerun as evidence that the `main` release signal recovered.
   - For a still-open affected PR, rerun that PR head run/check so reviewers get signal on the exact branch they are reviewing.

2. Re-run the selected failed workflow run:

   ```bash
   gh run rerun <run-id> --failed
   gh run watch <run-id>
   ```

   If the old run can no longer be rerun, dispatch a fresh smoke run from the workflow page or the CLI:

   ```bash
   gh workflow run "CI Smoke" --ref main
   ```

3. Verify the rerun has non-empty job steps and a real runner assignment.
4. Confirm the fast lane reaches `go test ./...`, smoke script syntax checks, and `make smoke-local`.
5. On `main` or Docker-scoped PRs, confirm the Docker lane reaches `make smoke-docker` or produces normal actionable logs.
6. Comment on issue #850 with the recovered run URL and whether the repository commands passed or exposed a new actionable repo-side failure.
