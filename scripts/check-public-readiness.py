#!/usr/bin/env python3
"""Read-only checks for a Tenet public announcement; never changes visibility."""
import argparse
import json
import subprocess
import sys
from urllib.parse import quote

REQUIRED_CHECKS = {
    "Fast smoke", "Docker smoke scope", "Docker smoke",
    "ECS fixture validation (no AWS)", "Native ECS rollback (AWS Emulator)",
    "Go vulnerability scan",
}


def api(path):
    result = subprocess.run(["gh", "api", path], capture_output=True, text=True)
    if result.returncode:
        return None
    return json.loads(result.stdout)


def evaluate(repo, reporting, protection):
    status = (protection or {}).get("required_status_checks") or {}
    contexts = set(status.get("contexts", []))
    contexts.update(check.get("context") for check in status.get("checks", []))
    return {
        "public visibility": repo.get("visibility") == "public",
        "About description and homepage": bool(repo.get("description") and repo.get("homepage")),
        "private vulnerability reporting": bool(reporting and reporting.get("enabled")),
        "required pull requests": bool(protection and protection.get("required_pull_request_reviews") is not None),
        "required CI checks and up-to-date branch": bool(status.get("strict") and REQUIRED_CHECKS <= contexts),
        "administrator enforcement": bool(protection and protection.get("enforce_admins", {}).get("enabled")),
        "force pushes and deletions disabled": bool(protection and not protection.get("allow_force_pushes", {}).get("enabled") and not protection.get("allow_deletions", {}).get("enabled")),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", help="owner/repository (defaults to current GitHub repository)")
    args = parser.parse_args()
    repo_name = args.repo or subprocess.check_output(
        ["gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner"], text=True
    ).strip()
    repo = api("repos/" + repo_name)
    if repo is None:
        print("FAIL: repository settings unavailable; authenticate as a repository administrator.")
        return 1
    branch = quote(repo["default_branch"], safe="")
    checks = evaluate(repo, api(f"repos/{repo_name}/private-vulnerability-reporting"),
                      api(f"repos/{repo_name}/branches/{branch}/protection"))
    for label, passed in checks.items():
        print(f"{'PASS' if passed else 'FAIL'}: {label}")
    if not all(checks.values()):
        print("Public announcement is not ready. See docs/guides/public-release.md; unavailable settings require manual follow-up.")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
