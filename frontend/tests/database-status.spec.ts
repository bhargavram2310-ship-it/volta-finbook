import { expect, test } from '@playwright/test'

test('database connection and pending migrations are reported without enabling posting', async ({ page }) => {
  await page.route('**/api/v1/status', route => route.fulfill({ json: { database: 'connected_schema_pending', schema_ready: false } }))
  await page.goto('/app/international/sales')
  await expect(page.getByText('Cloud connected · migrations pending', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Post', exact: true })).toBeDisabled()
  await page.unroute('**/api/v1/status')
  await page.route('**/api/v1/status', route => route.fulfill({ json: { database: 'connected', schema_ready: true } }))
  await page.reload()
  await expect(page.getByText('Cloud connected · schema ready', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Post', exact: true })).toBeDisabled()
})

test('an unavailable database does not appear connected', async ({ page }) => {
  await page.route('**/api/v1/status', route => route.fulfill({ status: 503, json: { error: 'unavailable' } }))
  await page.goto('/app/dashboard')
  await expect(page.getByText('Cloud database unavailable', { exact: true })).toBeVisible()
})
