import { type Route } from '@playwright/test';

export const periods = ['1h', '24h', '7d', '30d', '3mo', '6mo', '1y', 'ALL'].map((label) => ({
  label,
  input_tokens: 0,
  cached_tokens: 0,
  cache_creation_tokens: 0,
  output_tokens: 0,
  cost: 0,
  cache_hit_rate: 0,
}));

export const emptyStatsPayload = {
  periods,
  sources: [],
  devices: [{ id: 'local', display_name: 'local' }],
  projects: [],
  is_paused: false,
};

export const sourceCoveragePayload = {
  sources: [
    {
      source: 'Claude Code',
      display_name: 'Claude Code',
      status: 'detected',
      health: 'pending_import',
      data_dir: '/Users/test/.claude/projects',
      records: 0,
      total_cost: 0,
      reason: 'Source logs were found but have not been imported yet.',
    },
    {
      source: 'Codex',
      display_name: 'Codex',
      status: 'watching',
      health: 'complete',
      data_dir: '/Users/test/.codex/sessions',
      records: 2,
      total_cost: 2.5,
      last_seen: '2026-05-15T10:00:00Z',
      reason: 'Usage records are present in the local Token Ray ledger.',
    },
    {
      source: 'Gemini CLI',
      display_name: 'Gemini CLI',
      status: 'no_data',
      health: 'unavailable',
      data_dir: '/Users/test/.gemini/tmp',
      records: 0,
      total_cost: 0,
      reason: 'Default source data directory was not found.',
    },
  ],
};

export async function fulfillJSON(route: Route, body: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  });
}

export async function fulfillEmptyLANScan(route: Route) {
  await fulfillJSON(route, { peers: [] });
}

export async function fulfillLANStatus(route: Route, enabled = true) {
  await fulfillJSON(route, { enabled });
}

export async function fulfillSourceCoverage(route: Route) {
  await fulfillJSON(route, sourceCoveragePayload);
}
