import { useCallback, useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { Info, Pencil, Plus, Search, UsersRound } from 'lucide-react'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { LoadingState } from '@/components/screen-state'
import { apiJSON } from '@/lib/mir'
import { MasterLinks } from './users'

const employeeSchema = z.object({
  id: z.string().uuid(),
  employeeCode: z.string(),
  name: z.string(),
  officialEmail: z.string(),
  mobileNumber: z.string(),
  designation: z.string(),
  department: z.string(),
  employmentStatus: z.enum(['Active', 'Inactive', 'Resigned']),
  dateOfJoining: z.string(),
  dateOfLeaving: z.string(),
  version: z.number().int().positive(),
})
type Employee = z.infer<typeof employeeSchema>
const employeesResponseSchema = z.object({
  employees: z.array(employeeSchema),
  capabilities: z.object({ create: z.boolean(), update: z.boolean() }),
})
const profileSchema = z.object({ employee: employeeSchema })
type EmployeeForm = Omit<Employee, 'id' | 'version'> & { version?: number }
const blankEmployee: EmployeeForm = {
  employeeCode: '', name: '', officialEmail: '', mobileNumber: '', designation: '',
  department: '', employmentStatus: 'Active', dateOfJoining: '', dateOfLeaving: '',
}

function errorText(error: unknown) {
  return error instanceof Error ? error.message.replaceAll('_', ' ') : 'The employee request failed.'
}

export function Employees() {
  const [profile, setProfile] = useState<Employee | null>(null)
  const [employees, setEmployees] = useState<Employee[]>([])
  const [capabilities, setCapabilities] = useState({ create: false, update: false })
  const [loading, setLoading] = useState(true)
  const [signedIn, setSignedIn] = useState(true)
  const [directoryAvailable, setDirectoryAvailable] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [query, setQuery] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<Employee | null>(null)
  const [form, setForm] = useState<EmployeeForm>(blankEmployee)
  const [saving, setSaving] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    const [profileResult, directoryResult] = await Promise.allSettled([
      apiJSON('/api/v1/employees/me'),
      apiJSON('/api/v1/employees'),
    ])
    if (profileResult.status === 'fulfilled') {
      const parsed = profileSchema.safeParse(profileResult.value)
      if (parsed.success) setProfile(parsed.data.employee)
      else setError('The employee profile response was invalid.')
    } else {
      const message = errorText(profileResult.reason)
      if (message.includes('authentication required') || message.includes('unauthorized')) {
        setSignedIn(false)
      } else {
        setError(message)
      }
    }
    if (directoryResult.status === 'fulfilled') {
      const parsed = employeesResponseSchema.safeParse(directoryResult.value)
      if (parsed.success) {
        setEmployees(parsed.data.employees)
        setCapabilities(parsed.data.capabilities)
        setDirectoryAvailable(true)
      } else {
        setError('The employee directory response was invalid.')
      }
    } else {
      const message = errorText(directoryResult.reason)
      if (!message.includes('authentication required') && !message.includes('unauthorized')) {
        setError(message)
      }
      setDirectoryAvailable(false)
    }
    setLoading(false)
  }, [])

  useEffect(() => { void load() }, [load])

  const filteredEmployees = useMemo(() => {
    const normalized = query.trim().toLowerCase()
    if (!normalized) return employees
    return employees.filter(employee => [
      employee.employeeCode, employee.name, employee.officialEmail, employee.department, employee.designation,
    ].some(value => value.toLowerCase().includes(normalized)))
  }, [employees, query])

  function openCreate() {
    setEditing(null)
    setForm(blankEmployee)
    setError('')
    setDialogOpen(true)
  }

  function openEdit(employee: Employee) {
    setEditing(employee)
    setForm({
      employeeCode: employee.employeeCode, name: employee.name, officialEmail: employee.officialEmail,
      mobileNumber: employee.mobileNumber, designation: employee.designation, department: employee.department,
      employmentStatus: employee.employmentStatus, dateOfJoining: employee.dateOfJoining,
      dateOfLeaving: employee.dateOfLeaving, version: employee.version,
    })
    setError('')
    setDialogOpen(true)
  }

  async function saveEmployee(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSaving(true)
    setError('')
    try {
      const response = await apiJSON(editing ? `/api/v1/employees/${editing.id}` : '/api/v1/employees', {
        method: editing ? 'PATCH' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form),
      })
      const parsed = z.object({ employee: employeeSchema }).safeParse(response)
      if (!parsed.success) throw new Error('The saved employee response was invalid.')
      setDialogOpen(false)
      setNotice(editing ? 'Employee details updated.' : 'Employee created.')
      await load()
    } catch (saveError) {
      setError(errorText(saveError))
    } finally {
      setSaving(false)
    }
  }

  return <>
    <div className="page-heading">
      <div><div className="page-eyebrow">ADMIN CONTROL</div><h1>Employees<span className="heading-dot">.</span></h1>
        <p>Manage employee records and view your own employee information.</p>
      </div>
      {directoryAvailable && capabilities.create && <Button onClick={openCreate}><Plus size={16} />Add employee</Button>}
    </div>
    <MasterLinks />
    <div className="notice notice-blue"><Info size={17} /><span>Create the employee here, then use Users to create a login account linked to that employee.</span></div>
    {notice && <div className="notice notice-blue" role="status">{notice}</div>}
    {error && <div className="notice notice-amber" role="alert">{error}<Button variant="outline" size="sm" onClick={() => void load()}>Try again</Button></div>}
    {loading ? <LoadingState /> : !signedIn ? <section className="panel employee-panel"><div className="employee-empty"><UsersRound size={25} /><h2>Sign in to view employee information</h2><p>Your profile and employee directory are available after authentication.</p><Button asChild variant="outline"><a href="/login">Go to sign in</a></Button></div></section> : <>
      <section className="panel employee-panel" aria-labelledby="employee-profile-heading">
        <div className="panel-header"><div><h2 id="employee-profile-heading">My employee information</h2><p>Only your own profile is shown here.</p></div></div>
        {profile ? <EmployeeDetails employee={profile} /> : <div className="employee-empty"><p>No employee profile is linked to your account. Contact an administrator.</p></div>}
      </section>
      {directoryAvailable && <section className="panel employee-panel employee-directory" aria-labelledby="employee-directory-heading">
        <div className="panel-header"><div><h2 id="employee-directory-heading">Employee directory</h2><p>Employee changes are recorded in the audit log.</p></div><span className="badge badge-blue">{employees.length} employees</span></div>
        <div className="table-toolbar">
          <label className="table-search"><Search size={16} /><span className="sr-only">Search employees</span><input type="search" aria-label="Search employees" placeholder="Search name, code, email or department…" value={query} onChange={event => setQuery(event.target.value)} /></label>
        </div>
        <div className="table-scroll"><table><caption className="sr-only">Employee directory</caption><thead><tr>
          <th scope="col">Employee code</th><th scope="col">Name</th><th scope="col">Email / mobile</th><th scope="col">Department / designation</th><th scope="col">Status</th>
          {capabilities.update && <th scope="col"><span className="sr-only">Actions</span></th>}
        </tr></thead><tbody>
          {filteredEmployees.map(employee => <tr key={employee.id}>
            <td>{employee.employeeCode}</td><td>{employee.name}</td>
            <td>{employee.officialEmail || '—'}<br /><span className="text-muted">{employee.mobileNumber || '—'}</span></td>
            <td>{employee.department || '—'}<br /><span className="text-muted">{employee.designation || '—'}</span></td>
            <td><span className={`badge ${employee.employmentStatus === 'Active' ? 'badge-teal' : 'badge-neutral'}`}>{employee.employmentStatus}</span></td>
            {capabilities.update && <td><Button variant="outline" size="sm" aria-label={`Edit ${employee.name}`} onClick={() => openEdit(employee)}><Pencil size={14} />Edit</Button></td>}
          </tr>)}
          {!filteredEmployees.length && <tr><td colSpan={capabilities.update ? 6 : 5}><div className="employee-empty"><p>{query ? 'No employees match your search.' : 'No employee records are available.'}</p></div></td></tr>}
        </tbody></table></div>
        <div className="employee-mobile-list">
          {filteredEmployees.map(employee => <article className="employee-mobile-card" key={employee.id}>
            <div className="employee-mobile-card-heading"><div><strong>{employee.name}</strong><span>{employee.employeeCode}</span></div><span className={`badge ${employee.employmentStatus === 'Active' ? 'badge-teal' : 'badge-neutral'}`}>{employee.employmentStatus}</span></div>
            <dl><div><dt>Email / mobile</dt><dd>{employee.officialEmail || '—'}{employee.mobileNumber ? ` · ${employee.mobileNumber}` : ''}</dd></div><div><dt>Department / designation</dt><dd>{employee.department || '—'}{employee.designation ? ` · ${employee.designation}` : ''}</dd></div></dl>
            {capabilities.update && <Button variant="outline" size="sm" aria-label={`Edit ${employee.name}`} onClick={() => openEdit(employee)}><Pencil size={14} />Edit</Button>}
          </article>)}
          {!filteredEmployees.length && <div className="employee-empty"><p>{query ? 'No employees match your search.' : 'No employee records are available.'}</p></div>}
        </div>
      </section>}
    </>}
    <Dialog open={dialogOpen} onOpenChange={setDialogOpen} title={editing ? 'Edit employee' : 'Add employee'} description="Changes are saved to the employee record and written to the audit log." className="employee-dialog">
      <form className="employee-form" onSubmit={saveEmployee}>
        <div className="employee-form-grid">
          <label className="form-field">Employee code<input required maxLength={100} value={form.employeeCode} onChange={event => setForm({ ...form, employeeCode: event.target.value })} /></label>
          <label className="form-field">Full name<input required maxLength={191} value={form.name} onChange={event => setForm({ ...form, name: event.target.value })} /></label>
          <label className="form-field">Official email<input type="email" maxLength={254} value={form.officialEmail} onChange={event => setForm({ ...form, officialEmail: event.target.value })} /></label>
          <label className="form-field">Mobile number<input maxLength={64} value={form.mobileNumber} onChange={event => setForm({ ...form, mobileNumber: event.target.value })} /></label>
          <label className="form-field">Department<input maxLength={100} value={form.department} onChange={event => setForm({ ...form, department: event.target.value })} /></label>
          <label className="form-field">Designation<input maxLength={100} value={form.designation} onChange={event => setForm({ ...form, designation: event.target.value })} /></label>
          <label className="form-field">Employment status<select value={form.employmentStatus} onChange={event => setForm({ ...form, employmentStatus: event.target.value as EmployeeForm['employmentStatus'] })}><option>Active</option><option>Inactive</option><option>Resigned</option></select></label>
          <label className="form-field">Date of joining<input type="date" value={form.dateOfJoining} onChange={event => setForm({ ...form, dateOfJoining: event.target.value })} /></label>
          <label className="form-field">Date of leaving<input type="date" value={form.dateOfLeaving} onChange={event => setForm({ ...form, dateOfLeaving: event.target.value })} /></label>
        </div>
        {error && <p role="alert" className="field-error">{error}</p>}
        <div className="employee-form-actions"><Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button><Button type="submit" disabled={saving}>{saving ? 'Saving…' : editing ? 'Save changes' : 'Create employee'}</Button></div>
      </form>
    </Dialog>
  </>
}

function EmployeeDetails({ employee }: { employee: Employee }) {
  const fields: Array<[string, string]> = [
    ['Employee code', employee.employeeCode], ['Full name', employee.name],
    ['Official email', employee.officialEmail], ['Mobile number', employee.mobileNumber],
    ['Department', employee.department], ['Designation', employee.designation],
    ['Employment status', employee.employmentStatus], ['Date of joining', employee.dateOfJoining],
  ]
  return <dl className="employee-details">{fields.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value || '—'}</dd></div>)}</dl>
}
