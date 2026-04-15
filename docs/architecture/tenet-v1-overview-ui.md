# Tenet Overview UI

Tenet now serves a minimal embedded UI at `/`.

## Purpose

The UI is intended to make the current control-plane slice visible without introducing a separate frontend toolchain.

It groups deployments by AWS account and shows:
- deployment name and environment
- target type
- rollout strategy
- derived execution mode
- desired artifact
- deployed artifact
- sync and health summaries

## Backing API

The page consumes one joined endpoint:
- `GET /v1/overview/accounts`

This endpoint combines catalog placement data with deployment status snapshots.

## Why account-first grouping

The product direction is a multi-account control plane. Grouping by account keeps the local demo aligned with the eventual operating model and makes it easy to compare deployments across environments within the same account boundary.

## Current scope

The UI is intentionally small:
- static HTML/CSS/JS embedded into the Go binary
- no Node build
- no mutations or write APIs

Future slices can replace or extend this with a richer frontend once the runtime API stabilizes.
