import { expect, test } from '@playwright/test';

const periods = ['1h', '24h', '7d', '30d', '3mo', '6mo', '1y', 'ALL'].map((label) => ({
  label,
  input_tokens: 0,
  cached_tokens: 0,
  cache_creation_tokens: 0,
  output_tokens: 0,
  cost: 0,
  cache_hit_rate: 0,
}));

test('CODEX tab stays usable against real backend on first install data', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));

  await page.goto('/');
  await expect(page.getByRole('button', { name: 'CODEX' })).toBeVisible();

  await page.getByRole('button', { name: 'CODEX' }).click();

  await expect(page.getByRole('button', { name: 'CODEX' })).toBeVisible();
  await expect(page.locator('body')).not.toBeEmpty();
  await expect(page.getByText('SYSTEM ERROR')).toHaveCount(0);
  expect(pageErrors).toEqual([]);
});

test('CODEX tab tolerates malformed Codex stats without a blank page', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));
  const requestedSources: string[] = [];

  await page.route('**/api/stats?*', async (route) => {
    const url = new URL(route.request().url());
    const source = url.searchParams.get('source') ?? '';
    requestedSources.push(source);

    const sources = source === 'Codex'
      ? [{
          name: 'Codex',
          total_input: '2000',
          total_cached: null,
          total_cache_creation: undefined,
          total_output: 300,
          total_cost: null,
          total_events: 1,
          models: [{
            model: null,
            events: 1,
            input_tokens: 2000,
            cached_tokens: null,
            cache_creation_tokens: undefined,
            output_tokens: 300,
            total_cost: null,
            input_price_per_m: null,
            cached_price_per_m: undefined,
            cache_creation_price_per_m: 5,
            output_price_per_m: '30',
          }],
        }]
      : [];

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        periods,
        sources,
        devices: [{ id: 'windows-box', display_name: 'windows-box' }],
        projects: source === 'Codex'
          ? [{
              project: null,
              events: 1,
              input_tokens: 2000,
              cached_tokens: null,
              cache_creation_tokens: undefined,
              output_tokens: 300,
              total_cost: null,
            }]
          : [],
        is_paused: false,
      }),
    });
  });
  await page.route('**/api/lan/scan', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ peers: [] }),
    });
  });

  await page.goto('/');
  await expect(page.getByRole('button', { name: 'CODEX' })).toBeVisible();

  await page.getByRole('button', { name: 'CODEX' }).click();

  await expect.poll(() => requestedSources).toContain('Codex');
  await expect(page.getByRole('heading', { name: 'Codex' })).toBeVisible();
  await expect(page.getByText('SYSTEM ERROR')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'CODEX' })).toBeEnabled();
  await expect(page.locator('body')).not.toBeEmpty();
  expect(pageErrors).toEqual([]);
});

test('LAN radar shows per-source peer totals before full sync completes', async ({ page }) => {
  await page.route('**/api/stats?*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        periods,
        sources: [],
        devices: [{ id: 'local', display_name: 'local' }],
        projects: [],
        is_paused: false,
      }),
    });
  });
  await page.route('**/api/lan/scan', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        peers: ['icebear'],
        peer_infos: [{
          id: 'icebear',
          display_name: 'icebear',
          ip: '192.168.10.5',
          http_port: 19100,
          sync_status: 'syncing',
          tokens_24h: 6000,
          tokens_total: 13000,
          cost_total: 12.34,
          sources: [
            { source: 'Claude Code', tokens_24h: 1000, tokens_total: 2000, cost_total: 2.00 },
            { source: 'Codex', tokens_24h: 2000, tokens_total: 4000, cost_total: 4.00 },
            { source: 'Gemini CLI', tokens_24h: 3000, tokens_total: 7000, cost_total: 6.34 },
          ],
        }],
      }),
    });
  });

  await page.goto('/');

  const lanTable = page.locator('tbody');
  await expect(lanTable.getByText('Claude Code')).toBeVisible();
  await expect(lanTable.getByText('Codex', { exact: true })).toBeVisible();
  await expect(lanTable.getByText('Gemini CLI')).toBeVisible();
  await expect(lanTable.getByText('2.0K', { exact: true })).toBeVisible();
  await expect(lanTable.getByText('4.0K', { exact: true })).toBeVisible();
  await expect(lanTable.getByText('7.0K', { exact: true })).toBeVisible();
});

test('LAN radar can leave and rejoin the network', async ({ page }) => {
  let lanEnabled = false;
  const lanActions: string[] = [];

  await page.route('**/api/stats?*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        periods,
        sources: [],
        devices: [{ id: 'local', display_name: 'local' }],
        projects: [],
        is_paused: false,
      }),
    });
  });
  await page.route('**/api/lan/status', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ enabled: lanEnabled }),
    });
  });
  await page.route('**/api/lan/scan', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(lanEnabled ? { peers: ['peer-a'], peer_infos: [{ id: 'peer-a', display_name: 'peer-a' }] } : { peers: [] }),
    });
  });
  await page.route('**/api/lan/join', async (route) => {
    lanActions.push('join');
    lanEnabled = true;
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ enabled: true }),
    });
  });
  await page.route('**/api/lan/leave', async (route) => {
    lanActions.push('leave');
    lanEnabled = false;
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ enabled: false }),
    });
  });

  await page.goto('/');

  await expect(page.getByRole('button', { name: '加入网络' })).toBeVisible();
  await page.getByRole('button', { name: '加入网络' }).click();
  await expect(page.getByRole('button', { name: '退出网络' })).toBeVisible();

  await page.getByRole('button', { name: '退出网络' }).click();
  await expect(page.getByRole('button', { name: '加入网络' })).toBeVisible();

  await page.getByRole('button', { name: '加入网络' }).click();
  await expect(page.getByRole('button', { name: '退出网络' })).toBeVisible();
  expect(lanActions).toEqual(['join', 'leave', 'join']);
});

test('system logs click shows a visible fallback in web mode', async ({ page }) => {
  await page.route('**/api/stats?*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        periods,
        sources: [],
        devices: [{ id: 'local', display_name: 'local' }],
        projects: [],
        is_paused: false,
      }),
    });
  });
  await page.route('**/api/lan/scan', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ peers: [] }),
    });
  });
  await page.route('**/api/lan/status', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ enabled: true }),
    });
  });
  await page.route('**/api/system/logs', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ path: '/tmp/ai-flight-dashboard/stats' }),
    });
  });

  await page.goto('/');

  await page.getByText('[ 系统日志 ]').click();
  await expect(page.getByText('系统日志路径: /tmp/ai-flight-dashboard/stats')).toBeVisible();
});

test('dashboard shows cache hit rate in stats tables', async ({ page }) => {
  await page.route('**/api/stats?*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        periods: periods.map((period) => ({
          ...period,
          input_tokens: 1000,
          cached_tokens: 250,
          cache_hit_rate: 25,
        })),
        sources: [{
          name: 'Codex',
          total_input: 1000,
          total_cached: 250,
          total_cache_creation: 100,
          total_output: 50,
          total_cost: 1.23,
          total_events: 1,
          cache_hit_rate: 25,
          models: [{
            model: 'gpt-5.5',
            events: 1,
            input_tokens: 1000,
            cached_tokens: 250,
            cache_creation_tokens: 100,
            output_tokens: 50,
            total_cost: 1.23,
            input_price_per_m: 2,
            cached_price_per_m: 0.5,
            cache_creation_price_per_m: 5,
            output_price_per_m: 10,
            cache_hit_rate: 25,
          }],
        }],
        devices: [{ id: 'local', display_name: 'local' }],
        projects: [{
          project: 'token',
          events: 1,
          input_tokens: 1000,
          cached_tokens: 250,
          cache_creation_tokens: 100,
          output_tokens: 50,
          total_cost: 1.23,
          cache_hit_rate: 25,
        }],
        is_paused: false,
      }),
    });
  });
  await page.route('**/api/lan/scan', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ peers: [] }),
    });
  });

  await page.goto('/');

  await expect(page.getByText('缓存命中率').first()).toBeVisible();
  await expect(page.getByText('25.0%').first()).toBeVisible();
});

test('dashboard period cards show raw input tokens without subtracting cache', async ({ page }) => {
  await page.route('**/api/stats?*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        periods: periods.map((period) => ({
          ...period,
          input_tokens: 322_400_000,
          cached_tokens: 304_900_000,
          output_tokens: 1_100_000,
          cache_hit_rate: 94.6,
        })),
        sources: [],
        devices: [{ id: 'local', display_name: 'local' }],
        projects: [],
        is_paused: false,
      }),
    });
  });
  await page.route('**/api/lan/scan', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ peers: [] }),
    });
  });

  await page.goto('/');

  await expect(page.getByText('输入: 322.40M').first()).toBeVisible();
  await expect(page.getByText('缓存读取: 304.90M').first()).toBeVisible();
  await expect(page.getByText('输入: 17.50M')).toHaveCount(0);
});

test('dashboard can collapse project and model tables', async ({ page }) => {
  await page.route('**/api/stats?*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        periods,
        sources: [{
          name: 'Codex',
          total_input: 1000,
          total_cached: 250,
          total_cache_creation: 100,
          total_output: 50,
          total_cost: 1.23,
          total_events: 2,
          cache_hit_rate: 25,
          models: [
            {
              model: 'gpt-5.5',
              events: 1,
              input_tokens: 1000,
              cached_tokens: 250,
              cache_creation_tokens: 100,
              output_tokens: 50,
              total_cost: 1.23,
              input_price_per_m: 2,
              cached_price_per_m: 0.5,
              cache_creation_price_per_m: 5,
              output_price_per_m: 10,
              cache_hit_rate: 25,
            },
            {
              model: 'gpt-5.4',
              events: 1,
              input_tokens: 500,
              cached_tokens: 100,
              cache_creation_tokens: 50,
              output_tokens: 25,
              total_cost: 0.42,
              input_price_per_m: 1,
              cached_price_per_m: 0.25,
              cache_creation_price_per_m: 2,
              output_price_per_m: 5,
              cache_hit_rate: 20,
            },
          ],
        }],
        devices: [{ id: 'local', display_name: 'local' }],
        projects: [
          {
            project: 'token',
            events: 1,
            input_tokens: 1000,
            cached_tokens: 250,
            cache_creation_tokens: 100,
            output_tokens: 50,
            total_cost: 1.23,
            cache_hit_rate: 25,
          },
          {
            project: 'codex-proxy',
            events: 1,
            input_tokens: 500,
            cached_tokens: 100,
            cache_creation_tokens: 50,
            output_tokens: 25,
            total_cost: 0.42,
            cache_hit_rate: 20,
          },
        ],
        is_paused: false,
      }),
    });
  });
  await page.route('**/api/lan/scan', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ peers: [] }),
    });
  });

  await page.goto('/');

  await expect(page.getByText('token', { exact: true })).toBeVisible();
  await expect(page.getByText('gpt-5.5', { exact: true })).toBeVisible();

  await page.getByRole('button', { name: '收起项目统计' }).click();
  await expect(page.getByText('token', { exact: true })).toBeHidden();
  await expect(page.getByRole('button', { name: '展开项目统计' })).toBeVisible();

  await page.getByRole('button', { name: '收起模型列表' }).click();
  await expect(page.getByText('gpt-5.5', { exact: true })).toBeHidden();
  await expect(page.getByRole('button', { name: '展开模型列表' })).toBeVisible();

  await page.getByRole('button', { name: '展开项目统计' }).click();
  await page.getByRole('button', { name: '展开模型列表' }).click();
  await expect(page.getByText('token', { exact: true })).toBeVisible();
  await expect(page.getByText('gpt-5.5', { exact: true })).toBeVisible();
});
