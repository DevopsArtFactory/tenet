# CI Smoke runner-start failures

Use this runbook when a GitHub Actions `CI Smoke` run fails before Tenet repository commands start. These failures are different from product, test, or smoke regressions because GitHub never assigns a runner to the job.

## Recognize the pattern

A runner-start failure typically has one of these evidence shapes:

- the failing jobs show `runner_id=0`
- the failing jobs have `steps=[]`
- the Actions jobs API returns `jobs: []` for a completed `startup_failure` run before GitHub creates per-job runner records
- `gh run view <run-id> --log-failed` returns no repository logs, often `log not found`
- check-run annotations mention an account-level condition, for example: `The job was not started because recent account payments have failed or your spending limit needs to be increased. Please check the 'Billing & plans' section in your settings`

When those signals are present, do not treat the red check as evidence that `go test ./...`, `make smoke-local`, or `make smoke-docker` failed. No Tenet command ran.

## Collect evidence

From the repository checkout, capture the workflow summary, failed logs, Actions job assignment evidence, and annotations before commenting on a PR or issue. The helper below prints the same evidence bundle and treats missing downloadable failed logs as a runner-start signal to record rather than as a script failure:

```bash
scripts/ci-smoke-runner-evidence.sh <run-id>
```

Keep `scripts/ci-smoke-runner-evidence.sh` directly executable because operators copy and run the path from this runbook during runner-start incidents.

The helper also prints a compact `Runner-start verdict` section before the raw evidence. Treat a line as a likely pre-runner blocker when `Fast smoke` or `Docker smoke scope` has `runner_id=0` or an empty `runner_name`, and `steps=[]`; that means the lane did not reach repository commands. If the Actions jobs API returns `jobs: []` for a completed `startup_failure` run, treat that as the newer no-runner-record variant of the same blocker rather than as missing helper evidence. If `gh run view --log-failed` returns `log not found`, record that missing-log line as evidence that GitHub never produced step logs; do not keep rerunning or debugging the repository just to chase logs that cannot exist before runner assignment. The same verdict section calls out any billing/spending-limit annotation found on the CI Smoke check-runs, preferring check-run rows whose `details_url` belongs to the requested run ID when GitHub exposes those links, so reviewers do not have to infer the account-level blocker from raw JSON or stale same-SHA attempts alone.

For a pasteable PR or issue comment, use the concise Markdown mode:

```bash
scripts/ci-smoke-runner-evidence.sh --summary <run-id>
```

`--summary` and `--markdown` are aliases. The Markdown summary keeps the run URL, explicit run ID, workflow/run name, event (`push`, `pull_request`, or manual dispatch), head branch, head SHA, and current status/conclusion plus attempt and created/updated timestamps, run-specific `Fast smoke` and `Docker smoke scope` runner assignment evidence (job ID, `runner_id`, `runner_name`, and `steps` count) when check-run links can be matched to the requested run ID, a Docker smoke skipped note when that check-run is skipped, a missing failed-log note when `gh run view --log-failed` cannot download logs, and any billing/spending-limit annotations found on the CI Smoke check-runs.

The helper exits `0` after collecting evidence even when failed logs are missing, because missing logs are part of the runner-start evidence bundle. Usage errors exit `2`, missing `gh` exits `127`, and GitHub auth/API lookup failures exit non-zero so operators can distinguish local tooling problems from a real pre-runner CI result.

The CI Smoke fast lane syntax-checks this helper with `bash -n` alongside the local and Docker smoke scripts. If an edit breaks the evidence collector itself, expect the hosted fast lane to fail before operators need it during a runner-start incident.

If you need to collect each piece manually or paste individual commands into a PR comment, use the same sequence:

```bash
gh run view <run-id> --json databaseId,name,workflowName,event,status,conclusion,attempt,headBranch,headSha,url,createdAt,updatedAt \
  --jq '{id:.databaseId,name,workflowName,event,status,conclusion,attempt,headBranch,headSha,url,createdAt,updatedAt}'

gh api "repos/:owner/:repo/actions/runs/<run-id>/jobs" \
  --jq '.jobs[] | {id,name,status,conclusion,runner_id,runner_name,steps}'

gh run view <run-id> --log-failed
```

Use the Actions jobs API for `runner_id` / `runner_name` evidence; `gh run view --json jobs` is still useful for a compact step list, but it does not expose the runner assignment fields needed to prove the pre-runner failure mode.

Capture the check-run annotations too, because GitHub often puts the account-level billing or spending-limit reason there instead of in downloadable logs:

```bash
SHA=$(gh run view <run-id> --json headSha --jq .headSha)
gh api --paginate "repos/:owner/:repo/commits/${SHA}/check-runs" \
  --jq '.check_runs[] | select(.name == "Fast smoke" or .name == "Docker smoke scope" or .name == "Docker smoke") | {id,name,status,conclusion,details_url}'

for id in $(gh api --paginate "repos/:owner/:repo/commits/${SHA}/check-runs" \
  --jq '.check_runs[] | select(.name == "Fast smoke" or .name == "Docker smoke scope" or .name == "Docker smoke") | .id'); do
  gh api --paginate "repos/:owner/:repo/check-runs/${id}/annotations" \
    --jq '.[] | {path,start_line,annotation_level,message}'
done
```

If the run is attached to a PR, also inspect the PR check rollup so reviewers can see that the failure happened before the lane reached repository commands:

```bash
gh pr view <pr-number> --json statusCheckRollup \
  --jq '.statusCheckRollup[] | select(.workflowName == "CI Smoke") | {name,status,conclusion,detailsUrl}'
```

## Triage decision

- If `runner_id=0`, `steps=[]`, and an account-level billing/spending-limit annotation is present, classify the incident as external GitHub Actions runner availability. Track it on issue #1661 or the current runner-availability issue instead of opening a product-fix PR.
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

Before adding another runner-start note, check the latest comments on the PR and the active runner-availability issue. Do not post duplicate evidence for the same run URL, head SHA, attempt, and failure shape; a newer rerun only needs a fresh comment when it changes the head, reaches a different lane, or produces a materially different runner/log signal.

## Avoid false recovery signals

A rerun request alone is not recovery evidence. Before closing #1661 or telling a reviewer that hosted CI recovered, verify the current run or replacement run has all of these signals:

- at least one real runner assignment (`runner_id` is not `0`, or `runner_name` is populated)
- non-empty `steps[]` that include repository setup or checkout work
- either passing checks or normal downloadable/actionable repository logs from failed commands

If the rerun still shows `runner_id=0`, `steps=[]`, or only account-level billing/spending-limit annotations, keep the incident classified as external runner-start availability. Do not open repo-side product fixes from a run that never reached repository commands.

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
   gh run watch <run-id> --exit-status
   ```

   If `gh run rerun` reports that the workflow is already running, do not start a second ad-hoc run; watch the in-progress attempt to a terminal state, then collect fresh evidence from that same run ID so the issue history does not mix duplicate reruns with the recovery check.

   If the old run can no longer be rerun, dispatch a fresh smoke run from the workflow page or the CLI:

   ```bash
   gh workflow run "CI Smoke" --ref main
   ```

3. Verify the rerun or already-running attempt has non-empty job steps and a real runner assignment.
4. Confirm the fast lane reaches `go test ./...`, smoke script syntax checks, and `make smoke-local`.
5. On `main` or Docker-scoped PRs, confirm the Docker lane reaches `make smoke-docker` or produces normal actionable logs.
6. Comment on issue #1661 with the recovered run URL and whether the repository commands passed or exposed a new actionable repo-side failure.

Treat PR-head recovery and `main` recovery as separate signals. A green or actionable PR-head rerun tells reviewers whether a specific branch is reviewable; it does not close the release-readiness gap for `main`. A green or actionable `main` rerun proves the post-merge release signal recovered, but it does not replace required PR-head evidence for an open branch whose checks were blocked.

## Post-recovery release-health checklist

Use this checklist for the first `main` run that starts after the account-level runner blocker is fixed. The goal is to prove the hosted release signal recovered before anyone treats remaining red checks as product regressions or closes issue #1661.

- Rerun the newest failed `main` `CI Smoke` run, or dispatch a fresh `CI Smoke` run on `main` if GitHub no longer allows rerunning the old run.
- Confirm `Fast smoke` and `Docker smoke scope` have non-empty `steps[]` plus a real runner assignment. A recovered run must show repository steps such as checkout/setup instead of the pre-runner `runner_id=0` / `steps=[]` pattern.
- Confirm the fast lane reaches the repository commands documented in `.github/workflows/ci-smoke.yml`: `go test ./...`, `bash -n scripts/smoke-local.sh`, `bash -n scripts/smoke-docker.sh`, `bash -n scripts/ci-smoke-runner-evidence.sh`, and `make smoke-local`.
- For `main`, confirm the Docker lane either reaches `make smoke-docker` or fails with normal downloadable logs from repository commands. Do not require the Docker lane on unrelated PRs just to prove runner recovery; keep the existing CI cost profile intact.
- If any command fails after steps are non-empty, classify it as a normal actionable CI failure: read the logs, reproduce locally, and open or link the repo-side follow-up. Do not keep labeling post-recovery command failures as the billing/spending-limit blocker.
- Leave one concrete #1661 comment with the run URL, the non-empty `steps[]` evidence, the first repository command reached in each lane, and the follow-up issue/PR for any remaining product failure. Close #1661 only after this evidence is recorded.

## Closeout comment template

Use a closeout comment like this before closing the runner-availability issue:

```text
Runner availability recovered for CI Smoke run <run-url>.

Evidence:
- Fast smoke and Docker smoke scope now have non-empty runner assignments and steps[].
- Fast smoke reached repository commands: go test ./..., smoke script syntax checks, and make smoke-local.
- Docker smoke reached make smoke-docker or produced normal actionable logs.
- Outcome: <passed | failed with actionable repo-side failure>.
- Follow-up: <none | opened issue/PR #... for the new repo-side failure>.
```

Only close the runner-start blocker when the recovered run proves GitHub assigned runners and reached repository commands. If the rerun exposes a normal Tenet failure, keep that evidence on the runner issue, open or link the repo-side follow-up, and close only the external runner-availability portion.
