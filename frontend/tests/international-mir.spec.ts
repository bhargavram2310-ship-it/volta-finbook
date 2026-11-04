import { expect, test } from '@playwright/test'
import type { MirRecord } from '../src/lib/mir'

const fixture: MirRecord = {
  id: `${'a'.repeat(64)}:r1:p1`, fileName: 'synthetic-test.MIR', sha256: 'a'.repeat(64),
  recordDate: '2026-10-29', status: 'Extracted', issues: [],
  fields: {
    ticketNumber: { value: '1234567890123', lines: [1], fieldIds: ['fixture-ticket'] },
    passengerName: { value: 'SYNTHETIC/PASSENGER', lines: [1], fieldIds: ['fixture-pax'] },
    pnr: { value: 'TESTPN', lines: [1], fieldIds: ['fixture-pnr'] },
    basicFare: { value: '100.25', lines: [2], fieldIds: ['fixture-base'] },
    basicCurrency: { value: 'INR', lines: [2], fieldIds: ['fixture-currency'] },
    mirTotalFare: { value: '120.25', lines: [2], fieldIds: ['fixture-total'] },
  },
  rawLines: [{ number: 1, text: 'SYNTHETIC SOURCE ONLY', rawHex: '53594e544845544943', terminatorHex: '0d0a' }],
}
async function lockedSource(page: import('@playwright/test').Page) {
  await page.route('**/api/v1/mir/source', route => route.fulfill({ json: { path: 'D:\\SyntheticMIR', locked: true } }))
  await page.route('**/api/v1/mir/records?*', route => route.fulfill({ json: { records: [fixture], issues: [], matchedFiles: 1, dateBasis: 'File modified date' } }))
  await page.route('**/api/v1/mir/record?*', route => route.fulfill({ json: fixture }))
}
async function fetchOne(page: import('@playwright/test').Page) {
  await page.getByLabel('From date', { exact: true }).fill('2026-10-01')
  await page.getByLabel('To date', { exact: true }).fill('2026-10-31')
  await page.getByRole('button', { name: 'Fetch MIR files', exact: true }).click()
  await page.getByRole('radio').check()
}

test('sidebar starts with Overview and module labels stand out from processes', async ({ page, isMobile }) => {
  await page.goto('/app/dashboard')
  if (isMobile) await page.getByRole('button', { name: 'Open navigation' }).click()
  const sidebar = page.getByRole('complementary', { name: 'Main navigation' })
  await expect(sidebar.getByText('Company setup pending')).toHaveCount(0)
  await expect(sidebar.getByText('Workspace guide', { exact: true })).toHaveCount(0)
  await expect(sidebar.getByText('WORKSPACE', { exact: true })).toHaveCount(0)
  await expect(sidebar.locator('nav > a').first()).toHaveText('Overview')
  const module = sidebar.getByRole('button', { name: 'International', exact: true })
  expect(Number(await module.evaluate(element => getComputedStyle(element).fontWeight))).toBeGreaterThanOrEqual(600)
  await expect(sidebar.getByRole('link', { name: 'MIR import', exact: true })).toBeVisible()
})

test('source folder can be entered once and stays locked after reload', async ({ page }) => {
  let locked = false
  let path = ''
  await page.route('**/api/v1/mir/source', async route => {
    if (route.request().method() === 'POST') { path = route.request().postDataJSON().path; locked = true }
    await route.fulfill({ json: { path, locked } })
  })
  await page.goto('/app/administration/mir-source')
  await page.getByLabel('Local MIR folder path').fill('D:\SyntheticMIR')
  await page.getByRole('button', { name: 'Save & lock folder', exact: true }).click()
  await page.getByRole('button', { name: 'Confirm & lock folder', exact: true }).click()
  await expect(page.getByLabel('Locked folder')).toHaveValue('D:\SyntheticMIR')
  await page.reload()
  await expect(page.getByLabel('Locked folder')).toHaveAttribute('readonly', '')
  await expect(page.getByRole('button', { name: 'Save & lock folder' })).toHaveCount(0)
})

test('date selection fetches records and Proceed populates sales without inferred values', async ({ page }) => {
  await lockedSource(page)
  const writes: string[] = []
  const dates: string[] = []
  page.on('request', request => { if (request.method() === 'POST') writes.push(request.url()); if (request.url().includes('/mir/records?')) dates.push(request.url()) })
  await page.goto('/app/international/mir')
  await fetchOne(page)
  expect(dates[0]).toContain('from=2026-10-01&to=2026-10-31')
  await page.getByRole('button', { name: 'Proceed to sales entry' }).click()
  await expect(page).toHaveURL(/international\/sales\?mode=manual/)
  await expect(page.getByLabel('Ticket number', { exact: true })).toHaveValue('1234567890123')
  await expect(page.getByLabel('Passenger name', { exact: true })).toHaveValue('SYNTHETIC/PASSENGER')
  await expect(page.getByLabel('Basic fare', { exact: true })).toHaveValue('100.25')
  await expect(page.getByLabel('Client total', { exact: true })).toHaveValue('')
  await expect(page.getByLabel('Supplier total', { exact: true })).toHaveValue('')
  await page.getByRole('button', { name: 'View source & issues' }).click()
  await expect(page.getByRole('dialog').getByText('1: SYNTHETIC SOURCE ONLY')).toBeVisible()
  await page.getByRole('button', { name: 'Close dialog' }).click()
  expect(writes).toEqual([])
  await expect(page.getByRole('button', { name: 'Post', exact: true })).toBeDisabled()
})

test('corrections retain old/new history and repeated Proceed preserves the loaded ticket', async ({ page }) => {
  await lockedSource(page)
  await page.goto('/app/international/sales?mode=mir')
  await fetchOne(page)
  await page.getByRole('button', { name: 'Proceed to sales entry' }).click()
  await page.getByRole('button', { name: 'Correct Passenger name', exact: true }).click()
  await page.getByLabel('New value', { exact: true }).fill('SYNTHETIC/CORRECTED')
  await page.getByLabel('Reason for correction').fill('Synthetic test correction')
  await page.getByRole('button', { name: 'Apply correction' }).click()
  await page.getByRole('button', { name: 'View Passenger name history', exact: true }).click()
  const history = page.getByRole('dialog')
  await expect(history.getByText('SYNTHETIC/PASSENGER', { exact: true })).toBeVisible()
  await expect(history.getByText('SYNTHETIC/CORRECTED', { exact: true })).toBeVisible()
  await expect(history.getByText('Synthetic test correction', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Close dialog' }).click()
  await page.getByRole('tab', { name: 'MIR import', exact: true }).click()
  await fetchOne(page)
  await page.getByRole('button', { name: 'Proceed to sales entry' }).click()
  await expect(page.getByText('This ticket is already open in sales.', { exact: false })).toBeVisible()
  await page.getByRole('tab', { name: 'Manual entry', exact: true }).click()
  await expect(page.getByLabel('Passenger name', { exact: true })).toHaveValue('SYNTHETIC/CORRECTED')
  await expect(page.getByText('1 loaded ticket', { exact: true })).toBeVisible()
})

test('date edits invalidate the selection and changed source blocks Proceed', async ({ page }) => {
  await lockedSource(page)
  await page.route('**/api/v1/mir/record?*', route => route.fulfill({ status: 409, json: { error: 'MIR file changed since fetch; fetch the records again' } }))
  await page.goto('/app/international/sales?mode=mir')
  await fetchOne(page)
  await page.getByLabel('To date', { exact: true }).fill('2026-11-01')
  await expect(page.getByRole('button', { name: 'Proceed to sales entry' })).toBeDisabled()
  await page.getByRole('button', { name: 'Fetch MIR files', exact: true }).click()
  await page.getByRole('radio').check()
  await page.getByRole('button', { name: 'Proceed to sales entry' }).click()
  await expect(page.getByRole('alert')).toContainText('MIR file changed since fetch')
  await expect(page.getByRole('tab', { name: 'MIR import', exact: true })).toHaveAttribute('aria-selected', 'true')
})

test('MIR import delegates folder setup to Admin Control', async ({ page }) => {
  await page.route('**/api/v1/mir/source', route => route.fulfill({ json: { path: '', locked: false } }))
  await page.goto('/app/international/sales?mode=mir')
  await expect(page.getByLabel('Local MIR folder path')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Fetch MIR files', exact: true })).toBeDisabled()
  await page.getByRole('link', { name: 'Admin Control: MIR File Reader Location' }).click()
  await expect(page).toHaveURL(/administration\/mir-source/)
  await expect(page.getByRole('heading', { level: 1 })).toContainText('MIR File Reader Location')
  await expect(page.getByLabel('Local MIR folder path')).toBeVisible()
})

test('Admin Control retains setup input after a rejected folder lock', async ({ page }) => {
  await page.route('**/api/v1/mir/source', route => route.request().method() === 'POST'
    ? route.fulfill({ status: 400, json: { error: 'Folder does not exist' } })
    : route.fulfill({ json: { path: '', locked: false } }))
  await page.goto('/app/administration/mir-source')
  await page.getByLabel('Local MIR folder path').fill('D:\\MissingMIR')
  await page.getByRole('button', { name: 'Save & lock folder', exact: true }).click()
  await page.getByRole('button', { name: 'Confirm & lock folder', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Folder does not exist')
  await expect(page.getByLabel('Local MIR folder path')).toHaveValue('D:\\MissingMIR')
  await expect(page.getByRole('button', { name: 'Save & lock folder', exact: true })).toBeEnabled()
})
