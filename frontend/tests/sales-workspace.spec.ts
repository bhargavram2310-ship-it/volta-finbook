import { expect, test } from '@playwright/test'
import { mockCaptureSettings } from './capture-fixtures'

const types = [
  { id: 'intl-id', values: { code: 'INTL', name: 'International', sales_class: 'international', active: 'true' } },
  { id: 'dom-id', values: { code: 'DOM', name: 'Domestic', sales_class: 'domestic', active: 'true' } },
  { id: 'misc-id', values: { code: 'MISC', name: 'Miscellaneous', sales_class: 'miscellaneous', active: 'true' } },
]
test.beforeEach(async ({ page }) => {
  await mockCaptureSettings(page)
  await page.route('**/api/v1/auth/me', route => route.fulfill({ json: { user: { displayName: 'Test user', username: 'test' } } }))
  await page.route('**/api/v1/status', route => route.fulfill({ json: { database: 'connected', schema_ready: true } }))
  await page.route('**/api/v1/masters/sales-types', route => route.fulfill({ json: { records: types } }))
  await page.route('**/api/v1/masters/sales-lookups', route => route.fulfill({ json: { airlines: [], currencies: [], customers: [], suppliers: [] } }))
})

test('one Sales Workspace supports all three master types without losing draft values', async ({ page }) => {
  await page.goto('/app/sales/workspace')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Sales Workspace.')
  await expect(page.getByLabel('Sales type', { exact: true })).toHaveValue('intl-id')
  await expect(page.getByRole('tab')).toHaveText(['GDS Capture', 'Entry', 'Refund', 'Cancellation'])
  await expect(page.getByRole('tab', { name: 'GDS Capture', exact: true })).toHaveAttribute('aria-selected', 'true')
  await page.getByRole('tab', { name: 'Entry', exact: true }).click()
  await page.getByLabel('Ticket number', { exact: true }).fill('REF-123')
  await page.getByLabel('PNR', { exact: true }).fill('KEEP01')
  await page.getByLabel('Client total', { exact: true }).fill('150.00')
  await page.getByRole('tab', { name: 'Entry', exact: true }).press('ArrowRight')
  await expect(page.getByRole('tabpanel', { name: 'Refund', exact: true })).toBeVisible()
  await page.getByRole('tab', { name: 'Refund', exact: true }).press('ArrowRight')
  await expect(page.getByRole('tabpanel', { name: 'Cancellation', exact: true })).toBeVisible()
  await page.getByRole('tab', { name: 'Cancellation', exact: true }).press('Home')
  await expect(page.getByRole('tab', { name: 'GDS Capture', exact: true })).toBeFocused()
  await page.getByRole('tab', { name: 'Entry', exact: true }).click()
  await expect(page.getByLabel('Ticket number', { exact: true })).toHaveValue('REF-123')
  await expect(page.getByLabel('PNR', { exact: true })).toHaveValue('KEEP01')
  await expect(page.getByLabel('Client total', { exact: true })).toHaveValue('150.00')
  await page.getByLabel('Sales type', { exact: true }).selectOption('dom-id')
  await expect(page.getByLabel('PNR', { exact: true })).toHaveValue('KEEP01')
  await expect.poll(() => new URL(page.url()).searchParams.get('type')).toBe('domestic')
  await page.getByLabel('Sales type', { exact: true }).selectOption('misc-id')
  await expect(page.getByLabel('Service reference', { exact: true })).toHaveValue('REF-123')
  await expect(page.getByLabel('Booking reference', { exact: true })).toHaveValue('KEEP01')
  await expect(page.getByLabel('Airline', { exact: true })).toHaveCount(0)
  await page.getByLabel('Service / description', { exact: true }).fill('Hotel booking')
  await expect(page.getByLabel('Client total', { exact: true })).toHaveValue('150.00')
  await page.getByLabel('Sales type', { exact: true }).selectOption('intl-id')
  await expect(page.getByLabel('PNR', { exact: true })).toHaveValue('KEEP01')
  await page.getByLabel('Sales type', { exact: true }).selectOption('misc-id')
  await expect(page.getByLabel('Service / description', { exact: true })).toHaveValue('Hotel booking')
  await expect(page.getByRole('link', { name: 'Manage sales types' })).toHaveAttribute('href', '/app/masters/sales-types')
  await page.getByRole('tab', { name: 'Cancellation', exact: true }).click()
  await page.reload()
  await expect(page.getByRole('tabpanel', { name: 'Cancellation', exact: true })).toBeVisible()
  await expect(page.getByLabel('Sales type', { exact: true })).toHaveValue('misc-id')
})

test('legacy entry links preserve their category and GDS mode on the shared screen', async ({ page }) => {
  for (const [legacy, classification, id] of [['international', 'international', 'intl-id'], ['domestic', 'domestic', 'dom-id'], ['miscellaneous', 'miscellaneous', 'misc-id']]) {
    await page.goto(`/app/${legacy}/sales?mode=gds`)
    await expect(page).toHaveURL(/\/app\/sales\/workspace\?/)
    expect(new URL(page.url()).searchParams.get('type')).toBe(classification)
    await expect.poll(() => new URL(page.url()).searchParams.get('mode')).toBe(classification === 'miscellaneous' ? 'manual' : 'gds')
    await expect(page.getByLabel('Sales type', { exact: true })).toHaveValue(id)
    if (classification === 'miscellaneous') {
      await expect(page.getByRole('tab', { name: 'GDS Capture', exact: true })).toHaveCount(0)
      await expect(page.getByRole('tab', { name: 'Entry', exact: true })).toHaveAttribute('aria-selected', 'true')
    } else await expect(page.getByRole('tab', { name: 'GDS Capture', exact: true })).toHaveAttribute('aria-selected', 'true')
  }
})

test('navigation has one sales entry point and the selector uses configured active master names', async ({ page, isMobile }) => {
  await page.route('**/api/v1/masters/sales-types', route => route.fulfill({ json: { records: types.map(type => type.id === 'dom-id' ? { ...type, values: { ...type.values, active: 'false' } } : type.id === 'misc-id' ? { ...type, values: { ...type.values, name: 'Other travel services' } } : type) } }))
  await page.goto('/app/sales/workspace')
  await expect(page.getByLabel('Sales type', { exact: true }).getByRole('option', { name: /Domestic/ })).toHaveCount(0)
  await expect(page.getByLabel('Sales type', { exact: true }).getByRole('option', { name: /Other travel services/ })).toHaveCount(1)
  if (isMobile) await page.getByRole('button', { name: 'Open navigation', exact: true }).click()
  const navigation = page.getByRole('complementary', { name: 'Main navigation' })
  await expect(navigation.getByRole('link', { name: 'Sales Workspace', exact: true })).toHaveCount(1)
  await expect(navigation.getByRole('link', { name: 'Sales entry', exact: true })).toHaveCount(0)
})
