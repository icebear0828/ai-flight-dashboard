# Testing and CI/CD

This project uses GitHub Actions as the source of truth for quality gates and releases. Local checks are required before pushing, but a change is not considered mergeable or shipped until the matching CI workflows pass.

## Workflows

### `Test`

File: `.github/workflows/test.yml`

Triggers:

- every branch push,
- every pull request targeting `main`.

Jobs:

- `Unit Tests`
  - installs Linux GUI dependencies,
  - builds the frontend,
  - installs the Playwright Chromium browser,
  - runs frontend E2E with `cd frontend && npm run test:e2e`,
  - runs Go tests with `go test -race -count=1 -timeout=5m ./...`,
  - runs `go build ./...`.
- `Lint`
  - installs Linux GUI dependencies,
  - builds the frontend,
  - runs `go vet ./...`.

### `Tag Release`

File: `.github/workflows/tag-release.yml`

Trigger:

- manual `workflow_dispatch`.

Purpose:

- resolves the requested `vX.Y.Z` version or bumps the latest patch tag,
- rejects duplicate or malformed tags,
- rejects attempts to tag a `main` commit that already has a release tag,
- verifies that `Test` passed on the exact `main` commit,
- creates and pushes the release tag,
- dispatches `Release` for that tag.

### `Release`

File: `.github/workflows/release.yml`

Triggers:

- manual `workflow_dispatch`,
- push of tags matching `v*`.

Jobs:

- `Validate`
  - validates the tag format,
  - aligns Wails product version from the tag,
  - builds the frontend,
  - runs Playwright E2E,
  - runs Go tests,
  - runs Go build verification.
- `Build macOS`
  - builds Apple Silicon and Intel `.app` archives.
- `Build Windows`
  - builds and uploads the Windows `.zip`.
- `Build Linux`
  - builds and uploads the Linux `.tar.gz`.

The platform build jobs run only after `Validate` passes.

## Required Local Checks

Before opening or updating a PR that changes behavior:

```bash
cd frontend && npm run build
cd frontend && npm run test:e2e
go test -race -count=1 -timeout=5m ./...
go vet ./...
go build ./...
```

For docs-only changes, inspect the changed Markdown and confirm it matches the workflow files. Do not spend CI time on unrelated local test loops unless the docs change also touches executable behavior.

## Merge Gate

A PR can be merged only when:

- `Test / Unit Tests` passes,
- `Test / Lint` passes,
- the review has no unresolved blocking findings,
- required tests for the touched behavior were added or updated,
- no unrelated dirty files are included.

After merge, wait for the `Test` workflow on `main` to pass before running `Tag Release`.

## Release Gate

A release is complete only when:

- `Tag Release` is green,
- the new tag exists,
- `Release` is green for that tag,
- GitHub Release assets are uploaded for Linux, macOS Apple Silicon, macOS Intel, and Windows.

Merging to `main` without creating the release tag is incomplete shipping work.

## Technical-Debt Rules

- Do not create release tags locally unless GitHub Actions is unavailable and the maintainer explicitly approves the exception.
- Do not move, delete, or force-push published release tags.
- Do not skip or disable failing tests to make CI green.
- Do not merge with failing or pending required checks.
- Do not let flaky tests accumulate. Reproduce the failure, fix the flake, and keep the test in CI.
- Do not add a workflow that bypasses the existing `Test` and `Release` gates.
- Keep workflow behavior and documentation in sync in the same PR.
- Update this file when adding, removing, or renaming CI jobs.

## Failure Runbook

When `Test` fails:

1. Open the failed job log.
2. Reproduce the failing command locally.
3. Fix the root cause.
4. Rerun the local command.
5. Push the fix and wait for CI.

When `Tag Release` fails:

1. Read the failing gate.
2. Fix the underlying issue.
3. Rerun `Tag Release`.
4. Do not create a local tag to bypass the workflow.

When `Release` fails:

1. Read the failed platform or validation job log.
2. Fix the cause in a new PR.
3. Merge after `Test` passes on the PR.
4. Wait for `Test` on `main`.
5. Run `Tag Release` again with the next patch version.

## Branch Protection Expectations

Repository settings should require these checks before merging to `main`:

- `Test / Unit Tests`
- `Test / Lint`

Direct pushes to `main` should be avoided for behavior changes. Force-pushes to `main` and release tags should stay disabled.
