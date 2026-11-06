import { expect, test } from '@playwright/test'

test('signed-out master screen offers sign-in and returns to the selected master tab', async ({ page }) => {
  let signedIn = false
  await page.route('**/api/v1/auth/me', route => route.fulfill(signedIn
    ? { json: { user: { username: 'test', displayName: 'Test user' } } }
    : { status: 401, json: { error: 'authentication_required' } }))
  await page.route('**/api/v1/auth/login', route => {
    signedIn = true
    return route.fulfill({ json: { user: { passwordChangeRequired: false } } })
  })
  await page.route('**/api/v1/masters/locations', route => route.fulfill(signedIn
    ? { json: { title: 'Airports & geography', fields: [], records: [] } }
    : { status: 401, json: { error: 'authentication_required' } }))
  await page.goto('/app/masters/travel?tab=locations')
  await expect(page.getByRole('alert')).toContainText('Sign in to manage master records')
  await expect(page.getByRole('button', { name: 'Create record' })).toBeDisabled()
  await page.getByRole('alert').getByRole('link', { name: 'Sign in', exact: true }).click()
  await page.getByLabel('Username or email').fill('test')
  await page.getByLabel('Password', { exact: true }).fill('test-password')
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page).toHaveURL(/\/app\/masters\/travel\?tab=locations$/)
  await expect(page.getByRole('button', { name: 'Create record' })).toBeEnabled()
})

test('sign-in refuses an external return destination', async ({ page }) => {
  await page.route('**/api/v1/auth/login', route => route.fulfill({ json: { user: { passwordChangeRequired: false } } }))
  await page.goto('/login?returnTo=https%3A%2F%2Fother.example%2Fapp%2Fmasters')
  await page.getByLabel('Username or email').fill('test')
  await page.getByLabel('Password', { exact: true }).fill('test-password')
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page).toHaveURL(/\/app\/dashboard$/)
})
