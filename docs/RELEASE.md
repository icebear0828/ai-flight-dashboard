# Release Process

Releases are tag-driven, but release tags should be created by GitHub Actions instead of a local shell.

1. Merge the PR to `main`.
2. Wait for the `Test` workflow on `main` to pass.
3. Run the `Tag Release` workflow from GitHub Actions.
4. Leave `version` empty to bump the latest `vX.Y.Z` patch tag, or enter an explicit `vX.Y.Z` version.

The `Tag Release` workflow validates that:

- the version uses `vX.Y.Z` format,
- the tag does not already exist,
- `main` HEAD is not already release-tagged,
- the `Test` workflow passed for the exact `main` commit being tagged.

The `Tag Release` workflow pushes the tag, then explicitly dispatches the `Release` workflow at that tag. Manual user-created `vX.Y.Z` tag pushes also trigger `Release`.

During release builds, Wails `productVersion` is derived from the tag and written into both Wails config files before packaging.
