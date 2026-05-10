# Release Process

Releases are tag-driven, but release tags must be created by GitHub Actions instead of a local shell. This keeps the release audit trail in one place and prevents tags from bypassing the required `main` test gate.

## Required Sequence

1. Merge the PR to `main`.
2. Wait for the `Test` workflow on the exact `main` merge commit to pass.
3. Run the `Tag Release` workflow from GitHub Actions.
4. Leave `version` empty to bump the latest `vX.Y.Z` patch tag, or enter an explicit `vX.Y.Z` version.
5. Wait for the dispatched `Release` workflow on the new tag to finish.
6. Verify that the GitHub Release contains the expected platform assets.

Do not stop after merging a release-worthy PR. A merged fix without a release tag is incomplete shipping work.

## Command Runbook

Use these commands when operating from a local checkout:

```bash
gh run list --workflow Test --branch main --limit 5
git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-version:refname
gh workflow run "Tag Release" --ref main
gh run list --limit 10
gh run watch <release-run-id>
gh release view <tag>
```

For an explicit version, pass `-f version=vX.Y.Z` to `gh workflow run "Tag Release" --ref main`.

## Tag Release Gate

The `Tag Release` workflow validates that:

- the version uses `vX.Y.Z` format,
- the tag does not already exist,
- `main` HEAD is not already release-tagged,
- the `Test` workflow passed for the exact `main` commit being tagged.

The workflow then pushes the tag and explicitly dispatches the `Release` workflow at that tag. Manual user-created `vX.Y.Z` tag pushes also trigger `Release`, but the standard process is still to use `Tag Release`.

## Release Gate

The `Release` workflow validates and packages the tag:

- validates the tag name,
- derives Wails `productVersion` from the tag,
- builds the frontend,
- runs frontend Playwright E2E,
- runs Go tests,
- runs build verification,
- builds and uploads Linux, macOS Apple Silicon, macOS Intel, and Windows assets.

During release builds, Wails `productVersion` is derived from the tag and written into both Wails config files before packaging.

## Failure Handling

If `Tag Release` fails, do not create a local replacement tag. Fix the failed gate, rerun `Tag Release`, and keep the audit trail in Actions.

If `Release` fails after the tag was pushed:

1. Read the failed job log.
2. Fix the cause in a new PR.
3. Merge the PR after `Test` passes.
4. Run `Tag Release` again with the next patch version.

Do not move or force-push a published release tag.

## Completion Checklist

Before saying a release is done:

- [ ] PR is merged to `main`.
- [ ] `Test` on `main` is green for the merge commit.
- [ ] `Tag Release` completed successfully.
- [ ] New `vX.Y.Z` tag exists.
- [ ] `Release` completed successfully for that tag.
- [ ] GitHub Release exists with all expected assets.
