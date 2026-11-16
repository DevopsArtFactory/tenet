# Security policy

## Supported versions

Security fixes target `main` and the latest early-access release candidate.
Older releases do not receive guaranteed backports; upgrade to the current
candidate before evaluating Tenet in a shared environment. Early-access status
does not imply production or live AWS qualification.

## Reporting a vulnerability

Please do **not** disclose vulnerabilities, credentials or exploit details in
public GitHub issues, discussions or pull requests.

When private vulnerability reporting is enabled, use
[Report a vulnerability](https://github.com/DevopsArtFactory/tenet/security/advisories/new)
to contact maintainers privately. If the repository is still private or the
reporting form is unavailable, email
[yj.jung5078@gmail.com](mailto:yj.jung5078@gmail.com).

Include the affected commit/release, a description, reproduction steps, potential
impact and any mitigation. Use synthetic accounts and redact credentials or
personal data. Maintainers will coordinate investigation and disclosure through
the private reporting channel; this volunteer project does not promise a fixed
response SLA.

## Maintainer release gate

Enable GitHub private vulnerability reporting when the repository becomes
public, and verify that the reporting form and maintainer notifications work.
Until then, keep the email fallback available. Run
`python3 scripts/check-public-readiness.py` before announcing a public release;
it checks visibility, reporting, About metadata and branch protection without
changing repository settings.
