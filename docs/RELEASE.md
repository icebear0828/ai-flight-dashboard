# Release Process

Releases are tag-driven, but release tags must be created by GitHub Actions instead of a local shell. This keeps the release audit trail in one place and prevents tags from bypassing the required `main` test gate.

## Standard Sequence

1. Merge the PR to `main`.
2. Wait for the `Test` workflow on the exact `main` merge commit to pass.
3. `Auto Release` decides whether the commit is release-worthy.
4. If release-worthy, `Auto Release` creates the next `vX.Y.Z` tag and dispatches `Release`.
5. Wait for the dispatched `Release` workflow on the new tag to finish.
6. Verify that the GitHub Release contains the bilingual notes and expected platform assets.

Documentation-only and non-runtime changes should finish with `Auto Release` skipped. Do not stop after merging a release-worthy PR. A merged fix without a release tag is incomplete shipping work.

## Pull Request Notes

Release-worthy PRs must fill both sections from `.github/pull_request_template.md`:

```markdown
## Release notes zh-CN
- 修复价格更新后历史用量成本不会重新计算的问题。

## Release notes en-US
- Fixed historical usage costs not being recalculated after pricing updates.
```

Leaving the default `N/A` placeholder is a release gate failure. Use the `release:skip` or `release:none` label only when a product change intentionally should not publish a release.

## Auto Version Rules

- `release:skip` or `release:none`: skip release.
- `release:major`: bump major.
- `release:minor` or `feat:`: bump minor.
- other release-worthy product changes: bump patch.

Docs-only, `.github/`, test-only, and Markdown-only changes are skipped by default.

## Command Runbook

Use these commands when operating from a local checkout:

```bash
gh run list --workflow Test --branch main --limit 5
gh run list --workflow "Auto Release" --branch main --limit 5
git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-version:refname
gh run list --limit 10
gh run watch <release-run-id>
gh release view <tag>
```

Manual fallback:

```bash
gh workflow run "Tag Release" --ref main
```

For an explicit fallback version, pass `-f version=vX.Y.Z` to `gh workflow run "Tag Release" --ref main`.

## Auto Release Gate

The `Auto Release` workflow validates that:

- the completed `Test` run was successful for a `main` push,
- the tested commit is still current `main`,
- `main` HEAD is not already release-tagged,
- changed files are release-worthy runtime files,
- associated PRs have both Chinese and English release notes,
- the next tag does not already exist.

The workflow then pushes the tag and explicitly dispatches the `Release` workflow at that tag with encoded bilingual notes.

## Tag Release Gate

The `Tag Release` workflow validates that:

- the version uses `vX.Y.Z` format,
- the tag does not already exist,
- `main` HEAD is not already release-tagged,
- the `Test` workflow passed for the exact `main` commit being tagged,
- release-worthy PRs include both Chinese and English release notes.

The workflow then pushes the tag and explicitly dispatches the `Release` workflow at that tag with encoded bilingual notes. Manual user-created `vX.Y.Z` tag pushes also trigger `Release`, but the standard process is still `Auto Release`; `Tag Release` is the fallback.

## Release Gate

The `Release` workflow validates and packages the tag:

- validates the tag name,
- derives Wails `productVersion` from the tag,
- builds the frontend,
- runs frontend Playwright E2E,
- runs Go tests,
- runs build verification,
- builds and uploads Linux, macOS Apple Silicon, macOS Intel, and Windows assets,
- updates the GitHub Release title and bilingual body when notes were provided by `Auto Release`.

During release builds, Wails `productVersion` is derived from the tag and written into both Wails config files before packaging.

## Failure Handling

If `Auto Release` fails because release notes are missing, update the PR body and rerun the workflow. If the product change should not publish a release, add `release:skip` or `release:none` and rerun.

If `Tag Release` fails, do not create a local replacement tag. Fix the failed gate, rerun `Tag Release`, and keep the audit trail in Actions.

If `Release` fails after the tag was pushed:

1. Read the failed job log.
2. Fix the cause in a new PR.
3. Merge the PR after `Test` passes.
4. Let `Auto Release` create the next version, or run `Tag Release` as the approved fallback.

Do not move or force-push a published release tag.

## Completion Checklist

Before saying a release is done:

- [ ] PR is merged to `main`.
- [ ] `Test` on `main` is green for the merge commit.
- [ ] `Auto Release` created the tag, or `Tag Release` completed successfully as the approved fallback.
- [ ] New `vX.Y.Z` tag exists.
- [ ] `Release` completed successfully for that tag.
- [ ] GitHub Release body contains Chinese and English notes.
- [ ] GitHub Release exists with all expected assets.
