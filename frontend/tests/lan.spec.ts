import { expect, test } from '@playwright/test';

import { emptyStatsPayload, fulfillJSON, periods } from './dashboard-helpers';

test('LAN radar shows per-source peer totals before full sync completes', async ({ page }) => {
  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, emptyStatsPayload);
  });
  await page.route('**/api/lan/scan', async (route) => {
    await fulfillJSON(route, {
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
    await fulfillJSON(route, { ...emptyStatsPayload, periods });
  });
  await page.route('**/api/lan/status', async (route) => {
    await fulfillJSON(route, { enabled: lanEnabled });
  });
  await page.route('**/api/lan/scan', async (route) => {
    await fulfillJSON(route, lanEnabled ? { peers: ['peer-a'], peer_infos: [{ id: 'peer-a', display_name: 'peer-a' }] } : { peers: [] });
  });
  await page.route('**/api/lan/join', async (route) => {
    lanActions.push('join');
    lanEnabled = true;
    await fulfillJSON(route, { enabled: true });
  });
  await page.route('**/api/lan/leave', async (route) => {
    lanActions.push('leave');
    lanEnabled = false;
    await fulfillJSON(route, { enabled: false });
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
