import { expect, test } from '@playwright/test'
test('sidebar starts with Overview and module labels stand out from processes', async ({ page, isMobile }) => {
  await page.goto('/app/dashboard')
  if (isMobile) await page.getByRole('button', { name: 'Open navigation' }).click()
  const sidebar = page.getByRole('complementary', { name: 'Main navigation' })
  await expect(sidebar.getByText('Company setup pending')).toHaveCount(0)
  await expect(sidebar.getByText('Workspace guide', { exact: true })).toHaveCount(0)
  await expect(sidebar.getByText('WORKSPACE', { exact: true })).toHaveCount(0)
  await expect(sidebar.locator('nav > a').first()).toHaveText('Overview')
  await expect(sidebar.getByRole('button', { name: 'International', exact: true })).toHaveCount(0)
  await expect(sidebar.getByRole('button', { name: 'Domestic', exact: true })).toHaveCount(0)
  await expect(sidebar.getByRole('link', { name: 'Sales Workspace', exact: true })).toBeVisible()
  const module = sidebar.getByRole('button', { name: 'Admin Control', exact: true })
  expect(Number(await module.evaluate(element => getComputedStyle(element).fontWeight))).toBeGreaterThanOrEqual(500)
  await expect(sidebar.getByRole('link', { name: 'Overview', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(sidebar.locator('#nav-international').getByRole('link', { name: /MIR/ })).toHaveCount(0)
  await sidebar.getByRole('button', { name: 'Admin Control', exact: true }).click()
  await expect(sidebar.getByRole('link', { name: 'MIR Report', exact: true })).toBeVisible()
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
