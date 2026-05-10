import { expect, test } from '@playwright/test';

import { emptyStatsPayload, fulfillEmptyLANScan, fulfillJSON, fulfillLANStatus, periods } from './dashboard-helpers';

test('settings device management can create edit clear and soft-delete devices', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));
  page.on('dialog', async (dialog) => {
    await dialog.accept();
  });

  type DeviceRow = {
    id: string;
    display_name: string;
    events: number;
    input_tokens: number;
    cached_tokens: number;
    cache_creation_tokens: number;
    output_tokens: number;
    total_cost: number;
    first_seen?: string;
    last_seen?: string;
  };

  let devices: DeviceRow[] = [{
    id: 'nas.local',
    display_name: 'nas.local',
    events: 12,
    input_tokens: 1000,
    cached_tokens: 100,
    cache_creation_tokens: 50,
    output_tokens: 25,
    total_cost: 3.5,
    first_seen: '2026-05-01T12:00:00Z',
    last_seen: '2026-05-10T12:00:00Z',
  }];
  let clearedAlias = '';
  let softDeletedDevice = '';

  await page.route('**/api/stats?*', async (route) => {
    await fulfillJSON(route, { ...emptyStatsPayload, periods, devices: [] });
  });
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);
  await page.route('**/api/lan/status', fulfillLANStatus);
  await page.route('**/api/pricing', async (route) => {
    await fulfillJSON(route, []);
  });
  await page.route('**/api/config', async (route) => {
    await fulfillJSON(route, { auto_start: false, extra_watch_dirs: [], enable_lan: true });
  });
  await page.route('**/api/devices**', async (route) => {
    if (route.request().method() === 'DELETE') {
      const url = new URL(route.request().url());
      softDeletedDevice = url.searchParams.get('device_id') ?? '';
      devices = devices.filter((device) => device.id !== softDeletedDevice);
      await fulfillJSON(route, { device_id: softDeletedDevice, superseded_count: 1 });
      return;
    }
    await fulfillJSON(route, devices);
  });
  await page.route('**/api/device-alias**', async (route) => {
    if (route.request().method() === 'POST') {
      const body = await route.request().postDataJSON() as { device_id: string; display_name: string };
      const existing = devices.find((device) => device.id === body.device_id);
      if (existing) {
        existing.display_name = body.display_name;
      } else {
        devices = [{
          id: body.device_id,
          display_name: body.display_name,
          events: 0,
          input_tokens: 0,
          cached_tokens: 0,
          cache_creation_tokens: 0,
          output_tokens: 0,
          total_cost: 0,
        }, ...devices];
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: '' });
      return;
    }
    if (route.request().method() === 'DELETE') {
      const url = new URL(route.request().url());
      clearedAlias = url.searchParams.get('device_id') ?? '';
      devices = devices.map((device) => device.id === clearedAlias ? { ...device, display_name: device.id } : device);
      await route.fulfill({ status: 200, contentType: 'application/json', body: '' });
      return;
    }
    await route.fulfill({ status: 405, contentType: 'text/plain', body: 'Method not allowed' });
  });

  await page.goto('/');
  await page.getByRole('button', { name: /系统设置|SETTINGS/ }).click();
  await page.getByRole('button', { name: /设备管理|DEVICE MANAGEMENT/ }).click();

  await expect(page.getByRole('row', { name: /nas\.local/ })).toBeVisible();

  await page.getByPlaceholder(/设备 ID|DEVICE ID/).fill('probe-local');
  await page.getByPlaceholder(/显示名称|DISPLAY NAME/).fill('Probe Local');
  await page.getByRole('button', { name: /\+ 添加别名|\+ ADD ALIAS/ }).click();
  await expect(page.getByText('Probe Local', { exact: true })).toBeVisible();

  const nasRow = page.locator('tr').filter({ hasText: 'nas.local' });
  await nasRow.getByRole('button', { name: /编辑|EDIT/ }).click();
  await nasRow.getByRole('textbox').fill('NAS');
  await nasRow.getByRole('button', { name: /保存|SAVE/ }).click();
  await expect(page.getByText('NAS', { exact: true })).toBeVisible();

  await nasRow.getByRole('button', { name: /清除别名|CLEAR ALIAS/ }).click();
  await expect.poll(() => clearedAlias).toBe('nas.local');

  const probeRow = page.locator('tr').filter({ hasText: 'probe-local' });
  await probeRow.getByRole('button', { name: /软删除|SOFT DELETE/ }).click();
  await expect.poll(() => softDeletedDevice).toBe('probe-local');
  await expect(page.getByText('probe-local', { exact: true })).toHaveCount(0);
  expect(pageErrors).toEqual([]);
});
