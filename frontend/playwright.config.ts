import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  globalTimeout: 300_000,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: 'list',
  use: {
    baseURL: 'http://127.0.0.1:5173', trace: 'retain-on-failure',
    ...(process.env.PLAYWRIGHT_CHANNEL === 'msedge' ? { channel: 'msedge' } : {}),
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile', use: { ...devices['Pixel 7'] } },
  ],
  webServer: {
    command: 'npm run dev', url: 'http://127.0.0.1:5173', reuseExistingServer: !process.env.CI,
  },
})
