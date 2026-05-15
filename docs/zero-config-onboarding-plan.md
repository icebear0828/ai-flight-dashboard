# Zero-Config Onboarding Implementation Plan

## Overview

Token Ray's next product pass should strengthen the core differentiation: open the app, get a trustworthy local AI usage ledger without configuring hooks, paths, ports, tokens, or commands. The work should favor first-run value, source coverage clarity, and background collection visibility over advanced power-user features.

## Architecture Decisions

- Keep the existing SQLite usage ledger as the source of truth. First-run and coverage views should read from the same `usage_records`, scanner offsets, and dedup/supersede paths used by the dashboard.
- Add a lightweight source coverage layer before changing the scanner flow. The first UI improvement needs a stable backend contract that can explain detected, imported, watching, empty, permission, and unsupported states.
- Treat LAN, Forwarder, CLI reports, and StatusLine as optional enhancements. They should not appear in the first-run critical path.
- Prefer additive API/model changes. The current checkout has broad rename/release work in progress, so this plan avoids invasive rewrites until the zero-config contracts are proven.

## Task List

### Phase 1: Source Coverage Foundation

#### Task 1: Add source coverage status API

**Description:** Add a backend API that reports local coverage for Claude Code, Codex, Gemini CLI, and known unsupported sources. It should combine filesystem detection with existing database totals so the UI can explain what is detected, what has records, and what needs user action.

**Acceptance criteria:**
- [ ] `GET /api/sources/status` returns one entry per known source.
- [ ] Each entry includes `source`, `display_name`, `status`, `health`, `data_dir`, `records`, `total_cost`, `last_seen`, and `reason`.
- [ ] Missing directories return `no_data`; unreadable directories return `needs_permission`; unsupported sources return `unsupported`.

**Verification:**
- [ ] Tests pass: `go test ./internal/onboarding ./internal/web ./internal/db`

**Dependencies:** None

**Files likely touched:**
- `internal/model/stats.go`
- `internal/db/types.go`
- `internal/db/usage_query.go`
- `internal/onboarding/*`
- `internal/web/handler.go`
- `internal/web/source_status_handler_test.go`

**Estimated scope:** Medium

#### Task 2: Display Source Coverage Cards

**Description:** Add compact dashboard cards above the main stats that show Claude, Codex, Gemini, and unsupported coverage states. The cards should explain source health without forcing users into settings.

**Acceptance criteria:**
- [ ] Dashboard fetches `GET /api/sources/status`.
- [ ] Cards show detected/importing/watching/no-data/needs-permission/unsupported states.
- [ ] Empty states provide a concrete reason instead of blank dashboards.

**Verification:**
- [ ] Frontend build succeeds: `cd frontend && npm run build`
- [ ] E2E covers at least detected and no-data cards: `cd frontend && npm run test:e2e`

**Dependencies:** Task 1

**Files likely touched:**
- `frontend/src/App.tsx`
- `frontend/src/dashboardData.ts`
- `frontend/src/locales/en.json`
- `frontend/src/locales/zh.json`
- `frontend/tests/dashboard-ui.spec.ts`

**Estimated scope:** Medium

### Phase 2: First-Run Import and Diagnosis

#### Task 3: Add first-run import state

**Description:** Track whether zero-config history import has completed for the current data dir and source set. This should prevent repeated expensive imports while still allowing explicit repair/retry.

**Acceptance criteria:**
- [ ] First-run import state is stored outside `usage_records`.
- [ ] Import state records per-source completion, last attempt, imported count, and last error.
- [ ] Existing `repair-history` remains explicit and unchanged.

**Verification:**
- [ ] Tests pass for import-state read/write and malformed-state recovery.

**Dependencies:** Task 1

**Estimated scope:** Medium

#### Task 4: Run safe automatic history import

**Description:** On first GUI/Web startup, safely run the existing scanner/Codex import paths in the background and stream status into the source coverage response.

**Acceptance criteria:**
- [ ] First startup imports Claude/Gemini JSONL and Codex session totals without user commands.
- [ ] Import progress and final counts appear in source coverage status.
- [ ] Import is idempotent and uses existing offsets/dedup/supersede behavior.

**Verification:**
- [ ] Go tests cover first-run import idempotency.
- [ ] Manual smoke with temp Claude/Codex/Gemini fixtures shows records appearing in dashboard.

**Dependencies:** Tasks 1 and 3

**Estimated scope:** Medium

### Phase 3: Background Productization

#### Task 5: Make background collection state visible

**Description:** Surface whether Token Ray is currently watching, polling, paused, or blocked. The status should be visible from the dashboard and desktop shell.

**Acceptance criteria:**
- [ ] API exposes collection mode and pause state.
- [ ] UI shows collecting/paused/error state without opening settings.
- [ ] Closing the window keeps collection alive in desktop mode.

**Verification:**
- [ ] Existing GUI lifecycle tests pass.
- [ ] Browser smoke shows status updates correctly.

**Dependencies:** Tasks 1 and 2

**Estimated scope:** Medium

#### Task 6: Add recoverable fix actions

**Description:** Convert common onboarding failures into safe retry or repair actions: permission blocked, missing extra path, import failed, stale source status.

**Acceptance criteria:**
- [ ] Each actionable failure has a clear UI action.
- [ ] Retry does not duplicate records.
- [ ] Failures log a concrete reason for support/debugging.

**Verification:**
- [ ] API tests cover retry failure and success paths.
- [ ] Frontend E2E covers at least one recoverable failure.

**Dependencies:** Tasks 1-5

**Estimated scope:** Medium

## Checkpoints

### Checkpoint A: After Tasks 1-2
- [ ] Source coverage contract is stable.
- [ ] Dashboard can explain installed, missing, and unsupported sources.
- [ ] Existing stats dashboard still works.

### Checkpoint B: After Tasks 3-4
- [ ] First-run import runs automatically and idempotently.
- [ ] Users see useful historical totals within the first minute.

### Checkpoint C: After Tasks 5-6
- [ ] Background collection feels like a desktop product, not a CLI process.
- [ ] Common first-run failures are recoverable without terminal commands.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Recursive source detection becomes expensive | Dashboard polling feels slow | Cache source status briefly and stop filesystem walks after first evidence file |
| Automatic import surprises users | Trust problem | Keep all data local, show what is being imported, and provide retry/diagnostic output |
| Scanner and first-run logic diverge | Duplicate or inconsistent records | Reuse scanner/Codex import paths and existing SQLite dedup/supersede |
| LAN concepts crowd the first screen | Weak onboarding | Keep LAN as delayed prompt or settings-only until local value is visible |

## Not Doing

- StatusLine as a P0 feature: useful, but it requires user config mutation and is not the open-the-app experience.
- New Python/pip distribution: conflicts with the single binary/desktop product direction.
- Replacing the existing SQLite model: the ledger already provides the durable advantage.
- Making LAN the first-run hero: multi-device is a power feature, not the first minute value.
