import { expect, test } from '@playwright/test'

const masters = {
  airlines: [{ id: 'airline-id', code: 'XX', name: 'Test Airline' }],
  currencies: [{ id: 'INR', code: 'INR', name: 'Indian rupee' }, { id: 'USD', code: 'USD', name: 'US dollar' }],
  customers: [{ id: 'customer-id', code: 'C001', name: 'Test Customer' }],
  suppliers: [{ id: 'supplier-id', code: 'S001', name: 'Test Supplier' }],
}

test('sales uses master selections and retains distinct relationship IDs', async ({ page }) => {
  await page.route('**/api/v1/masters/sales-lookups', route => route.fulfill({ json: masters }))
  await page.goto('/app/international/sales?mode=manual')
  const fields = [
    ['Airline', 'airline-id', 'XX — Test Airline'],
    ['Basic fare currency', 'USD', 'USD — US dollar'],
    ['Ticket fare currency', 'INR', 'INR — Indian rupee'],
    ['Bill to', 'customer-id', 'C001 — Test Customer'],
    ['Supplier', 'supplier-id', 'S001 — Test Supplier'],
  ]
  for (const [label, id, text] of fields) {
    const field = page.getByRole('combobox', { name: label, exact: true })
    await expect(field.getByRole('option', { name: text, exact: true })).toHaveCount(1)
    await field.selectOption(id)
    await expect(field).toHaveValue(id)
  }
  await page.getByRole('tab', { name: 'GDS Capture', exact: true }).click()
  await page.getByRole('tab', { name: 'Entry', exact: true }).click()
  await expect(page.getByRole('combobox', { name: 'Bill to', exact: true })).toHaveValue('customer-id')
})

test('failed master load disables selection and can be retried', async ({ page }) => {
  let failed = true
  await page.route('**/api/v1/masters/sales-lookups', route => failed
    ? route.fulfill({ status: 503, json: { error: 'unavailable' } })
    : route.fulfill({ json: masters }))
  await page.goto('/app/international/sales?mode=manual')
  await expect(page.getByRole('alert')).toContainText('Master lists could not be loaded')
  await expect(page.getByRole('combobox', { name: 'Airline', exact: true })).toBeDisabled()
  failed = false
  await page.getByRole('button', { name: 'Retry master lists' }).click()
  await expect(page.getByRole('combobox', { name: 'Airline', exact: true })).toBeEnabled()
  await expect(page.getByRole('alert')).toHaveCount(0)
})

test('empty masters show empty lists without invented options', async ({ page }) => {
  await page.route('**/api/v1/masters/sales-lookups', route => route.fulfill({ json: { airlines: [], currencies: [], customers: [], suppliers: [] } }))
  await page.goto('/app/international/sales?mode=manual')
  const airline = page.getByRole('combobox', { name: 'Airline', exact: true })
  await expect(airline.getByRole('option')).toHaveText(['No active airline records'])
})
