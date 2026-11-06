import { expect, test } from '@playwright/test'

const statement = { id: '1', fileName: 'bank.xlsx', kind: 'bank', rowCount: 2, uploadedBy: 'test-user', createdAt: '2026-11-06T10:00:00Z' }
const rows = [
  { number: 1, section: 'Sheet1', sourceRow: 1, cells: ['Date', 'Reference', 'Debit', 'Credit'] },
  { number: 2, section: 'Sheet1', sourceRow: 2, cells: ['06/11/2026', '0000123', '1,234.50', ''] },
]

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/auth/me', route => route.fulfill({ json: { user: { displayName: 'Test user', username: 'test' } } }))
  await page.route('**/api/v1/status', route => route.fulfill({ json: { database: 'connected', schema_ready: true } }))
})

test('sidebar opens statement upload and saved extracted values remain available after reload', async ({ page, isMobile }) => {
  let imported = false
  await page.route('**/api/v1/reconciliation/statements**', async route => {
    const request = route.request()
    if (request.method() === 'POST') {
      expect(request.headers()['content-type']).toContain('multipart/form-data')
      imported = true
      await route.fulfill({ status: 201, json: { statement, duplicate: false } })
    } else if (new URL(request.url()).pathname.endsWith('/1')) {
      await route.fulfill({ json: { statement, rows, offset: 0 } })
    } else await route.fulfill({ json: { statements: imported ? [statement] : [] } })
  })
  await page.goto('/app/dashboard')
  if (isMobile) await page.getByRole('button', { name: 'Open navigation', exact: true }).click()
  const sidebar = page.getByRole('complementary', { name: 'Main navigation' })
  for (const name of ['International', 'Domestic', 'Miscellaneous']) await expect(sidebar.getByRole('button', { name, exact: true })).toHaveCount(0)
  await sidebar.getByRole('link', { name: 'Upload for reconciliation', exact: true }).click()
  await expect(page.getByRole('heading', { level: 1 })).toContainText('Upload statements')
  await expect(page.getByText('No statements uploaded yet.')).toBeVisible()
  const upload = page.getByRole('button', { name: 'Upload, extract & save', exact: true })
  await expect(upload).toBeDisabled()
  await page.getByLabel('PDF or Excel statement').setInputFiles({ name: 'bank.xlsx', mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', buffer: Buffer.from('synthetic-workbook') })
  await upload.click()
  await expect(page.getByRole('status').filter({ hasText: '2 rows extracted and saved' })).toBeVisible()
  await expect(page.getByRole('cell', { name: 'Sheet1', exact: true })).toHaveCount(2)
  await expect(page.getByText('0000123', { exact: true })).toBeVisible()
  await expect(page.getByText('1,234.50', { exact: true })).toBeVisible()
  await page.reload()
  await page.getByRole('button', { name: 'bank.xlsx', exact: true }).click()
  await expect(page.getByText('0000123', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})

test('failed upload keeps the chosen file for retry and duplicate response opens existing rows', async ({ page }) => {
  let fail = true
  await page.route('**/api/v1/reconciliation/statements**', async route => {
    if (route.request().method() === 'POST') {
      await route.fulfill(fail ? { status: 422, json: { error: 'No readable statement rows found.' } } : { json: { statement, duplicate: true } })
    } else if (new URL(route.request().url()).pathname.endsWith('/1')) await route.fulfill({ json: { statement, rows, offset: 0 } })
    else await route.fulfill({ json: { statements: [statement] } })
  })
  await page.goto('/app/reconciliation/upload')
  await page.getByLabel('Statement type').selectOption('supplier')
  const input = page.getByLabel('PDF or Excel statement')
  await input.setInputFiles({ name: 'supplier.pdf', mimeType: 'application/pdf', buffer: Buffer.from('synthetic-pdf') })
  await page.getByRole('button', { name: 'Upload, extract & save', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('No readable statement rows')
  await expect(input).not.toHaveValue('')
  fail = false
  await page.getByRole('button', { name: 'Upload, extract & save', exact: true }).click()
  await expect(page.getByRole('status').filter({ hasText: 'already imported' })).toBeVisible()
  await expect(page.getByText('0000123', { exact: true })).toBeVisible()
})

test('unsupported files are rejected before making an upload request', async ({ page }) => {
  let uploads = 0
  await page.route('**/api/v1/reconciliation/statements', route => { if (route.request().method() === 'POST') uploads++; return route.fulfill({ json: { statements: [] } }) })
  await page.goto('/app/reconciliation/upload')
  await page.getByLabel('PDF or Excel statement').setInputFiles({ name: 'statement.xls', mimeType: 'application/vnd.ms-excel', buffer: Buffer.from('old-workbook') })
  await expect(page.getByRole('alert')).toContainText('Save older .xls workbooks as .xlsx')
  await expect(page.getByRole('button', { name: 'Upload, extract & save', exact: true })).toBeDisabled()
  expect(uploads).toBe(0)
})
