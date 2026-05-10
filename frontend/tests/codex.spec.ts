import { expect, test } from '@playwright/test';

import { fulfillEmptyLANScan, periods } from './dashboard-helpers';

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
  await page.route('**/api/lan/scan', fulfillEmptyLANScan);

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
