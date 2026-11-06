import { expect, test } from '@playwright/test'
import { captureResponse, mockCaptureSettings } from './capture-fixtures'

const first = { record_id: 1, passenger_id: 2, ticket_number: '1231234567890', pnr: 'SYN001', gds: '1G', staff: '', client_reference: '', invoice_number: '', branch: '', passenger_name: 'SYNTHETIC/ONE', issue_date: '2026-10-21', issue_date_source: '21OCT26', sector: 'AAA–BBB', web_reference: '', fare: '120.00', currency: 'USD', processing_date: '2026-11-05' }
const second = { ...first, passenger_id: 3, ticket_number: '1231234567891', passenger_name: 'SYNTHETIC/TWO' }
const masterLists = {
  airlines: [{ id: 'airline-id', code: 'ZZ', name: 'Test Airline' }],
  currencies: [{ id: 'USD', code: 'USD', name: 'US dollar' }],
  customers: [{ id: 'customer-id', code: 'C1', name: 'Test Customer' }],
  suppliers: [{ id: 'supplier-id', code: 'S1', name: 'Test Supplier' }],
}
function draft(row: typeof first) {
  const field = (value: string) => ({ value, lines: [1], fieldIds: ['synthetic-evidence'] })
  return {
    id: `mir:${row.record_id}:passenger:${row.passenger_id}`, fileName: 'SYNTHETIC.MIR', sha256: 'a'.repeat(64), recordDate: '2026-11-05', status: 'Validation required',
    fields: { ticketNumber: field(row.ticket_number), passengerName: field(row.passenger_name), pnr: field(row.pnr), airline: field('ZZ'), ticketDate: field('21OCT26'), travelDate: field('10DEC26'), sector: field('AAA → BBB'), basicFare: field('100.00'), basicCurrency: field('USD'), mirTotalFare: field('120.00'), fareCurrency: field('USD') },
    rawLines: [{ number: 1, text: 'SYNTHETIC SOURCE EVIDENCE', rawHex: '53594e', terminatorHex: '0d' }], issues: ['Synthetic fixture for selection testing.'],
  }
}
test.beforeEach(async ({ page }) => {
  await mockCaptureSettings(page)
  await page.route('**/api/v1/auth/me', route => route.fulfill({ json: { user: { displayName: 'Test user', username: 'test' } } }))
  await page.route('**/api/v1/status', route => route.fulfill({ json: { database: 'connected', schema_ready: true } }))
  await page.route('**/api/v1/masters/sales-lookups', route => route.fulfill({ json: masterLists }))
  await page.route('**/api/v1/mir/capture?*', route => route.fulfill({ json: captureResponse([first, second]) }))
  await page.route('**/api/v1/mir/capture/*/draft?*', route => route.fulfill({ json: draft(new URL(route.request().url()).pathname.includes('/3/') ? second : first) }))
})

test('Proceed loads the selected ticket, maps masters, and allows completing missing accounting fields', async ({ page }) => {
  await page.goto('/app/international/sales?mode=gds')
  await expect(page.getByRole('button', { name: 'Open sales draft', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: 'Search Tickets' }).click()
  await page.getByRole('radio', { name: `Select ticket ${second.ticket_number}`, exact: true }).check()
  const request = page.waitForRequest('**/api/v1/mir/capture/3/draft?record_id=1')
  await page.getByRole('button', { name: 'Open sales draft', exact: true }).click()
  await request
  await expect(page.getByRole('tab', { name: 'Entry', exact: true })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByLabel('Ticket number', { exact: true })).toHaveValue(second.ticket_number)
  await expect(page.getByLabel('Passenger name', { exact: true })).toHaveValue(second.passenger_name)
  await expect(page.getByLabel('PNR', { exact: true })).toHaveValue('SYN001')
  await expect(page.getByLabel('Sector', { exact: true })).toHaveValue('AAA → BBB')
  await expect(page.getByRole('combobox', { name: 'Airline', exact: true })).toHaveValue('airline-id')
  await expect(page.getByLabel('Basic fare', { exact: true })).toHaveValue('100.00')
  await expect(page.getByLabel('MIR ticket fare total', { exact: true })).toHaveValue('120.00')
  await expect(page.getByRole('combobox', { name: 'Ticket fare currency', exact: true })).toHaveValue('USD')
  await expect(page.getByLabel('Client total', { exact: true })).toHaveValue('')
  await expect(page.getByLabel('Supplier total', { exact: true })).toHaveValue('')
  await page.getByRole('combobox', { name: 'Bill to', exact: true }).selectOption('customer-id')
  await page.getByRole('combobox', { name: 'Supplier', exact: true }).selectOption('supplier-id')
  await page.getByLabel('Client total', { exact: true }).fill('150.00')
  await page.getByRole('button', { name: 'View source & issues' }).click()
  await expect(page.getByRole('dialog').getByText(/SYNTHETIC SOURCE EVIDENCE/)).toBeVisible()
})

test('proceeding again with the same ticket retains corrections and unsaved accounting values', async ({ page }) => {
  await page.goto('/app/international/sales?mode=gds')
  await page.getByRole('button', { name: 'Search Tickets' }).click()
  await page.getByRole('radio', { name: `Select ticket ${first.ticket_number}`, exact: true }).check()
  await page.getByRole('button', { name: 'Open sales draft', exact: true }).click()
  await page.getByLabel('Client total', { exact: true }).fill('150.00')
  await page.getByRole('button', { name: 'Correct Passenger name', exact: true }).click()
  await page.getByLabel('New value', { exact: true }).fill('CORRECTED/ONE')
  await page.getByLabel('Reason for correction', { exact: true }).fill('Name confirmed from ticket.')
  await page.getByRole('button', { name: 'Apply correction' }).click()
  await page.getByRole('tab', { name: 'GDS Capture', exact: true }).click()
  await page.getByRole('button', { name: 'Search Tickets' }).click()
  await page.getByRole('radio', { name: `Select ticket ${first.ticket_number}`, exact: true }).check()
  await page.getByRole('button', { name: 'Open sales draft', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByLabel('Passenger name', { exact: true })).toHaveValue('CORRECTED/ONE')
  await expect(page.getByLabel('Client total', { exact: true })).toHaveValue('150.00')
})

test('a new selection asks before replacing a manual draft and request failures retain it', async ({ page }) => {
  await page.goto('/app/international/sales?mode=manual')
  await page.getByLabel('Ticket number', { exact: true }).fill('UNSAVED-TICKET')
  await page.getByRole('tab', { name: 'GDS Capture', exact: true }).click()
  await page.getByRole('button', { name: 'Search Tickets' }).click()
  await page.getByRole('radio', { name: `Select ticket ${first.ticket_number}`, exact: true }).check()
  await page.getByRole('button', { name: 'Open sales draft', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'Replace current draft?' })).toBeVisible()
  await page.getByRole('button', { name: 'Keep current draft' }).click()
  await page.route('**/api/v1/mir/capture/*/draft?*', route => route.fulfill({ status: 404, json: { error: 'Selected ticket changed. Refresh GDS Capture.' } }))
  await page.getByRole('button', { name: 'Open sales draft', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Selected ticket changed')
  await page.getByRole('tab', { name: 'Entry', exact: true }).click()
  await expect(page.getByLabel('Ticket number', { exact: true })).toHaveValue('UNSAVED-TICKET')
})

test('applying another date range clears the ticket selection', async ({ page }) => {
  await page.goto('/app/international/sales?mode=gds')
  await page.getByRole('button', { name: 'Search Tickets' }).click()
  await page.getByRole('radio', { name: `Select ticket ${first.ticket_number}`, exact: true }).check()
  await expect(page.getByRole('button', { name: 'Open sales draft', exact: true })).toBeEnabled()
  await page.getByRole('button', { name: 'Search Tickets' }).click()
  await expect(page.getByRole('button', { name: 'Open sales draft', exact: true })).toBeDisabled()
  await expect(page.getByRole('radio', { name: `Select ticket ${first.ticket_number}`, exact: true })).not.toBeChecked()
})

test('Proceed retains the selected Domestic classification in the unified workspace', async ({ page }) => {
  await page.route('**/api/v1/masters/sales-types', route => route.fulfill({ json: { records: [
    { id: 'intl-id', values: { code: 'INTL', name: 'International', sales_class: 'international', active: 'true' } },
    { id: 'dom-id', values: { code: 'DOM', name: 'Domestic', sales_class: 'domestic', active: 'true' } },
  ] } }))
  await page.goto('/app/sales/workspace?type=domestic&mode=gds')
  await expect(page.getByLabel('Sales type', { exact: true })).toHaveValue('dom-id')
  await page.getByRole('button', { name: 'Search Tickets' }).click()
  await page.getByRole('radio', { name: `Select ticket ${first.ticket_number}`, exact: true }).check()
  await page.getByRole('button', { name: 'Open sales draft', exact: true }).click()
  await expect(page.getByRole('tab', { name: 'Entry', exact: true })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByLabel('Sales type', { exact: true })).toHaveValue('dom-id')
  await expect(page.getByLabel('Ticket number', { exact: true })).toHaveValue(first.ticket_number)
  expect(new URL(page.url()).searchParams.get('type')).toBe('domestic')
})
