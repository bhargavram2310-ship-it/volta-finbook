import { expect, test } from '@playwright/test'

const employee = {
  id: 'a7e658fb-b3d2-48f4-a3ef-1fe09af17010',
  employeeCode: 'EMP-TEST-01',
  name: 'Synthetic Employee',
  officialEmail: 'employee@example.invalid',
  mobileNumber: '9000000000',
  designation: 'Finance Associate',
  department: 'Finance',
  employmentStatus: 'Active',
  dateOfJoining: '2025-01-15',
  dateOfLeaving: '',
  version: 1,
}

test('employee administrator can create and edit records', async ({ page }) => {
  let savedEmployee = { ...employee }
  const writes: Array<{ method: string; body: Record<string, unknown> }> = []
  await page.route('**/api/v1/employees/me', route => route.fulfill({ json: { employee } }))
  await page.route('**/api/v1/employees', async route => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON() as Record<string, unknown>
      writes.push({ method: 'POST', body })
      savedEmployee = { ...employee, ...body, id: 'b7e658fb-b3d2-48f4-a3ef-1fe09af17010', version: 1 } as typeof employee
      await route.fulfill({ status: 201, json: { employee: savedEmployee } })
      return
    }
    await route.fulfill({ json: { employees: [savedEmployee], capabilities: { create: true, update: true } } })
  })
  await page.route(/\/api\/v1\/employees\/[a-f0-9-]+$/, async route => {
    const body = route.request().postDataJSON() as Record<string, unknown>
    writes.push({ method: route.request().method(), body })
    savedEmployee = { ...savedEmployee, ...body, version: Number(body.version) + 1 } as typeof employee
    await route.fulfill({ json: { employee: savedEmployee } })
  })
  await page.goto('/app/administration/employees')
  await expect(page.getByRole('heading', { name: 'Employees.' })).toBeVisible()
  await expect(page.getByText('Synthetic Employee').first()).toBeVisible()
  await page.getByRole('button', { name: 'Add employee' }).click()
  await page.getByLabel('Employee code').fill('EMP-NEW-02')
  await page.getByLabel('Full name').fill('Synthetic New Hire')
  await page.getByLabel('Official email').fill('new-hire@example.invalid')
  await page.getByRole('button', { name: 'Create employee' }).click()
  await expect(page.getByText('Employee created.', { exact: true })).toBeVisible()
  expect(writes[0]).toMatchObject({ method: 'POST', body: { employeeCode: 'EMP-NEW-02', name: 'Synthetic New Hire' } })

  await page.getByRole('button', { name: 'Edit Synthetic New Hire' }).click()
  await page.getByLabel('Department').fill('Operations')
  await page.getByRole('button', { name: 'Save changes' }).click()
  await expect(page.getByText('Employee details updated.', { exact: true })).toBeVisible()
  expect(writes[1]).toMatchObject({ method: 'PATCH', body: { department: 'Operations', version: 1 } })
})

test('a signed-in employee can create employee records without a permission assignment', async ({ page }) => {
  await page.route('**/api/v1/employees/me', route => route.fulfill({ json: { employee } }))
  await page.route('**/api/v1/employees', route => route.fulfill({ json: { employees: [employee], capabilities: { create: true, update: true } } }))
  await page.goto('/app/administration/employees')
  await expect(page.getByRole('heading', { name: 'My employee information' })).toBeVisible()
  await expect(page.getByText('Synthetic Employee').first()).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Employee directory' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Add employee' })).toBeVisible()
})

test('employee page reports a missing session instead of inventing a profile', async ({ page }) => {
  await page.route('**/api/v1/employees/me', route => route.fulfill({ status: 401, json: { error: 'authentication_required' } }))
  await page.route('**/api/v1/employees', route => route.fulfill({ status: 401, json: { error: 'authentication_required' } }))
  await page.goto('/app/administration/employees')
  await expect(page.getByRole('heading', { name: 'Sign in to view employee information' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Go to sign in' })).toHaveAttribute('href', '/login')
})
