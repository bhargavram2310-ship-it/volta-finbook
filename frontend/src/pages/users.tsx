import { useCallback, useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { Info, Pencil, Plus, Search, UsersRound } from 'lucide-react'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { LoadingState } from '@/components/screen-state'
import { apiJSON } from '@/lib/mir'

const userSchema = z.object({
  id: z.string().uuid(), employeeId: z.string().uuid(), employeeName: z.string(), employeeCode: z.string(),
  username: z.string(), loginMethod: z.enum(['username', 'email']), displayName: z.string(), loginEmail: z.string(),
  accountStatus: z.enum(['Active', 'Inactive', 'Locked']), roleIds: z.array(z.string().uuid()), version: z.number().int().positive(),
})
const roleSchema = z.object({
  id: z.string().uuid(), code: z.string(), name: z.string(), description: z.string(),
  active: z.boolean(), version: z.number().int().positive(),
})
const usersSchema = z.object({
  users: z.array(userSchema), currentUserId: z.string().uuid(),
  capabilities: z.object({ create: z.boolean(), update: z.boolean() }),
})
const optionsSchema = z.object({
  employees: z.array(z.object({
    id: z.string().uuid(), employeeCode: z.string(), name: z.string(), employmentStatus: z.string(),
    officialEmail: z.string(), userId: z.string(),
  })),
})
type User = z.infer<typeof userSchema>
type UserForm = {
  employeeId: string
  username: string
  loginMethod: User['loginMethod']
  displayName: string
  loginEmail: string
  roleIds: string[]
  accountStatus: User['accountStatus']
  password: string
  remarks: string
}
const blankUser: UserForm = {
  employeeId: '', username: '', loginMethod: 'username', displayName: '', loginEmail: '', roleIds: [], accountStatus: 'Active', password: '', remarks: '',
}
const readableError = (error: unknown) =>
  error instanceof z.ZodError ? 'The server returned an invalid response. Please reload and try again.' :
    error instanceof Error ? error.message.replaceAll('_', ' ') : 'The request failed. Please try again.'
const usernameFromName = (name: string) =>
  name.toLowerCase().trim().replace(/\s+/g, '.').replace(/[^a-z0-9._-]/g, '').slice(0, 191)

export function MasterLinks() {
  return <nav className="master-links" aria-label="Employee, user and role pages">
    <Link to="/app/administration/employees">Employees</Link>
    <Link to="/app/administration/users">Users</Link>
    <Link to="/app/administration/roles">Roles</Link>
  </nav>
}

export function UsersPage() {
  const [users, setUsers] = useState<User[]>([])
  const [roles, setRoles] = useState<z.infer<typeof roleSchema>[]>([])
  const [options, setOptions] = useState<z.infer<typeof optionsSchema>>({ employees: [] })
  const [capabilities, setCapabilities] = useState({ create: false, update: false })
  const [currentUserId, setCurrentUserId] = useState('')
  const [loading, setLoading] = useState(true)
  const [available, setAvailable] = useState(false)
  const [error, setError] = useState('')
  const [formError, setFormError] = useState('')
  const [notice, setNotice] = useState('')
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('All')
  const [open, setOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editing, setEditing] = useState<User | null>(null)
  const [form, setForm] = useState<UserForm>(blankUser)

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    setAvailable(false)
    setCapabilities({ create: false, update: false })
    try {
      const [directory, choices, roleDirectory] = await Promise.all([
        apiJSON('/api/v1/users'),
        apiJSON('/api/v1/users/options'),
        apiJSON('/api/v1/roles'),
      ])
      const parsed = usersSchema.parse(directory)
      setUsers(parsed.users)
      setCurrentUserId(parsed.currentUserId)
      setCapabilities(parsed.capabilities)
      setOptions(optionsSchema.parse(choices))
      setRoles(z.object({ roles: z.array(roleSchema) }).parse(roleDirectory).roles)
      setAvailable(true)
    } catch (loadError) {
      setError(readableError(loadError))
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => { void load() }, [load])

  const visibleUsers = useMemo(() => users.filter(user =>
    (status === 'All' || user.accountStatus === status) &&
    [user.username, user.displayName, user.loginEmail, user.employeeName, user.employeeCode,
      ...roles.filter(role => user.roleIds.includes(role.id)).map(role => role.name)]
      .some(value => value.toLowerCase().includes(query.trim().toLowerCase()))
  ), [users, roles, status, query])

  function create() {
    setEditing(null)
    setForm({ ...blankUser })
    setFormError('')
    setOpen(true)
  }
  function edit(user: User) {
    setEditing(user)
    setForm({
      employeeId: user.employeeId, username: user.username, displayName: user.displayName,
      loginMethod: user.loginMethod, loginEmail: user.loginEmail, roleIds: user.roleIds,
      accountStatus: user.accountStatus, password: '', remarks: '',
    })
    setFormError('')
    setOpen(true)
  }
  function changeOpen(next: boolean) {
    if (!saving) {
      setOpen(next)
      if (!next) setForm(previous => ({ ...previous, password: '' }))
    }
  }
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const rolesChanged = editing
      ? form.roleIds.length !== editing.roleIds.length || form.roleIds.some(id => !editing.roleIds.includes(id))
      : form.roleIds.length > 0
    if (rolesChanged && !form.remarks.trim()) {
      setFormError('Enter a reason when changing a user’s role assignments.')
      return
    }
    setSaving(true)
    setFormError('')
    try {
      const response = await apiJSON(`/api/v1/users${editing ? `/${editing.id}` : ''}`, {
        method: editing ? 'PATCH' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          ...form,
          ...(editing ? { password: undefined, version: editing.version } : {}),
        }),
      })
      z.object({ user: userSchema }).parse(response)
      setOpen(false)
      setForm(previous => ({ ...previous, password: '' }))
      setNotice(`User ${editing ? 'updated' : 'created'} successfully.`)
      await load()
    } catch (saveError) {
      setFormError(readableError(saveError))
    } finally {
      setSaving(false)
    }
  }

  return <>
    <div className="page-heading">
      <div><div className="page-eyebrow">ADMIN CONTROL</div><h1>Users<span className="heading-dot">.</span></h1>
        <p>Create login accounts and connect them to employee records.</p></div>
      {available && capabilities.create && <Button onClick={create}><Plus size={16} />Add user</Button>}
    </div>
    <MasterLinks />
    <div className="notice notice-blue"><Info size={17} /><span>Create an employee first. Each employee can be linked to one login account; new users must change their temporary password at first sign-in.</span></div>
    {notice && <div className="notice notice-blue" role="status">{notice}</div>}
    {error && <div className="notice notice-amber" role="alert">{error}<Button variant="outline" size="sm" onClick={() => void load()}>Try again</Button></div>}
    {loading ? <LoadingState /> : !available ?
      <section className="panel employee-panel"><div className="employee-empty"><UsersRound size={25} /><h2>User data is unavailable</h2><p>Sign in and ensure the database service is available.</p><Button asChild variant="outline"><Link to="/login">Go to sign in</Link></Button></div></section> :
      <section className="panel employee-panel access-directory">
        <div className="panel-header"><div><h2>User accounts</h2><p>Each account is linked to one employee.</p></div><span className="badge badge-blue">{users.length} users</span></div>
        <div className="table-toolbar">
          <label className="table-search"><Search size={16} /><input type="search" aria-label="Search users" placeholder="Search users…" value={query} onChange={event => setQuery(event.target.value)} /></label>
          <label className="status-filter">Status <select aria-label="Filter users by status" value={status} onChange={event => setStatus(event.target.value)}><option>All</option><option>Active</option><option>Inactive</option><option>Locked</option></select></label>
        </div>
        <div className="table-scroll"><table><caption className="sr-only">User accounts and employee links</caption>
          <thead><tr>{['User / login', 'Employee', 'Roles', 'Status', 'Actions'].map(column => <th scope="col" key={column}>{column}</th>)}</tr></thead>
          <tbody>{visibleUsers.map(user => <tr key={user.id}>
            <td><strong>{user.displayName}</strong><br />{user.loginMethod === 'email' ? user.loginEmail : user.username}
              <br /><span className="text-muted">Login by {user.loginMethod === 'email' ? 'email' : 'username'}</span></td>
            <td>{user.employeeName}<br /><span className="text-muted">{user.employeeCode}</span></td>
            <td>{user.roleIds.length ? roles.filter(role => user.roleIds.includes(role.id)).map(role => role.name).join(', ') : 'No roles assigned'}</td>
            <td><Status value={user.accountStatus} /></td>
            <td>{capabilities.update && user.id !== currentUserId ?
              <Button size="sm" variant="outline" onClick={() => edit(user)} aria-label={`Edit ${user.username}`}><Pencil size={14} />Edit</Button> :
              user.id === currentUserId ? 'Your account' : 'View only'}</td>
          </tr>)}
          {!visibleUsers.length && <tr><td colSpan={5}><div className="employee-empty"><p>{query || status !== 'All' ? 'No users match your filters.' : 'No users have been created.'}</p>{capabilities.create && <Button variant="outline" onClick={create}>Add user</Button>}</div></td></tr>}
          </tbody></table></div>
      </section>}
    <Dialog open={open} onOpenChange={changeOpen} title={editing ? 'Edit user' : 'Add user'} description="Enter login details and select the employee for this account." className="employee-dialog">
      <form className="employee-form" onSubmit={save}>
        <fieldset disabled={saving} className="access-fieldset">
          <div className="employee-form-grid">
            <label className="form-field">Employee<select aria-label="Employee" required value={form.employeeId} onChange={event => {
              const employee = options.employees.find(item => item.id === event.target.value)
              setForm({
                ...form, employeeId: event.target.value, displayName: employee?.name ?? '',
                loginEmail: employee?.officialEmail ?? '', username: usernameFromName(employee?.name ?? ''),
              })
            }}>
              <option value="">Select employee</option>{options.employees.map(employee => <option key={employee.id} value={employee.id} disabled={Boolean(employee.userId && employee.userId !== editing?.id) || (employee.employmentStatus !== 'Active' && employee.id !== editing?.employeeId)}>{employee.employeeCode} · {employee.name}{employee.userId && employee.userId !== editing?.id ? ' (already linked)' : employee.employmentStatus !== 'Active' ? ` (${employee.employmentStatus})` : ''}</option>)}
            </select></label>
            <label className="form-field">Login with<select value={form.loginMethod} onChange={event => setForm({ ...form, loginMethod: event.target.value as User['loginMethod'] })}><option value="username">Username</option><option value="email">Email</option></select></label>
            <label className="form-field">{form.loginMethod === 'username' ? 'Username' : 'Account username'}<input required maxLength={191} pattern="[^@]+" title="Use a username without @." autoComplete="off" value={form.username} onChange={event => setForm({ ...form, username: event.target.value })} />{form.loginMethod === 'email' && <span className="text-muted">Used for account records; sign-in uses the email address.</span>}</label>
            <label className="form-field">Display name<input required maxLength={191} value={form.displayName} onChange={event => setForm({ ...form, displayName: event.target.value })} /></label>
            <label className="form-field">Login email<input required={form.loginMethod === 'email'} type="email" maxLength={254} value={form.loginEmail} onChange={event => setForm({ ...form, loginEmail: event.target.value })} /></label>
            <label className="form-field">Account status<select value={form.accountStatus} onChange={event => setForm({ ...form, accountStatus: event.target.value as User['accountStatus'] })}><option>Active</option><option>Inactive</option><option>Locked</option></select></label>
            {!editing && <label className="form-field">Temporary password<input required type="password" minLength={6} maxLength={72} autoComplete="new-password" value={form.password} onChange={event => setForm({ ...form, password: event.target.value })} /><span className="text-muted">6–72 characters; changed at first sign-in.</span></label>}
          </div>
          <fieldset className="access-options">
            <legend>Assign roles</legend>
            {roles.length ? roles.map(role => <label key={role.id}>
              <input type="checkbox" checked={form.roleIds.includes(role.id)} onChange={event => {
                const roleIds = event.target.checked
                  ? [...form.roleIds, role.id]
                  : form.roleIds.filter(id => id !== role.id)
                setForm({ ...form, roleIds })
              }} />
              <span>{role.name} <span className="text-muted">({role.code}{role.active ? '' : ', inactive'})</span></span>
            </label>) : <p className="text-muted">No roles created. <Link to="/app/administration/roles">Open Roles master</Link>.</p>}
          </fieldset>
          <label className="form-field">Reason for account change<textarea maxLength={1000} value={form.remarks} onChange={event => setForm({ ...form, remarks: event.target.value })} /></label>
        </fieldset>
        {formError && <p role="alert" className="field-error">{formError}</p>}
        <div className="employee-form-actions"><Button type="button" variant="outline" onClick={() => changeOpen(false)}>Cancel</Button><Button type="submit" disabled={saving}>{saving ? 'Saving…' : editing ? 'Save changes' : 'Create user'}</Button></div>
      </form>
    </Dialog>
  </>
}

function Status({ value }: { value: User['accountStatus'] }) {
  const style = value === 'Active' ? 'badge-teal' : value === 'Locked' ? 'badge-amber' : 'badge-neutral'
  return <span className={`badge ${style}`}>{value}</span>
}
