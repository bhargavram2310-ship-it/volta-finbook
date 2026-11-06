import { expect, test } from '@playwright/test'
import { captureResponse, captureSettings, mockCaptureSettings, readyProgress, reviewProgress } from './capture-fixtures'

const capture = {
  record_id: 1, passenger_id: 2, ticket_number: '1231234567890', pnr: 'SYN001', gds: '1G', staff: 'AA', client_reference: 'TEST-CLIENT',
  invoice_number: 'INV000001', branch: 'TEST', passenger_name: 'SYNTHETIC/ONE', issue_date: '2026-11-04', issue_date_source: '04NOV26',
  sector: 'AAA–BBB', web_reference: '', fare: '120.00', currency: 'USD', processing_date: '2026-11-05', imported_at: '2026-11-05T20:00:00Z', progress: reviewProgress,
}
const source = { id: 'mir:1:passenger:2', fileName: 'SYNTHETIC.MIR', sha256: 'a'.repeat(64), recordDate: '2026-11-05', status: 'Validation required', fields: {
  ticketNumber: { value: capture.ticket_number, lines: [1], fieldIds: ['synthetic-ticket'] }, passengerName: { value: capture.passenger_name, lines: [1], fieldIds: ['synthetic-name'] },
}, rawLines: [], issues: ['Review the source validation issues.'] }
const details = { ticket: capture, draft: source, draft_error: '', taxes: [{ code: 'XX', amount: '20.00', currency: 'USD' }], customer_mapping: 'Choose a customer in Entry. The source client reference has no verified customer-master mapping.', accounting_note: 'Imported fare is source evidence. Review the final sale amount in Entry.' }

test.beforeEach(async ({ page }) => {
  await mockCaptureSettings(page)
  await page.route('**/api/v1/auth/me', route => route.fulfill({ json: { user: { displayName: 'Test user', username: 'test' } } }))
  await page.route('**/api/v1/status', route => route.fulfill({ json: { database: 'connected', schema_ready: true } }))
  await page.route('**/api/v1/masters/sales-types', route => route.fulfill({ json: { records: [{ id: 'intl-id', values: { code: 'INTL', name: 'International', sales_class: 'international', active: 'true' } }] } }))
})

test('compact header, primary ticket columns, session column choices and responsive layout', async ({ page, isMobile }) => {
  let requests = 0
  await page.route('**/api/v1/mir/capture?*', route => { requests++; return route.fulfill({ json: captureResponse([capture]) }) })
  await page.goto('/app/sales/workspace?mode=gds')
  await expect(page.locator('.sales-page-header').getByLabel('Sales type', { exact: true })).toBeVisible()
  await expect(page.locator('.sales-type-bar')).toHaveCount(0)
  await expect(page.locator('.capture-empty')).toHaveText('Select a date range and search for tickets.')
  expect(requests).toBe(0)
  await page.getByLabel('From Date', { exact: true }).fill('2026-11-04')
  await page.getByLabel('To Date', { exact: true }).fill('2026-11-04')
  const request = page.waitForRequest('**/api/v1/mir/capture?*')
  await page.getByRole('button', { name: 'Search Tickets', exact: true }).click()
  const params = new URL((await request).url()).searchParams
  expect(params.get('date_basis')).toBe('processing')
  expect(params.get('from_date')).toBe('2026-11-04')
  await expect(page.getByRole('columnheader')).toHaveText(['Selection', 'Ticket Number', 'Passenger Name', 'PNR', 'GDS', 'Route', 'Issue Date', 'Amount', 'Status', 'Actions'])
  await expect(page.getByRole('button', { name: capture.ticket_number, exact: true })).toBeVisible()
  await expect(page.locator('.capture-amount')).toContainText('USD 120.00')
  await expect(page.locator('.capture-amount small')).toHaveText('Imported fare')
  await expect(page.getByRole('status').filter({ hasText: '1 matching ticket' })).toBeVisible()
  await page.locator('.capture-columns summary').click()
  for (const name of ['Staff', 'Client Reference', 'Invoice Number', 'Branch', 'Web Reference', 'Imported Date']) await expect(page.getByRole('checkbox', { name, exact: true })).toBeVisible()
  await page.getByRole('checkbox', { name: 'Staff', exact: true }).check()
  await page.getByRole('checkbox', { name: 'Imported Date', exact: true }).check()
  await page.locator('.capture-columns summary').click()
  await page.reload()
  await expect(page.getByRole('columnheader', { name: 'Staff', exact: true })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: 'Imported Date', exact: true })).toHaveCount(1)
  expect(await page.getByRole('columnheader', { name: 'Ticket Number', exact: true }).evaluate(element => getComputedStyle(element).position)).toBe('sticky')
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.getByRole('button', { name: 'Search Tickets', exact: true }).click()
  await expect(page.getByRole('button', { name: capture.ticket_number, exact: true })).toBeVisible()
  await page.evaluate(() => { if (document.activeElement instanceof HTMLElement) document.activeElement.blur(); window.scrollTo(0, 0) })
  await page.screenshot({ path: `../.local/sales-workspace-${isMobile ? 'mobile' : 'desktop'}.png`, fullPage: true })
})

test('empty results, request failures and retry show honest search state', async ({ page }) => {
  let fail = false
  await page.route('**/api/v1/mir/capture?*', route => fail ? route.fulfill({ status: 503, json: { error: 'gds_capture_unavailable' } }) : route.fulfill({ json: captureResponse([]) }))
  await page.goto('/app/international/sales?mode=mir')
  await page.getByRole('button', { name: 'Search Tickets', exact: true }).click()
  await expect(page.locator('.capture-empty')).toHaveText('No tickets found for the selected filters.')
  fail = true
  await page.getByRole('button', { name: 'Search Tickets', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Tickets could not be loaded')
  await expect(page.getByRole('status').filter({ hasText: 'matching' })).toHaveCount(0)
  fail = false
  await page.getByRole('button', { name: 'Retry search', exact: true }).click()
  await expect(page.getByRole('status').filter({ hasText: '0 matching tickets' })).toBeVisible()
})

test('issue dates remain distinct from imported dates and Reset restores the initial state', async ({ page }) => {
  await page.route('**/api/v1/mir/capture?*', route => route.fulfill({ json: captureResponse(new URL(route.request().url()).searchParams.get('date_basis') === 'issue' ? [] : [{ ...capture, issue_date: '2026-10-21' }]) }))
  await page.goto('/app/sales/workspace?mode=gds')
  await expect(page.getByLabel('Filter by', { exact: true }).getByRole('option')).toHaveText(['Imported Date', 'Ticket Issue Date'])
  await expect(page.getByText('Business timezone:', { exact: false })).toContainText('UTC')
  await page.getByLabel('Filter by', { exact: true }).selectOption('issue')
  await page.getByRole('button', { name: 'Search Tickets', exact: true }).click()
  await expect(page.locator('.capture-empty')).toContainText('No tickets found for the selected filters.')
  await page.getByRole('button', { name: 'Search by Imported Date', exact: true }).click()
  await expect(page.getByRole('cell', { name: '2026-10-21', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Reset', exact: true }).click()
  await expect(page.locator('.capture-empty')).toHaveText('Select a date range and search for tickets.')
  await expect(page.getByLabel('From Date', { exact: true })).toHaveValue(captureSettings.today)
  await expect(page.getByLabel('Filter by', { exact: true })).toHaveValue('processing')
})

test('date order validation, Enter search, configured timezone and pagination preserve the applied filters', async ({ page }) => {
  await mockCaptureSettings(page, { ...captureSettings, timezone: 'Asia/Kolkata' })
  const requests: URL[] = []
  await page.route('**/api/v1/mir/capture?*', route => { const url = new URL(route.request().url()); requests.push(url); return route.fulfill({ json: captureResponse([capture], 26, 'Asia/Kolkata') }) })
  await page.goto('/app/sales/workspace?mode=gds')
  await page.getByLabel('From Date', { exact: true }).fill('2026-11-07')
  await page.getByLabel('To Date', { exact: true }).fill('2026-11-06')
  await page.getByRole('button', { name: 'Search Tickets', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('From Date must be on or before To Date.')
  expect(requests).toHaveLength(0)
  await page.getByLabel('From Date', { exact: true }).fill('2026-11-06')
  await page.getByPlaceholder('Ticket, PNR or passenger').fill('SYN001')
  await page.getByPlaceholder('Ticket, PNR or passenger').press('Enter')
  await expect(page.getByRole('status').filter({ hasText: '26 matching tickets' })).toBeVisible()
  await page.getByRole('button', { name: 'Next', exact: true }).click()
  await expect(page.getByText('Page 2 of 2', { exact: true })).toBeVisible()
  expect(requests[1].searchParams.get('search')).toBe('SYN001')
  expect(requests[1].searchParams.get('page')).toBe('2')
  expect(requests[1].searchParams.get('date_basis')).toBe('processing')
  await expect(page.getByRole('button', { name: 'Next', exact: true })).toBeDisabled()
})

test('ticket number opens details with source fare, taxes, mapping and a prefilled draft', async ({ page }) => {
  let detailsFail = true
  await page.route('**/api/v1/mir/capture?*', route => route.fulfill({ json: captureResponse([capture]) }))
  await page.route('**/api/v1/mir/capture/2/details?*', route => detailsFail ? route.fulfill({ status: 503, json: { error: 'Ticket details could not be loaded. Try again.' } }) : route.fulfill({ json: details }))
  await page.route('**/api/v1/mir/capture/2/draft?*', route => route.fulfill({ json: source }))
  await page.route('**/api/v1/masters/sales-lookups', route => route.fulfill({ json: { airlines: [], currencies: [], customers: [], suppliers: [] } }))
  await page.goto('/app/sales/workspace?mode=gds')
  await page.getByRole('button', { name: 'Search Tickets', exact: true }).click()
  await page.getByRole('button', { name: capture.ticket_number, exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('alert')).toContainText('Ticket details could not be loaded')
  detailsFail = false
  await dialog.getByRole('button', { name: 'Retry details', exact: true }).click()
  await expect(dialog.getByText(capture.passenger_name, { exact: true })).toBeVisible()
  await expect(dialog.getByText('XX: USD 20.00', { exact: true })).toBeVisible()
  await expect(dialog.getByText(details.customer_mapping, { exact: true })).toBeVisible()
  await expect(dialog.getByText(reviewProgress.issues[0], { exact: true })).toBeVisible()
  await expect(dialog.getByRole('link', { name: 'MIR Import Report', exact: true })).toHaveAttribute('href', '/app/administration/mir-report')
  await dialog.getByRole('button', { name: 'Open sales draft', exact: true }).click()
  await expect(page.getByRole('tab', { name: 'Entry', exact: true })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByLabel('Ticket number', { exact: true })).toHaveValue(capture.ticket_number)
  await expect(page.getByLabel('Client total', { exact: true })).toHaveValue('')
  await expect(page.getByRole('button', { name: 'Save draft', exact: true })).toBeDisabled()
})

test('linked-sale actions respect permissions and stale Create Sale requests cannot bypass the backend', async ({ page }) => {
  const sold = { ...capture, passenger_id: 3, ticket_number: '1231234567891', progress: { ...reviewProgress, status: 'Sale Created', linked_sale: { id: 'sale-id', number: 'SALE-1', status: 'draft' }, can_prepare_draft: false } }
  await page.route('**/api/v1/mir/capture?*', route => route.fulfill({ json: captureResponse([{ ...capture, progress: readyProgress }, sold]) }))
  await page.route('**/api/v1/mir/capture/2/draft?*', route => route.fulfill({ status: 409, json: { error: 'A sale already exists for this imported ticket. Open the linked sale instead.' } }))
  await page.route('**/api/v1/masters/sales-lookups', route => route.fulfill({ json: { airlines: [], currencies: [], customers: [], suppliers: [] } }))
  await page.goto('/app/sales/workspace?mode=gds')
  await page.getByRole('button', { name: 'Search Tickets', exact: true }).click()
  await expect(page.getByRole('radio', { name: `Select ticket ${sold.ticket_number}`, exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'View Sale', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: 'Create Sale', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('A sale already exists')
  await expect(page.getByRole('tab', { name: 'GDS Capture', exact: true })).toHaveAttribute('aria-selected', 'true')
})

test('legacy MIR links preserve the import report and extraction error review', async ({ page }) => {
  await page.route('**/api/v1/mir/records?*', route => route.fulfill({ json: { records: [], total: 0 } }))
  await page.route('**/api/v1/mir/batches?*', route => route.fulfill({ json: { batches: [] } }))
  await page.route('**/api/v1/mir/pull', route => route.fulfill({ status: 201, json: { id: 1, status: 'completed', imported_count: 1, duplicate_count: 0, failed_count: 0, created_at: '2026-11-05T10:00:00Z', errors: [] } }))
  await page.goto('/app/international/mir')
  await expect(page).toHaveURL(/\/app\/administration\/mir-report$/)
  await page.getByRole('button', { name: 'Pull from configured folder', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'Import batch 1' })).toBeVisible()
  await expect(page.getByText('No batch errors.', { exact: true })).toBeVisible()
})

test('Reset cancels a loading search and ignores its late response', async ({ page }) => {
  let release!: () => void
  let finished!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  const handled = new Promise<void>(resolve => { finished = resolve })
  await page.route('**/api/v1/mir/capture?*', async route => { await gate; await route.fulfill({ json: captureResponse([capture]) }); finished() })
  await page.goto('/app/sales/workspace?mode=gds')
  await page.getByRole('button', { name: 'Search Tickets', exact: true }).click()
  await expect(page.getByRole('status').filter({ hasText: 'Searching tickets' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Ticket results', exact: true })).toHaveAttribute('aria-busy', 'true')
  await page.getByRole('button', { name: 'Reset', exact: true }).click()
  release(); await handled
  await expect(page.locator('.capture-empty')).toHaveText('Select a date range and search for tickets.')
  await expect(page.getByRole('button', { name: capture.ticket_number, exact: true })).toHaveCount(0)
})

test('View Sale shows a saved record and handles permission revocation at request time', async ({ page }) => {
  let denied = false
  const sold = { ...capture, progress: { ...reviewProgress, status: 'Sale Created', linked_sale: { id: 'sale-id', number: 'SALE-1', status: 'draft' }, can_prepare_draft: false, can_view_sale: true } }
  await page.route('**/api/v1/mir/capture?*', route => route.fulfill({ json: captureResponse([sold]) }))
  await page.route('**/api/v1/mir/capture/2/sale?*', route => denied ? route.fulfill({ status: 403, json: { error: 'Your role does not have permission to view sales records.' } }) : route.fulfill({ json: { sale: sold.progress.linked_sale, document_date: '2026-11-06', currency: 'USD', customer: 'Synthetic customer', narration: 'Saved sale narration', ticket_number: capture.ticket_number } }))
  await page.goto('/app/sales/workspace?mode=gds')
  await page.getByRole('button', { name: 'Search Tickets', exact: true }).click()
  await page.getByRole('button', { name: 'View Sale', exact: true }).click()
  await expect(page.getByRole('dialog').getByText('Synthetic customer', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Close dialog', exact: true }).click()
  denied = true
  await page.getByRole('button', { name: 'View Sale', exact: true }).click()
  await expect(page.getByRole('dialog').getByRole('alert')).toContainText('does not have permission')
  await expect(page.getByRole('dialog').getByText('Synthetic customer', { exact: true })).toHaveCount(0)
})
