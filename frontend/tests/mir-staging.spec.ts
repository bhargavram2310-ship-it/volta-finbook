import { expect, test } from '@playwright/test'
import { mockCaptureSettings } from './capture-fixtures'

// Synthetic HTTP fixtures for UI behavior only; actual MySQL and parser tests
// live in backend/internal/mir/staging_integration_test.go.
const record = { id: 1, source_file_id: 1, batch_id: 1, mir_date: '2026-11-05', first_travel_date: '2026-12-10', locator: 'SYN001', mir_type: 'H', passenger: 'SYNTHETIC/ONE (+1)', ticket_number: '1231234567890', currency: 'USD', fare_amount: '120.00', status: 'review', filename: 'SYNTHETIC.MIR', parser_version: 'finbook-mir-1.0.0' }
const batch = { id: 1, status: 'completed', imported_count: 1, duplicate_count: 0, failed_count: 0, created_at: '2026-11-05T10:00:00Z', errors: [] }
const detail = { ...record, header: { T50DTE: '05NOV26', T50DFT: '10DEC26' }, typed: { passenger: [{ name: 'SYNTHETIC/ONE', mapping_status: 'partial', decoded_data: { A02NME: 'SYNTHETIC/ONE' }, source_item_id: 2, section_id: 2 }], fare: [], misc_document: [] }, relationships: [], sections: [{ id: 2, identifier: 'A02', occurrence: 1, decoder_status: 'partial', decoder_reason: 'Synthetic UI test coverage', raw_hex: '413032', items: [] }], raw: 'SYNTHETIC RAW MIR FOR UI TEST ONLY', raw_hex: '53594e', issues: [{ Severity: 'warning', Code: 'SYNTHETIC_TEST', Message: 'Clearly labelled synthetic fixture.' }], coverage: [{ Section: 'A18', Status: 'unsupported', Reason: 'Discontinued by guide; original bytes retained' }] }

test.beforeEach(async ({ page }) => {
  await mockCaptureSettings(page)
  await page.route('**/api/v1/auth/me', route => route.fulfill({ json: { user: { displayName: 'Synthetic Test', username: 'synthetic' } } }))
  await page.route('**/api/v1/status', route => route.fulfill({ json: { database: 'connected', schema_ready: true } }))
  await page.route('**/api/v1/mir/records?*', route => route.fulfill({ json: { records: [record], total: 1, date_basis: 'T50DTE', page: 1, page_size: 25 } }))
  await page.route('**/api/v1/mir/batches?*', route => route.fulfill({ json: { batches: [batch] } }))
  await page.route('**/api/v1/mir/batches/1', route => route.fulfill({ json: batch }))
  await page.route('**/api/v1/mir/records/1', route => route.fulfill({ json: detail }))
  await page.route('**/api/v1/mir/records/1/reprocess', route => route.fulfill({ json: detail }))
  await page.route('**/api/v1/mir/import', route => route.fulfill({ status: 201, json: batch }))
})

test('uses MIR creation dates and shows all staged record detail sections', async ({ page, isMobile }) => {
  await page.goto('/app/administration/mir-report')
  await expect(page.getByRole('heading', { name: 'MIR Report', exact: true })).toBeVisible()
  if (isMobile) {
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: '../.local/mir-staging-mobile.png', fullPage: true })
  }
  await expect(page.getByText('Date filters use MIR creation date', { exact: false })).toBeVisible()
  await page.getByLabel('From Date', { exact: true }).fill('2026-11-05')
  await page.getByLabel('To Date', { exact: true }).fill('2026-11-05')
  const request = page.waitForRequest(r => r.url().includes('/mir/records?') && r.url().includes('from_date=2026-11-05'))
  await page.getByRole('button', { name: 'Apply filters' }).click()
  const url = new URL((await request).url())
  expect(url.searchParams.get('to_date')).toBe('2026-11-05')
  await page.getByRole('button', { name: 'SYN001', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByText('MIR creation date (T50DTE)', { exact: true })).toBeVisible()
  expect(await dialog.getByLabel('Detail section').locator('option').count()).toBe(17)
  await dialog.getByLabel('Detail section').selectOption('Passengers')
  await expect(dialog.getByText('SYNTHETIC/ONE', { exact: true })).toBeVisible()
  await dialog.getByLabel('Detail section').selectOption('Raw MIR')
  await expect(dialog.getByText(detail.raw, { exact: true })).toBeVisible()
  await dialog.getByLabel('Detail section').selectOption('Validation Issues')
  await expect(dialog.getByText('Clearly labelled synthetic fixture.')).toBeVisible()
  await dialog.getByRole('button', { name: 'Reprocess record' }).click()
  await expect(page.getByRole('status').filter({ hasText: 'Record reprocessed' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Proceed to sales entry' })).toHaveCount(0)
})

test('uploads multipart MIR files and shows batch results', async ({ page }) => {
  await page.goto('/app/administration/mir-report')
  await expect(page.getByRole('button', { name: 'SYN001', exact: true })).toBeVisible()
  await page.getByLabel('Upload MIR Files', { exact: true }).setInputFiles({ name: 'SYNTHETIC.MIR', mimeType: 'application/octet-stream', buffer: Buffer.from('SYNTHETIC UI TEST ONLY') })
  const request = page.waitForRequest('**/api/v1/mir/import')
  await page.getByRole('button', { name: 'Import (1)', exact: true }).click()
  expect((await request).headers()['content-type']).toContain('multipart/form-data')
  await expect(page.getByRole('dialog', { name: 'Import batch 1' })).toBeVisible()
  await expect(page.getByText('No batch errors.')).toBeVisible()
  await page.getByRole('button', { name: 'Close dialog' }).click()
  await expect(page.getByRole('button', { name: 'Import', exact: true })).toBeDisabled()
})

test('handles service errors and keeps extraction in Admin Control', async ({ page }) => {
  await page.route('**/api/v1/mir/records?*', route => route.fulfill({ status: 503, json: { error: 'mir_database_unavailable' } }))
  await page.goto('/app/administration/mir-report')
  await expect(page.getByRole('alert')).toHaveText('mir database unavailable')
  await page.goto('/app/international/sales?mode=mir')
  await expect(page.getByRole('tab', { name: 'GDS Capture' })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('link', { name: 'MIR Import Report', exact: true })).toHaveAttribute('href', '/app/administration/mir-report')
  await expect(page.getByRole('button', { name: 'Proceed to sales entry' })).toHaveCount(0)
})
