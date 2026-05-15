#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/ci-smoke-runner-evidence.sh <run-id>

Collect GitHub Actions CI Smoke runner-start evidence for a workflow run.

The report includes:
  - run summary with head SHA and URL
  - compact Runner-start verdict for likely pre-runner blockers
  - Actions jobs API output with runner_id, runner_name, and steps[]
  - failed workflow logs when GitHub exposes them (missing logs are reported, not fatal)
  - CI Smoke check-runs for Fast smoke, Docker smoke scope, and Docker smoke
  - check-run annotations that often carry billing/spending-limit runner-start reasons

Run from a gh-authenticated checkout of DevopsArtFactory/tenet.
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -ne 1 ]]; then
  usage >&2
  exit 2
fi

RUN_ID="$1"

if ! command -v gh >/dev/null 2>&1; then
  echo "error: gh CLI is required" >&2
  exit 127
fi

if ! gh auth status >/dev/null 2>&1; then
  echo "error: gh CLI is not authenticated" >&2
  exit 1
fi

section() {
  printf '\n== %s ==\n' "$1"
}

print_runner_start_verdict() {
  local run_id="$1"
  local sha="$2"
  local blocker_lines
  local check_runs

  section "Runner-start verdict"

  blocker_lines=$(gh api "repos/:owner/:repo/actions/runs/${run_id}/jobs" \
    --jq '.jobs[] | select((.name == "Fast smoke" or .name == "Docker smoke scope") and (((.runner_id // 0) == 0) or ((.runner_name // "") == "")) and (((.steps // []) | length) == 0)) | "- Likely pre-runner blocker: \(.name) has runner_id=\(.runner_id // 0), runner_name=\((.runner_name // "") | if . == "" then "<empty>" else . end), and steps=[] before repository commands ran."')
  if [[ -z "$blocker_lines" ]]; then
    echo "No likely pre-runner blocker found for Fast smoke or Docker smoke scope from runner assignment evidence."
  else
    printf '%s\n' "$blocker_lines"
  fi

  check_runs=$(gh api "repos/:owner/:repo/commits/${sha}/check-runs" \
    --jq '.check_runs[] | select(.name == "Fast smoke" or .name == "Docker smoke scope" or .name == "Docker smoke") | [.id, .name] | @tsv')
  local billing_lines=()
  local id
  local name
  local annotations
  local message
  while IFS=$'\t' read -r id name; do
    [[ -n "$id" ]] || continue
    annotations=$(gh api "repos/:owner/:repo/check-runs/${id}/annotations" \
      --jq '.[] | select((.message // "") | test("billing|spending limit|payments have failed|Billing & plans"; "i")) | .message')
    while IFS= read -r message; do
      [[ -n "$message" ]] || continue
      billing_lines+=("- Billing/spending-limit annotation on ${name}: ${message}")
    done <<< "$annotations"
  done <<< "$check_runs"

  if [[ ${#billing_lines[@]} -eq 0 ]]; then
    echo "No billing/spending-limit annotation found on CI Smoke check-runs."
  else
    printf '%s\n' "${billing_lines[@]}"
  fi
}

section "Run summary"
gh run view "$RUN_ID" --json databaseId,name,status,conclusion,headSha,url \
  --jq '{id:.databaseId,name,status,conclusion,headSha,url}'

SHA=$(gh run view "$RUN_ID" --json headSha --jq .headSha)

print_runner_start_verdict "$RUN_ID" "$SHA"

section "Actions jobs API runner assignment evidence"
gh api "repos/:owner/:repo/actions/runs/${RUN_ID}/jobs" \
  --jq '.jobs[] | {name,status,conclusion,runner_id,runner_name,steps}'

section "Failed workflow logs"
if ! gh run view "$RUN_ID" --log-failed; then
  echo "gh run view ${RUN_ID} --log-failed did not return downloadable failed logs."
  echo "For runner-start blockers this commonly appears as: log not found."
fi

section "CI Smoke check-runs"
gh api "repos/:owner/:repo/commits/${SHA}/check-runs" \
  --jq '.check_runs[] | select(.name == "Fast smoke" or .name == "Docker smoke scope" or .name == "Docker smoke") | {id,name,status,conclusion,details_url}'

section "CI Smoke check-run annotations"
CHECK_RUN_IDS=()
while IFS= read -r id; do
  [[ -n "$id" ]] && CHECK_RUN_IDS+=("$id")
done < <(gh api "repos/:owner/:repo/commits/${SHA}/check-runs" \
  --jq '.check_runs[] | select(.name == "Fast smoke" or .name == "Docker smoke scope" or .name == "Docker smoke") | .id')

if [[ ${#CHECK_RUN_IDS[@]} -eq 0 ]]; then
  echo "No CI Smoke check-runs found for ${SHA}."
else
  for id in "${CHECK_RUN_IDS[@]}"; do
    echo "-- check-run ${id} --"
    annotations=$(gh api "repos/:owner/:repo/check-runs/${id}/annotations" \
      --jq '.[] | {path,start_line,annotation_level,message}')
    if [[ -z "$annotations" ]]; then
      echo "No annotations returned."
    else
      printf '%s\n' "$annotations"
    fi
  done
fi
