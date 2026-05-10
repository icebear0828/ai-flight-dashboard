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
