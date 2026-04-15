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
- expandable on-demand deployment detail panels for placement, sync reasons, rollout generations, conditions, and the full rollout-step list

## Backing API

The page consumes joined overview endpoints:
- `GET /v1/overview/accounts`
- `GET /v1/overview/deployments/{deploymentName}`

`/v1/overview/accounts` keeps the top-level account page fast by returning the summary slice needed to render the default cards.

`/v1/overview/deployments/{deploymentName}` combines catalog placement data with the current deployment status snapshot for one deployment, including:
- `sync.reason`
- account and region placement metadata
- rollout desired/observed generations
- `conditions[*]`
- the complete `rollout.steps[*]` list

The embedded UI only fetches the deployment-detail endpoint when an operator expands a row.

Each deployment card still surfaces the most operator-useful rollout cues directly in the overview response/UI:
- `rollout.phase`
- `rollout.currentStep`
- `rollout.steps[*].kind`

The expandable detail panel extends that view without forcing operators to switch to the raw status endpoint for common troubleshooting.

## Detail-panel rendering constraints

The UI remains static embedded HTML/CSS/JS and preserves safe DOM construction:
- detail content is rendered with `document.createElement`, `textContent`, and `replaceChildren`
- manifest/status-derived values are not injected with `innerHTML`
- loading and error states are scoped to the expanded deployment row

## Why account-first grouping

The product direction is a multi-account control plane. Grouping by account keeps the local demo aligned with the eventual operating model and makes it easy to compare deployments across environments within the same account boundary.

## Current scope

The UI is intentionally small:
- static HTML/CSS/JS embedded into the Go binary
- no Node build
- no mutations or write APIs

Future slices can replace or extend this with a richer frontend once the runtime API stabilizes.
