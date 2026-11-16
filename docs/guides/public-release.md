# Preparing a public release

Tenet's public introduction should help a new user answer three questions.
What can it deploy? Can I try it locally? What has been validated against AWS?

The recommended first public release is an early-access release focused on
native ECS GitOps. Use a tagged revision containing the current executor,
recovery and emulator tests as the documented baseline.

## Current starting point

Repository settings checked on 2026-11-16 show private visibility, an empty About
description and homepage, and disabled Discussions. The latest published release
is `v0.1.154`, dated 2026-07-17. Select a new release baseline containing the
recent ECS work and make its notes describe that baseline's actual capabilities.

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

Confirm who will handle issues, releases and vulnerability reports. Test delivery
to the addresses currently listed in `SECURITY.md` and `CODE_OF_CONDUCT.md`.
Make the 72-hour acknowledgment commitment in `SECURITY.md` match the team's
actual support capacity.

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
