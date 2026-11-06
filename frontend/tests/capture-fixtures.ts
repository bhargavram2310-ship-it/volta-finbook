import type { Page } from '@playwright/test'

// Synthetic HTTP fixtures only. Real source, status, permission and uniqueness
// behavior is exercised by the isolated MySQL MIR integration suite.
export const captureSettings = { timezone: 'UTC', today: '2026-11-06', sales_save_enabled: false, workflow_note: 'Review the draft. Saving, approval and posting are not connected yet.' }
export const reviewProgress = { status: 'Needs Review', issues: ['Review the source validation issues.'], linked_sale: null, can_prepare_draft: true, can_view_sale: false }
export const readyProgress = { ...reviewProgress, status: 'Ready for Sale', issues: [] }
export function captureResponse<T extends object>(records: T[], total = records.length, timezone = 'UTC') {
  return { records: records.map(record => ({ imported_at: '2026-11-05T20:00:00Z', progress: reviewProgress, ...record })), total, timezone }
}
export async function mockCaptureSettings(page: Page, settings = captureSettings) {
  await page.route('**/api/v1/mir/capture/settings', route => route.fulfill({ json: settings }))
}
