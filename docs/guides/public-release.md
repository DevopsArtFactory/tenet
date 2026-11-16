# Preparing a public release

Tenet's public introduction should help a new user answer three questions.
What can it deploy? Can I try it locally? What has been validated against AWS?

The recommended first public release is an early-access release focused on
native ECS GitOps. Use a tagged revision containing the current executor,
recovery and emulator tests as the documented baseline.

## Current starting point

Repository settings checked on 2026-11-16 still show private visibility and
disabled Discussions. About now contains the ECS GitOps description, README
homepage and AWS/ECS/GitOps topics. The latest published release
was `v0.1.154`, dated 2026-07-17, at the initial review. The next candidate is
`v0.2.0-rc.1`; its [release notes](../releases/v0.2.0-rc.1.md) describe the ECS
execution baseline and security migration. Select its commit only after all
security PRs and that commit's CI have passed.

The repository already includes Apache-2.0, contributing guidelines, a code of
conduct, a security policy, issue/PR templates and CI. These provide a starting
point for contributors.

## Present the supported product

A proposed short description is **GitOps deployment control plane for native
Amazon ECS across AWS accounts**. The README can also explain EC2 catalog,
planning and inspection support.

Document execution scope with the public release.

| Area | Current scope |
| --- | --- |
| ECS execution | Existing native Rolling, Blue-Green, Canary and Linear services, behind explicit execution gates |
| GitOps | GitHub ref polling, SHA-bound plans, scoped automatic approval, count changes and optional self-heal |
| Recovery | Separate recovery Apply, captured source revision/count restoration, durable checkpoints |
| EC2 | Catalog, planning, status and inspection |
| Authentication | GitHub OAuth, Google OIDC and Okta OIDC; Okta SAML configuration diagnostics |
| Persistence | Local file-backed plans/Applies and OS locks; runtime status can use Redis |
| Integration evidence | AWS Emulator and fixture-based CI; live AWS sandbox evidence remains pending |

Describe the advanced strategy constraints, storage topology and test fixtures
in the release notes. Link the supported input contract and known limitations.

## Make the first experience reproducible

- Have someone unfamiliar with Tenet follow the [getting-started guide](getting-started.md)
  from a fresh checkout. Record the source revision, OS, Go version and result.
- Verify the documented toolchain can be installed and the source builds from the
  release tag. Choose a distribution path users can reproduce, starting with
  source and the checked-in Docker build.
- Capture an Applications screenshot and one Apply/recovery example using synthetic
  names and accounts. Show the product outcome in the README or release notes.
- Walk through the [ECS setup](ecs-gitops.md) in a disposable account, including
  identity mapping, role trust, manual approval, Git sync and a failed rollout.
  Preserve the evidence required by the sandbox runbook.
- Link one documentation entry point from GitHub's About section. Keep the README
  short enough to reach the demo command quickly.

A documentation page is complete when its commands, expected results and
troubleshooting steps let a new reader proceed without the authors' context.

## Review what becomes public

Before changing visibility, review the Git history, tracked files, release
assets, issue/PR content and screenshots for credentials or internal data. A
current working-tree check covers only part of that surface. Rotate any exposed
credentials as part of resolving a finding.

Review dependency licenses and attribution alongside the existing project
license. Confirm that the organization can distribute the code and included
assets under the declared terms. GitHub detects the repository's existing
Apache-2.0 license file. [GitHub licensing guidance](https://docs.github.com/en/communities/setting-up-your-project-for-healthy-contributions/adding-a-license-to-a-repository)

## Establish maintainership

Confirm who will handle issues, releases and vulnerability reports. The maintainer
has confirmed `yj.jung5078@gmail.com` as the contact in `SECURITY.md` and
`CODE_OF_CONDUCT.md`; the former organization mailboxes were not functional.
No fixed response SLA is promised. GitHub private vulnerability reporting is the
preferred security channel once enabled, with email as a fallback.

Define which release lines receive fixes and how users report unsupported
configurations. Add reproducible startup and test commands to contributor
guidance as the public release baseline is selected.

Decide whether to enable Discussions for setup questions. Keep bug reports and
feature requests in the existing issue templates. Enable private vulnerability
reporting when the repository is public and ensure maintainers receive its
notifications. [GitHub vulnerability reporting](https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/configure-vulnerability-reporting/configure-for-a-repository)

## Publish in stages

1. Merge the onboarding docs and agree on the supported early-access scope.
2. Resolve public-data findings and confirm the ownership/support decisions above.
3. Tag the chosen source revision, record its CI results and write release notes
   that distinguish emulator evidence from live sandbox results.
4. Change repository visibility through the organization's repository settings,
   then check anonymous access to code, documentation, issues and release assets.
5. Add the About description, documentation URL and relevant ECS/GitOps/AWS topics.
6. Announce the release with the local quickstart, screenshots, known limitations
   and a concrete request for feedback from sandbox users.

Public availability and production validation can advance separately. State the
release's validation level explicitly so adopters can choose an appropriate
evaluation environment.

## Release review record

Use this table in the release issue or notes to record evidence.

| Review | Evidence to attach |
| --- | --- |
| Source and installation | Tag/commit, supported toolchain and fresh-checkout result |
| CI | Passing run for that commit, including native ECS emulator scenarios |
| Live AWS | Per-strategy sandbox results, or an explicit pending status |
| Documentation | Tested quickstart and setup paths, screenshots and limitations |
| Public data and licensing | Review owner, reviewed surfaces and resolved findings |
| Security and support | Confirmed reporting channel, maintainers and supported versions |
| Distribution | Source/build instructions and any published artifact checksums |

## Package and publish the candidate

Keep repository visibility unchanged while preparing the candidate. Use a source
commit already on `main` with passing Fast smoke, Docker smoke, ECS fixture,
AWS Emulator and Go vulnerability checks. Record the run URL in the release body.

```bash
gh auth switch --hostname github.com --user YoungJinJung
git fetch origin main
release_commit=$(git rev-parse origin/main)
release_dir=$(mktemp -d)
./scripts/release-source.sh v0.2.0-rc.1 "$release_dir" "$release_commit"
(cd "$release_dir" && shasum -a 256 -c SHA256SUMS)
```

The archive contains only the selected commit's tracked files. `SOURCE_COMMIT`
and checksums identify the distribution; local logs, credentials and uncommitted
files are excluded. Extract and build/test the archive before publishing.
Create an annotated `v0.2.0-rc.1` tag at that exact commit, push the tag, and use
`gh release create --verify-tag --prerelease --latest=false` with the source
archive, `SOURCE_COMMIT`, `SHA256SUMS` and the reviewed release notes. Do not move
an existing release tag. A candidate must not become the recommended stable
release until the outstanding live validation and public-readiness gates pass.

## Verify repository settings before public announcement

Run `python3 scripts/check-public-readiness.py` while authenticated as a repository
administrator. It performs read-only GitHub API checks and fails closed when a
setting is missing or cannot be inspected. A private repository will intentionally
fail this public-announcement gate; do not change visibility just to make it pass.

Require pull requests, up-to-date branches and the Fast smoke, Docker smoke scope,
Docker smoke, ECS fixture validation (no AWS), Native ECS rollback (AWS Emulator),
and Go vulnerability scan checks on `main`. Enforce the rule for administrators,
disable force pushes/deletions, and use equivalent active rulesets if the
organization manages policies centrally. The checker inspects classic branch
protection; a ruleset-only configuration needs an explicit manual review.

Some private repository plans do not support these settings. In that case, enable
and verify them at the authorized public transition, then verify anonymous access
to the reporting form and release assets. This checklist does not itself change
visibility, send email, or publish a release.
