import { expect, test } from '@playwright/test'
import { allPages } from '../src/lib/navigation'

test('login validates locally and sends credentials only to the sign-in endpoint', async ({ page }) => {
  let submitted: unknown
  await page.route('**/api/v1/auth/login', async route => {
    submitted = route.request().postDataJSON()
    await route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ error: 'invalid_credentials' }) })
  })
  await page.goto('/login')
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByText('Enter your username.', { exact: true })).toBeVisible()
  await expect(page.getByText('Enter your password.', { exact: true })).toBeVisible()
  expect(submitted).toBeUndefined()
  await page.getByLabel('Username or email', { exact: true }).fill('preview-user')
  const password = page.getByLabel('Password', { exact: true })
  await password.fill('not-a-real-password')
  await page.getByRole('button', { name: 'Show password' }).click()
  await expect(password).toHaveAttribute('type', 'text')
  await page.getByRole('button', { name: 'Hide password' }).click()
  await expect(password).toHaveAttribute('type', 'password')
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Username or password is incorrect.')
  await expect(password).toHaveValue('')
  expect(submitted).toEqual({ username: 'preview-user', password: 'not-a-real-password' })
})

test('forced password change reuses the authenticated password and accepts six characters', async ({ page }) => {
  let changed: unknown
  await page.route('**/api/v1/auth/login', route => route.fulfill({
    json: { user: { passwordChangeRequired: true } },
  }))
  await page.route('**/api/v1/auth/password', async route => {
    changed = route.request().postDataJSON()
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ status: 'password_updated' }) })
  })
  await page.goto('/login')
  await page.getByLabel('Username or email').fill('new.user')
  await page.getByLabel('Password', { exact: true }).fill('temporary123')
  const loginResponse = page.waitForResponse(response => response.url().includes('/api/v1/auth/login'))
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await loginResponse
  await expect(page.getByRole('heading', { name: 'Update your password' })).toBeVisible()
  await expect(page.getByLabel('Current password')).toHaveValue('temporary123')
  await page.getByLabel('New password', { exact: true }).fill('12345')
  await page.getByLabel('Confirm new password', { exact: true }).fill('12345')
  await page.getByRole('button', { name: 'Update password' }).click()
  await expect(page.getByText('Use at least 6 characters.')).toBeVisible()
  expect(changed).toBeUndefined()
  await page.getByLabel('New password', { exact: true }).fill('six789')
  await page.getByLabel('Confirm new password', { exact: true }).fill('six789')
  await page.getByRole('button', { name: 'Update password' }).click()
  await expect(page).toHaveURL(/\/app\/dashboard$/)
  expect(changed).toEqual({ currentPassword: 'temporary123', newPassword: 'six789' })
})

test('keyboard search opens manual entry without enabling financial writes', async ({ page }) => {
  await page.goto('/app/dashboard')
  await page.keyboard.press('Control+k')
  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  await dialog.getByRole('combobox').fill('International Sales entry')
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/international\/sales/)
  await expect(page.getByRole('heading', { level: 1 })).toContainText('Sales entry')
  await expect(page.getByRole('button', { name: 'Post', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'New document' })).toBeDisabled()
})

test('every FRD destination has an explicit connected or deferred screen', async ({ page }) => {
  test.setTimeout(120_000)
  for (const destination of allPages) {
    await page.goto(`/app/${destination.id}`)
    await expect(page.locator('main h1')).toContainText(destination.label)
    await expect(page.getByText('Phase 2 foundation.', { exact: false })).toBeVisible()
  }
})

test('theme and saved filters survive reload without storing business data', async ({ page }) => {
  await page.goto('/app/international/sales')
  await page.getByRole('button', { name: 'Switch to dark theme' }).click()
  await page.getByLabel('Status filter').selectOption('Draft')
  await page.getByRole('button', { name: 'Saved views' }).click()
  await page.getByLabel('View name').fill('My drafts')
  await page.getByRole('button', { name: 'Save view', exact: true }).click()
  await expect(page.getByRole('dialog').getByRole('status')).toContainText('View saved')
  await page.getByRole('button', { name: 'Close dialog' }).click()
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.getByRole('button', { name: 'Saved views' }).click()
  await page.getByRole('dialog').getByRole('button').filter({ hasText: 'My drafts' }).click()
  await expect(page.getByLabel('Status filter')).toHaveValue('Draft')
})

test('loading and recoverable error states are accessible', async ({ page }) => {
  await page.goto('/app/masters/customers?state=loading')
  await expect(page.getByRole('status', { name: 'Loading screen' })).toBeVisible()
  await page.goto('/app/masters/customers?state=error')
  await expect(page.getByRole('alert')).toContainText('This screen could not load')
  await page.getByRole('button', { name: 'Try again' }).click()
  await expect(page.getByText('Your records will appear here')).toBeVisible()
})

test('mobile navigation opens and Escape returns focus', async ({ page, isMobile }) => {
  test.skip(!isMobile, 'Mobile navigation behavior')
  await page.goto('/app/dashboard')
  await page.getByRole('button', { name: 'Open navigation', exact: true }).click()
  await expect(page.getByRole('complementary', { name: 'Main navigation' })).toBeInViewport()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: 'Open navigation', exact: true })).toBeFocused()
})
