import { expect, test, type Route } from '@playwright/test';

import { fulfillEmptyLANScan, fulfillJSON, fulfillLANStatus, periods } from './dashboard-helpers';

test('source switching shows loading and ignores stale stats responses', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  let heldClaudeRoute: Route | null = null;

  const statsPayload = (source: string) => ({
    periods,
    sources: source
      ? [{
          name: source,
          total_input: 1000,
          total_cached: 100,
          total_cache_creation: 50,
          total_output: 25,
          total_cost: source === 'Codex' ? 2.5 : 1.5,
          total_events: 1,
          cache_hit_rate: 10,
          models: [{
            model: source === 'Codex' ? 'gpt-5.5' : 'claude-opus-4-7',
            events: 1,
            input_tokens: 1000,
            cached_tokens: 100,
            cache_creation_tokens: 50,
            output_tokens: 25,
            total_cost: source === 'Codex' ? 2.5 : 1.5,
            input_price_per_m: 1,
            cached_price_per_m: 0.1,
            cache_creation_price_per_m: 1,
            output_price_per_m: 2,
            cache_hit_rate: 10,
          }],
        }]
      : [],
    devices: [{ id: 'local', display_name: 'local' }],
    projects: source ? [{
      project: source === 'Codex' ? 'token' : 'api',
      events: 1,
      input_tokens: 1000,
      cached_tokens: 100,
      cache_creation_tokens: 50,
      output_tokens: 25,
      total_cost: source === 'Codex' ? 2.5 : 1.5,
      cache_hit_rate: 10,
    }] : [],
    is_paused: false,
  });

  await page.route('**/api/stats?*', async (route) => {
    const url = new URL(route.request().url());
    const source = url.searchParams.get('source') ?? '';
    if (source === 'Claude Code') {
      heldClaudeRoute = route;
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(statsPayload(source)),
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');
  await expect(page.getByRole('button', { name: 'CLAUDE' })).toBeVisible();

  await page.getByRole('button', { name: 'CLAUDE' }).click();
  await expect(page.getByRole('status').filter({ hasText: '加载中' })).toBeVisible();

  await page.getByRole('button', { name: 'CODEX' }).click();
  await expect(page.getByRole('heading', { name: 'Codex' })).toBeVisible();
  await expect(page.getByRole('status').filter({ hasText: '加载中' })).toHaveCount(0);

  expect(heldClaudeRoute).not.toBeNull();
  await heldClaudeRoute?.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(statsPayload('Claude Code')),
  });

  await expect(page.getByRole('heading', { name: 'Codex' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Claude Code' })).toHaveCount(0);
  expect(pageErrors).toEqual([]);
});

test('source switching includes Antigravity source tab', async ({ page }) => {
  const requestedSources: string[] = [];

  const statsPayload = (source: string) => ({
    periods,
    sources: source
      ? [{
          name: source,
          total_input: 2150,
          total_cached: 100,
          total_cache_creation: 50,
          total_output: 75,
          total_cost: 0.003765,
          total_events: 1,
          cache_hit_rate: 4.651162790697675,
          models: [{
            model: 'gemini-3.5-flash',
            events: 1,
            input_tokens: 2150,
            cached_tokens: 100,
            cache_creation_tokens: 50,
            output_tokens: 75,
            total_cost: 0.003765,
            input_price_per_m: 1.5,
            cached_price_per_m: 0.15,
            cache_creation_price_per_m: 1.5,
            output_price_per_m: 9,
            cache_hit_rate: 4.651162790697675,
          }],
        }]
      : [],
    devices: [{ id: 'local', display_name: 'local' }],
    projects: source ? [{
      project: 'token',
      events: 1,
      input_tokens: 2150,
      cached_tokens: 100,
      cache_creation_tokens: 50,
      output_tokens: 75,
      total_cost: 0.003765,
      cache_hit_rate: 4.651162790697675,
    }] : [],
    is_paused: false,
  });

  await page.route('**/api/stats?*', async (route) => {
    const url = new URL(route.request().url());
    const source = url.searchParams.get('source') ?? '';
    requestedSources.push(source);
    await fulfillJSON(route, statsPayload(source));
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');
  await expect(page.getByRole('button', { name: 'ANTIGRAVITY' })).toBeVisible();

  await page.getByRole('button', { name: 'ANTIGRAVITY' }).click();
  await expect(page.getByRole('heading', { name: 'Antigravity' })).toBeVisible();
  await expect(page.getByText('gemini-3.5-flash', { exact: true })).toBeVisible();
  await expect(page.getByText('$0.0038').first()).toBeVisible();
  expect(requestedSources).toContain('Antigravity');
});

test('source switching renders summary before delayed details', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  let heldCodexDetailsRoute: Route | null = null;
  const requestedDetails: string[] = [];

  const summaryPayload = (source: string) => ({
    periods,
    sources: source
      ? [{
          name: source,
          total_input: 1000,
          total_cached: 100,
          total_cache_creation: 50,
          total_output: 25,
          total_cost: 2.5,
          total_events: 1,
          cache_hit_rate: 10,
          models: [],
        }]
      : [],
    devices: [{ id: 'local', display_name: 'local' }],
    projects: [],
    is_paused: false,
  });

  const detailsPayload = (source: string) => ({
    periods: [],
    sources: source
      ? [{
          name: source,
          total_input: 1000,
          total_cached: 100,
          total_cache_creation: 50,
          total_output: 25,
          total_cost: 2.5,
          total_events: 1,
          cache_hit_rate: 10,
          models: [{
            model: 'gpt-5.5',
            events: 1,
            input_tokens: 1000,
            cached_tokens: 100,
            cache_creation_tokens: 50,
            output_tokens: 25,
            total_cost: 2.5,
            input_price_per_m: 1,
            cached_price_per_m: 0.1,
            cache_creation_price_per_m: 1,
            output_price_per_m: 2,
            cache_hit_rate: 10,
          }],
        }]
      : [],
    devices: [],
    projects: source ? [{
      project: 'token',
      events: 1,
      input_tokens: 1000,
      cached_tokens: 100,
      cache_creation_tokens: 50,
      output_tokens: 25,
      total_cost: 2.5,
      cache_hit_rate: 10,
    }] : [],
    is_paused: false,
  });

  await page.route('**/api/stats?*', async (route) => {
    const url = new URL(route.request().url());
    const source = url.searchParams.get('source') ?? '';
    const detail = url.searchParams.get('detail') ?? '';
    requestedDetails.push(detail);

    if (detail === 'details' && source === 'Codex') {
      heldCodexDetailsRoute = route;
      return;
    }

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(detail === 'summary' ? summaryPayload(source) : detailsPayload(source)),
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');
  await expect(page.getByRole('button', { name: 'CODEX' })).toBeVisible();

  await page.getByRole('button', { name: 'CODEX' }).click();
  await expect(page.getByRole('heading', { name: 'Codex' })).toBeVisible();
  await expect(page.getByText('gpt-5.5')).toHaveCount(0);
  await expect(page.getByText('token', { exact: true })).toHaveCount(0);
  expect(heldCodexDetailsRoute).not.toBeNull();
  expect(requestedDetails).toContain('summary');
  expect(requestedDetails).toContain('details');

  await heldCodexDetailsRoute?.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(detailsPayload('Codex')),
  });

  await expect(page.getByText('gpt-5.5')).toBeVisible();
  await expect(page.getByText('token', { exact: true })).toBeVisible();
  expect(pageErrors).toEqual([]);
});

test('background refresh keeps project stats visible while details reload', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  let detailsRequests = 0;
  let heldRefreshDetailsRoute: Route | null = null;

  const summaryPayload = {
    periods,
    sources: [{
      name: 'Codex',
      total_input: 1000,
      total_cached: 100,
      total_cache_creation: 50,
      total_output: 25,
      total_cost: 2.5,
      total_events: 1,
      cache_hit_rate: 10,
      models: [],
    }],
    devices: [{ id: 'local', display_name: 'local' }],
    projects: [],
    is_paused: false,
  };

  const detailsPayload = {
    periods: [],
    sources: [{
      name: 'Codex',
      total_input: 1000,
      total_cached: 100,
      total_cache_creation: 50,
      total_output: 25,
      total_cost: 2.5,
      total_events: 1,
      cache_hit_rate: 10,
      models: [{
        model: 'gpt-5.5',
        events: 1,
        input_tokens: 1000,
        cached_tokens: 100,
        cache_creation_tokens: 50,
        output_tokens: 25,
        total_cost: 2.5,
        input_price_per_m: 1,
        cached_price_per_m: 0.1,
        cache_creation_price_per_m: 1,
        output_price_per_m: 2,
        cache_hit_rate: 10,
      }],
    }],
    devices: [],
    projects: [{
      project: 'token',
      events: 1,
      input_tokens: 1000,
      cached_tokens: 100,
      cache_creation_tokens: 50,
      output_tokens: 25,
      total_cost: 2.5,
      cache_hit_rate: 10,
    }],
    is_paused: false,
  };

  await page.route('**/api/stats?*', async (route) => {
    const url = new URL(route.request().url());
    const detail = url.searchParams.get('detail') ?? '';

    if (detail === 'details') {
      detailsRequests += 1;
      if (detailsRequests === 2) {
        heldRefreshDetailsRoute = route;
        return;
      }
    }

    await fulfillJSON(route, detail === 'summary' ? summaryPayload : detailsPayload);
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');

  await expect(page.getByText('token', { exact: true })).toBeVisible();
  await expect(page.getByText('gpt-5.5')).toBeVisible();

  await expect.poll(() => heldRefreshDetailsRoute !== null, { timeout: 7000 }).toBe(true);
  await expect(page.getByText('token', { exact: true })).toBeVisible();
  await expect(page.getByText('gpt-5.5')).toBeVisible();

  await heldRefreshDetailsRoute?.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(detailsPayload),
  });

  expect(pageErrors).toEqual([]);
});

test('source switching keeps summary visible when details fail', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  let detailFailures = 0;
  const summaryPayload = (source: string) => ({
    periods,
    sources: source
      ? [{
          name: source,
          total_input: 1000,
          total_cached: 100,
          total_cache_creation: 50,
          total_output: 25,
          total_cost: 2.5,
          total_events: 1,
          cache_hit_rate: 10,
          models: [],
        }]
      : [],
    devices: [{ id: 'local', display_name: 'local' }],
    projects: [],
    is_paused: false,
  });

  await page.route('**/api/stats?*', async (route) => {
    const url = new URL(route.request().url());
    const source = url.searchParams.get('source') ?? '';
    const detail = url.searchParams.get('detail') ?? '';
    if (detail === 'details' && source === 'Codex') {
      detailFailures++;
      await route.fulfill({
        status: 500,
        contentType: 'text/plain',
        body: 'details unavailable',
      });
      return;
    }

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(summaryPayload(source)),
    });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);

  await page.goto('/');
  await expect(page.getByRole('button', { name: 'CODEX' })).toBeVisible();

  await page.getByRole('button', { name: 'CODEX' }).click();
  await expect(page.getByRole('heading', { name: 'Codex' })).toBeVisible();
  await expect.poll(() => detailFailures).toBeGreaterThan(0);
  await expect(page.getByRole('heading', { name: 'Codex' })).toBeVisible();
  await expect(page.getByText(/HTTP 500|details unavailable/)).toHaveCount(0);
  expect(pageErrors).toEqual([]);
});
