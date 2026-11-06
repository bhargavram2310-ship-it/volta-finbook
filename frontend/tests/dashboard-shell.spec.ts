import { expect, test } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/auth/me', route => route.fulfill({ status: 401, json: { error: 'unauthorized' } }))
  await page.route('**/api/v1/status', route => route.fulfill({ json: { database: 'connected_schema_pending', schema_ready: false } }))
})

test('dashboard preserves financial destinations without inventing balances', async ({ page }) => {
  await page.goto('/app/dashboard')
  await expect(page.locator('.financial-card')).toHaveCount(8)
  await expect(page.locator('.financial-card-value').filter({ hasText: 'Unavailable' })).toHaveCount(8)
  await expect(page.locator('.financial-availability')).toContainText('A dash does not represent zero')
  await expect(page.locator('.preview-banner')).toHaveCount(0)
  await page.getByRole('link', { name: /Month-to-date sales/ }).click()
  await expect(page).toHaveURL(/reports\/sales/)
})

test('user-menu help remains reachable without the workspace dropdown', async ({ page }) => {
  await page.goto('/app/dashboard')
  await expect(page.locator('.workspace-context')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Search workspace', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'User menu', exact: true }).click()
  await page.getByRole('menuitem', { name: 'Workspace guide', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'Workspace guide' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
})

test('dashboard reflows without horizontal overflow in both themes', async ({ page }) => {
  await page.goto('/app/dashboard')
  for (const width of [375, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 900 })
    for (const theme of ['light', 'dark']) {
      await page.evaluate(value => { document.documentElement.dataset.theme = value }, theme)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
      await expect(page.locator('main').getByRole('link', { name: 'Sales Workspace', exact: true })).toBeVisible()
    }
  }
  await page.emulateMedia({ reducedMotion: 'reduce' })
  expect(await page.locator('.financial-card').first().evaluate(element => getComputedStyle(element).transitionDuration)).toBe('0s')
})

test('navigation remembers expanded groups and identifies the active destination', async ({ page, isMobile }) => {
  await page.goto('/app/dashboard')
  if (isMobile) await page.getByRole('button', { name: 'Open navigation', exact: true }).click()
  const nav = page.getByRole('complementary', { name: 'Main navigation' })
  await nav.getByRole('button', { name: 'Reports', exact: true }).click()
  await nav.getByRole('link', { name: 'Sales reports', exact: true }).click()
  await expect(page).toHaveURL(/reports\/sales/)
  await page.reload()
  if (isMobile) await page.getByRole('button', { name: 'Open navigation', exact: true }).click()
  await expect(nav.getByRole('button', { name: 'Reports', exact: true })).toHaveAttribute('aria-expanded', 'true')
  await expect(nav.getByRole('link', { name: 'Sales reports', exact: true })).toHaveAttribute('aria-current', 'page')
})

test('a collapsed desktop preference does not hide mobile navigation links', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('volta.sidebar', 'collapsed'))
  await page.setViewportSize({ width: 375, height: 812 })
  await page.goto('/app/dashboard')
  await page.getByRole('button', { name: 'Open navigation', exact: true }).click()
  await page.getByRole('complementary').getByRole('button', { name: 'Admin Control', exact: true }).click()
  await expect(page.getByRole('complementary').getByRole('link', { name: 'MIR Report', exact: true })).toBeVisible()
})
