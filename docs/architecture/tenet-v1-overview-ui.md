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
- rollout phase
- current rollout step
- planned rollout steps
- sync and health summaries

## Backing API

The page consumes one joined endpoint:
- `GET /v1/overview/accounts`

This endpoint combines catalog placement data with deployment status snapshots, including the controller-owned `rollout` summary when one is present.

Each deployment card now surfaces the most operator-useful rollout cues directly in the overview response/UI:
- `rollout.phase`
- `rollout.currentStep`
- `rollout.steps[*].kind`

That keeps the account-first page useful for active rollouts without forcing operators to switch to the raw status endpoint for basic progress checks.

## Why account-first grouping

The product direction is a multi-account control plane. Grouping by account keeps the local demo aligned with the eventual operating model and makes it easy to compare deployments across environments within the same account boundary.

## Current scope

The UI is intentionally small:
- static HTML/CSS/JS embedded into the Go binary
- no Node build
- no mutations or write APIs

Future slices can replace or extend this with a richer frontend once the runtime API stabilizes.
