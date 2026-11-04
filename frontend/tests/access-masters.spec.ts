import { expect, test } from '@playwright/test'

const employeeId = 'a7e658fb-b3d2-48f4-a3ef-1fe09af17010'
const userId = 'b7e658fb-b3d2-48f4-a3ef-1fe09af17010'
const actorId = 'd7e658fb-b3d2-48f4-a3ef-1fe09af17010'
const roleId = 'e7e658fb-b3d2-48f4-a3ef-1fe09af17010'
const secondRoleId = 'f7e658fb-b3d2-48f4-a3ef-1fe09af17010'
const supportRoleId = 'c7e658fb-b3d2-48f4-a3ef-1fe09af17010'
const employee = { id: employeeId, employeeCode: 'EMP-01', name: 'Test Employee', employmentStatus: 'Active', officialEmail: 'employee@example.invalid', userId: '' }
const roles = [
  { id: roleId, code: 'OPERATIONS', name: 'Operations', description: '', active: true, version: 1 },
  { id: secondRoleId, code: 'FINANCE', name: 'Finance', description: '', active: true, version: 1 },
]
const user = {
  id: userId, employeeId, employeeName: 'Test Employee', employeeCode: 'EMP-01',
  username: 'test.employee', loginMethod: 'username', displayName: 'Test Employee', loginEmail: 'employee@example.invalid',
  accountStatus: 'Active', roleIds: [], version: 1,
}

test('create and edit a user linked to an employee with multiple roles', async ({ page }) => {
  let users: typeof user[] = []
  const writes: Record<string, unknown>[] = []
  await page.route('**/api/v1/users/options', route => route.fulfill({ json: {
    employees: [{ ...employee, userId: users.length ? userId : '' },
      { ...employee, id: actorId, employeeCode: 'EMP-02', name: 'Already Mapped', userId: actorId }],
  } }))
  await page.route('**/api/v1/roles', route => route.fulfill({ json: { roles } }))
  await page.route('**/api/v1/users', async route => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON()
      writes.push(body)
      users = [{ ...user, ...body }]
      return route.fulfill({ status: 201, json: { user: users[0] } })
    }
    await route.fulfill({ json: { users, currentUserId: actorId, capabilities: { create: true, update: true } } })
  })
  await page.route(`**/api/v1/users/${userId}`, async route => {
    const body = route.request().postDataJSON()
    writes.push(body)
    users = [{ ...users[0], ...body, version: 2 }]
    await route.fulfill({ json: { user: users[0] } })
  })
  await page.goto('/app/administration/users')
  await expect(page.getByRole('heading', { name: 'Users.' })).toBeVisible()
  await expect(page.getByRole('navigation', { name: 'Employee, user and role pages' }).getByRole('link', { name: 'Roles' })).toBeVisible()
  await page.getByRole('button', { name: 'Add user' }).first().click()
  await expect(page.getByRole('option', { name: /Already Mapped/ })).toHaveAttribute('disabled', '')
  await page.getByLabel('Employee', { exact: true }).selectOption(employeeId)
  await expect(page.getByLabel('Display name')).toHaveValue('Test Employee')
  await expect(page.getByLabel('Login email')).toHaveValue('employee@example.invalid')
  await page.getByLabel('Login with').selectOption('email')
  await expect(page.getByLabel('Account username')).toHaveValue('test.employee')
  await page.getByRole('checkbox').nth(0).check()
  await page.getByRole('checkbox').nth(1).check()
  await page.getByLabel('Temporary password').fill('six789')
  await page.getByLabel('Reason for account change').fill('New employee login')
  await page.getByRole('button', { name: 'Create user' }).click()
  await expect(page.getByRole('status').filter({ hasText: 'User created successfully.' })).toBeVisible()
  expect(writes[0]).toMatchObject({ employeeId, loginMethod: 'email', loginEmail: 'employee@example.invalid', remarks: 'New employee login', roleIds: [roleId, secondRoleId] })
  expect(writes[0]).not.toHaveProperty('permissions')
  await page.getByRole('button', { name: 'Edit test.employee' }).click()
  await expect(page.getByLabel('Temporary password')).toHaveCount(0)
  await page.getByLabel('Account status').selectOption('Inactive')
  await expect(page.getByRole('checkbox').nth(0)).toBeChecked()
  await expect(page.getByRole('checkbox').nth(1)).toBeChecked()
  await page.getByLabel('Reason for account change').fill('Access no longer needed')
  await page.getByRole('button', { name: 'Save changes' }).click()
  await expect(page.getByRole('status').filter({ hasText: 'User updated successfully.' })).toBeVisible()
  expect(writes[1]).toMatchObject({ version: 1, accountStatus: 'Inactive' })
  expect(writes[1]).not.toHaveProperty('password')
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain('six789')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy()
})

test('show user service errors and require a signed-in account', async ({ page }) => {
  await page.route('**/api/v1/users', route => route.request().method() === 'POST'
    ? route.fulfill({ status: 409, json: { error: 'username_email_or_employee_already_in_use' } })
    : route.fulfill({ json: { users: [], currentUserId: actorId, capabilities: { create: true, update: true } } }))
  await page.route('**/api/v1/users/options', route => route.fulfill({ json: { employees: [employee] } }))
  await page.route('**/api/v1/roles', route => route.fulfill({ json: { roles } }))
  await page.goto('/app/administration/users')
  await page.getByRole('button', { name: 'Add user' }).first().click()
  await page.getByLabel('Employee', { exact: true }).selectOption(employeeId)
  await page.getByLabel('Username', { exact: true }).fill('duplicate')
  await page.getByLabel('Display name').fill('Duplicate user')
  await page.getByLabel('Temporary password').fill('six789')
  await page.getByRole('button', { name: 'Create user' }).click()
  await expect(page.getByRole('dialog').getByRole('alert')).toContainText('username email or employee already in use')
  await expect(page.getByLabel('Username', { exact: true })).toHaveValue('duplicate')

  await page.route('**/api/v1/users**', route => route.fulfill({ status: 401, json: { error: 'authentication_required' } }))
  await page.goto('/app/administration/users')
  await expect(page.getByRole('heading', { name: 'User data is unavailable' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Go to sign in' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Add user' })).toHaveCount(0)
})

test('create a role from the role master', async ({ page }) => {
  let savedRoles = [...roles]
  const writes: Record<string, unknown>[] = []
  await page.route('**/api/v1/roles', async route => {
    if (route.request().method() === 'POST') {
      const requestBody = route.request().postDataJSON()
      writes.push(requestBody)
      const created = { ...roles[0], ...requestBody, id: supportRoleId, version: 1 }
      savedRoles = [...savedRoles, created]
      return route.fulfill({ status: 201, json: { role: created } })
    }
    return route.fulfill({ json: { roles: savedRoles } })
  })
  await page.route('**/api/v1/roles/*', async route => {
    const requestBody = route.request().postDataJSON()
    writes.push(requestBody)
    const updated = { ...savedRoles.find(role => role.id === supportRoleId)!, ...requestBody, version: 2 }
    savedRoles = savedRoles.map(role => role.id === supportRoleId ? updated : role)
    await route.fulfill({ json: { role: updated } })
  })
  await page.goto('/app/administration/roles')
  await expect(page.getByRole('heading', { name: 'Roles.' })).toBeVisible()
  await expect(page.getByText('They do not grant application permissions.')).toBeVisible()
  await page.getByRole('button', { name: 'Add role' }).click()
  await page.getByLabel('Role code').fill('SUPPORT')
  await page.getByLabel('Role name').fill('Support')
  await page.getByRole('button', { name: 'Create role' }).click()
  await expect(page.getByText('Role created successfully.', { exact: true })).toBeVisible()
  expect(writes[0]).toMatchObject({ code: 'SUPPORT', name: 'Support', active: true })
  await page.getByRole('button', { name: 'Edit Support' }).click()
  await page.getByLabel('Description').fill('Helps users')
  await page.getByLabel('Reason for change').fill('Clarify role purpose')
  await page.getByRole('button', { name: 'Save changes' }).click()
  await expect(page.getByText('Role updated successfully.', { exact: true })).toBeVisible()
  expect(writes[1]).toMatchObject({ version: 1, description: 'Helps users' })
})
