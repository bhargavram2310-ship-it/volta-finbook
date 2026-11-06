import { expect, test } from '@playwright/test'

const fields = [
  { name: 'code', label: 'Code', type: 'text', required: true, max: 32 },
  { name: 'name', label: 'Name', type: 'text', required: true, max: 191 },
  { name: 'active', label: 'Active', type: 'boolean', required: true, default: 'true' },
  { name: 'ticket_prefix', label: 'Ticket prefix', type: 'text', required: false },
]

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/status', route => route.fulfill({ json: { database: 'connected', schema_ready: true } }))
  await page.route('**/api/v1/auth/me', route => route.fulfill({ status: 401, json: { error: 'authentication_required' } }))
})

test('master records can be created, edited, searched and deleted after confirmation', async ({ page }) => {
  let records: Array<{ id: string; revision: string; values: Record<string, string> }> = []
  const writes: string[] = []
  await page.route('**/api/v1/masters/airlines**', async route => {
    const method = route.request().method()
    if (method === 'GET') return route.fulfill({ json: { title: 'Airlines', fields, records } })
    writes.push(method)
    if (method === 'DELETE') { records = []; return route.fulfill({ json: { record: null } }) }
    const input = route.request().postDataJSON()
    const record = { id: 'airline-id', revision: method === 'POST' ? 'rev-1' : 'rev-2', values: input.values }
    if (method === 'PATCH') expect(input.revision).toBe('rev-1')
    records = [record]
    await route.fulfill({ status: method === 'POST' ? 201 : 200, json: { record } })
  })
  await page.goto('/app/masters/travel')
  await expect(page.getByRole('button', { name: 'Create record', exact: true })).toBeEnabled()
  await page.getByRole('button', { name: 'Create record', exact: true }).click()
  let dialog = page.getByRole('dialog')
  await dialog.getByLabel('Code').fill('XX')
  await dialog.getByLabel('Name').fill('Test airline')
  await dialog.getByLabel('Ticket prefix').fill('999')
  await dialog.getByRole('button', { name: 'Create record', exact: true }).click()
  await expect(page.getByRole('cell', { name: 'Test airline', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Edit Test airline', exact: true }).click()
  dialog = page.getByRole('dialog')
  await dialog.getByLabel('Name').fill('Updated airline')
  await dialog.getByRole('button', { name: 'Save changes' }).click()
  await expect(page.getByRole('cell', { name: 'Updated airline', exact: true })).toBeVisible()
  await page.getByRole('searchbox', { name: 'Search master records' }).fill('Missing')
  await expect(page.getByText('No matching records', { exact: true })).toBeVisible()
  await page.getByRole('searchbox', { name: 'Search master records' }).fill('')
  await page.getByRole('button', { name: 'Delete Updated airline', exact: true }).click()
  expect(writes).toEqual(['POST', 'PATCH'])
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click()
  expect(writes).toEqual(['POST', 'PATCH'])
  await page.getByRole('button', { name: 'Delete Updated airline', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Delete record', exact: true }).click()
  await expect(page.getByText('No records yet', { exact: true })).toBeVisible()
  expect(writes).toEqual(['POST', 'PATCH', 'DELETE'])
  await expect(page.locator('main')).not.toContainText(/phase/i)
})

test('blocked delete leaves the record and explains the dependency', async ({ page }) => {
  await page.route('**/api/v1/masters/airlines**', route => route.request().method() === 'DELETE'
    ? route.fulfill({ status: 409, json: { error: 'This record is in use and cannot be deleted.' } })
    : route.fulfill({ json: { title: 'Airlines', fields, records: [{ id: 'id', revision: 'rev', values: { code: 'XX', name: 'Used airline', active: 'true', ticket_prefix: '' } }] } }))
  await page.goto('/app/masters/travel')
  await page.getByRole('button', { name: 'Delete Used airline', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Delete record', exact: true }).click()
  await expect(page.getByRole('dialog').getByRole('alert')).toContainText('in use')
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click()
  await expect(page.getByRole('cell', { name: 'Used airline', exact: true })).toBeVisible()
})

test('company form uses currencies from the related master', async ({ page }) => {
  await page.route('**/api/v1/masters/company', route => route.fulfill({ json: {
    title: 'Company', records: [], fields: [
      { name: 'legal_name', label: 'Legal name', type: 'text', required: true },
      { name: 'base_currency', label: 'Base currency', type: 'reference', ref: 'currencies', required: true },
    ],
  } }))
  await page.route('**/api/v1/masters/currencies', route => route.fulfill({ json: {
    title: 'Currencies', fields: [], records: [{ id: 'INR', revision: 'rev', values: { code: 'INR', name: 'Indian rupee', active: 'true' } }],
  } }))
  await page.goto('/app/masters/company')
  await page.getByRole('button', { name: 'Create record', exact: true }).click()
  await page.getByLabel('Base currency').selectOption('INR')
  await expect(page.getByLabel('Base currency')).toHaveValue('INR')
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click()
  await expect(page.getByRole('navigation', { name: 'Master record types' }).getByRole('button', { name: 'Branches', exact: true })).toBeVisible()
})
