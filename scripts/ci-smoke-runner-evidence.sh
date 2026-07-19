#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/ci-smoke-runner-evidence.sh [--summary|--markdown] <run-id>

Collect GitHub Actions CI Smoke runner-start evidence for a workflow run.
Use --summary or --markdown to print a concise Markdown block for PR/issue comments.

The report includes:
  - run summary with head SHA, attempt, timestamps, and URL
  - compact Runner-start verdict for likely pre-runner blockers
  - Actions jobs API output with runner_id, runner_name, and steps[]
  - failed workflow logs when GitHub exposes them (missing logs are reported, not fatal)
  - CI Smoke check-runs for Fast smoke, Docker smoke scope, and Docker smoke
    (filtered to the requested run when check-run details_url links expose the run ID)
  - check-run annotations that often carry billing/spending-limit runner-start reasons

Run from a gh-authenticated checkout of DevopsArtFactory/tenet.
USAGE
}

SUMMARY_MODE=false

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ "${1:-}" == "--summary" || "${1:-}" == "--markdown" ]]; then
  SUMMARY_MODE=true
  shift
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

ci_smoke_check_runs_tsv() {
  local sha="$1"
  local run_id="${2:-}"
  local rows
  local filtered

  rows=$(gh api --paginate "repos/:owner/:repo/commits/${sha}/check-runs" \
    --jq '.check_runs[] | select(.name == "Fast smoke" or .name == "Docker smoke scope" or .name == "Docker smoke") | [.id, .name, (.status // ""), (.conclusion // ""), (.details_url // "")] | @tsv')

  if [[ -n "$run_id" ]]; then
    filtered=$(printf '%s\n' "$rows" | awk -F '\t' -v run_id="$run_id" '$5 ~ "/runs/" run_id "/" {print}')
    if [[ -n "$filtered" ]]; then
      printf '%s\n' "$filtered"
      return
    fi
  fi

  printf '%s\n' "$rows"
}

print_billing_annotations() {
  local check_runs="$1"
  local prefix="$2"
  local id
  local name
  local status
  local conclusion
  local details_url
  local annotations
  local message
  local found=false

  while IFS=$'\t' read -r id name status conclusion details_url; do
    [[ -n "$id" ]] || continue
    annotations=$(gh api --paginate "repos/:owner/:repo/check-runs/${id}/annotations" \
      --jq '.[] | select((.message // "") | test("billing|spending limit|payments have failed|Billing & plans"; "i")) | .message')
    while IFS= read -r message; do
      [[ -n "$message" ]] || continue
      found=true
      printf '%s Billing/spending-limit annotation on %s: %s\n' "$prefix" "$name" "$message"
    done <<< "$annotations"
  done <<< "$check_runs"

  if [[ "$found" != true ]]; then
    printf '%s No billing/spending-limit annotation found on CI Smoke check-runs.\n' "$prefix"
  fi
}

print_runner_start_verdict() {
  local run_id="$1"
  local sha="$2"
  local blocker_lines
  local check_runs
  local job_count

  section "Runner-start verdict"

  job_count=$(gh api "repos/:owner/:repo/actions/runs/${run_id}/jobs" --jq '(.jobs // []) | length')
  if [[ "$job_count" == "0" ]]; then
    echo "- Likely pre-runner blocker: Actions jobs API returned jobs: [] (startup_failure/no runner records), so no repository commands ran."
  fi

  blocker_lines=$(gh api "repos/:owner/:repo/actions/runs/${run_id}/jobs" \
    --jq '.jobs[] | select((.name == "Fast smoke" or .name == "Docker smoke scope") and (((.runner_id // 0) == 0) or ((.runner_name // "") == "")) and (((.steps // []) | length) == 0)) | "- Likely pre-runner blocker: \(.name) has runner_id=\(.runner_id // 0), runner_name=\((.runner_name // "") | if . == "" then "<empty>" else . end), and steps=[] before repository commands ran."')
  if [[ -z "$blocker_lines" && "$job_count" != "0" ]]; then
    echo "No likely pre-runner blocker found for Fast smoke or Docker smoke scope from runner assignment evidence."
  elif [[ -n "$blocker_lines" ]]; then
    printf '%s\n' "$blocker_lines"
  fi

  check_runs=$(ci_smoke_check_runs_tsv "$sha" "$run_id")
  print_billing_annotations "$check_runs" "-"
}

print_markdown_summary() {
  local run_id="$1"
  local run_json
  local sha
  local url
  local attempt
  local created_at
  local updated_at
  local check_runs
  local jobs
  local docker_smoke
  local log_output

  run_json=$(gh run view "$run_id" --json databaseId,name,status,conclusion,attempt,headSha,url,createdAt,updatedAt \
    --jq '{id:.databaseId,name,status,conclusion,attempt,headSha,url,createdAt,updatedAt}')
  sha=$(gh run view "$run_id" --json headSha --jq .headSha)
  url=$(gh run view "$run_id" --json url --jq .url)
  attempt=$(gh run view "$run_id" --json attempt --jq .attempt)
  created_at=$(gh run view "$run_id" --json createdAt --jq .createdAt)
  updated_at=$(gh run view "$run_id" --json updatedAt --jq .updatedAt)
  check_runs=$(ci_smoke_check_runs_tsv "$sha" "$run_id")

  printf '### CI Smoke runner-start evidence\n\n'
  printf -- '- Run: %s\n' "$url"
  printf -- '- Head SHA: `%s`\n' "$sha"
  printf -- '- Attempt: `%s`\n' "$attempt"
  printf -- '- Created: `%s`\n' "$created_at"
  printf -- '- Updated: `%s`\n' "$updated_at"
  printf '\n'

  printf 'Run summary:\n\n'
  printf '```json\n%s\n```\n\n' "$run_json"

  printf 'Runner assignment evidence:\n\n'
  printf '| Scope | Status | Conclusion | runner_id | runner_name | steps |\n'
  printf '| --- | --- | --- | --- | --- | --- |\n'
  jobs=$(gh api "repos/:owner/:repo/actions/runs/${run_id}/jobs" \
    --jq '.jobs[] | select(.name == "Fast smoke" or .name == "Docker smoke scope") | "| \(.name) | \(.status // "") | \(.conclusion // "") | \(.runner_id // 0) | \((.runner_name // "") | if . == "" then "<empty>" else . end) | \((.steps // []) | length) |"')
  if [[ -z "$jobs" ]]; then
    echo '| Fast smoke / Docker smoke scope | unavailable | unavailable | unavailable | unavailable | unavailable |'
    printf '\n'
    printf -- '- Actions jobs API returned `jobs: []` (`startup_failure`/no runner records), so no repository commands ran.\n'
  else
    printf '%s\n' "$jobs"
  fi
  printf '\n'

  docker_smoke=$(printf '%s\n' "$check_runs" | awk -F '\t' '$2 == "Docker smoke" && $4 == "skipped" {print "- Docker smoke: skipped (" $3 "/" $4 ") " $5}')
  if [[ -n "$docker_smoke" ]]; then
    printf '%s\n\n' "$docker_smoke"
  fi

  if ! log_output=$(gh run view "$run_id" --log-failed 2>&1); then
    printf -- '- Missing failed logs: `gh run view %s --log-failed` did not return downloadable failed logs' "$run_id"
    if [[ -n "$log_output" ]]; then
      printf ' (`%s`)' "$(printf '%s' "$log_output" | tr '\n' ' ' | sed 's/[[:space:]]*$//')"
    fi
    printf '.\n\n'
  fi

  printf 'Billing/spending-limit annotations:\n\n'
  print_billing_annotations "$check_runs" "-"
}

if [[ "$SUMMARY_MODE" == true ]]; then
  print_markdown_summary "$RUN_ID"
  exit 0
fi

section "Run summary"
gh run view "$RUN_ID" --json databaseId,name,status,conclusion,attempt,headSha,url,createdAt,updatedAt \
  --jq '{id:.databaseId,name,status,conclusion,attempt,headSha,url,createdAt,updatedAt}'

SHA=$(gh run view "$RUN_ID" --json headSha --jq .headSha)
CHECK_RUNS=$(ci_smoke_check_runs_tsv "$SHA" "$RUN_ID")

print_runner_start_verdict "$RUN_ID" "$SHA"

section "Actions jobs API runner assignment evidence"
ACTIONS_JOBS_OUTPUT=$(gh api "repos/:owner/:repo/actions/runs/${RUN_ID}/jobs" \
  --jq '.jobs[] | {name,status,conclusion,runner_id,runner_name,steps}')
if [[ -z "$ACTIONS_JOBS_OUTPUT" ]]; then
  echo "Actions jobs API returned jobs: [] (startup_failure/no runner records), so no repository commands ran."
else
  printf '%s\n' "$ACTIONS_JOBS_OUTPUT"
fi

section "Failed workflow logs"
if ! gh run view "$RUN_ID" --log-failed; then
  echo "gh run view ${RUN_ID} --log-failed did not return downloadable failed logs."
  echo "For runner-start blockers this commonly appears as: log not found."
fi

section "CI Smoke check-runs"
if [[ -z "$CHECK_RUNS" ]]; then
  echo "No CI Smoke check-runs found for ${SHA}."
else
  while IFS=$'\t' read -r id name status conclusion details_url; do
    [[ -n "$id" ]] || continue
    printf '{"id":%s,"name":"%s","status":"%s","conclusion":"%s","details_url":"%s"}\n' \
      "$id" "$name" "$status" "$conclusion" "$details_url"
  done <<< "$CHECK_RUNS"
fi

section "CI Smoke check-run annotations"
CHECK_RUN_IDS=()
while IFS=$'\t' read -r id name status conclusion details_url; do
  [[ -n "$id" ]] && CHECK_RUN_IDS+=("$id")
done <<< "$CHECK_RUNS"

if [[ ${#CHECK_RUN_IDS[@]} -eq 0 ]]; then
  echo "No CI Smoke check-runs found for ${SHA}."
else
  for id in "${CHECK_RUN_IDS[@]}"; do
    echo "-- check-run ${id} --"
    annotations=$(gh api --paginate "repos/:owner/:repo/check-runs/${id}/annotations" \
      --jq '.[] | {path,start_line,annotation_level,message}')
    if [[ -z "$annotations" ]]; then
      echo "No annotations returned."
    else
      printf '%s\n' "$annotations"
    fi
  done
fi
