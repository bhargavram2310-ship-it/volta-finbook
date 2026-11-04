import { useCallback, useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { Info, Pencil, Plus, ShieldCheck } from 'lucide-react'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { LoadingState } from '@/components/screen-state'
import { apiJSON } from '@/lib/mir'
import { MasterLinks } from './users'

const roleSchema = z.object({
  id: z.string().uuid(), code: z.string(), name: z.string(), description: z.string(),
  active: z.boolean(), version: z.number().int().positive(),
})
const rolesSchema = z.object({ roles: z.array(roleSchema) })
type Role = z.infer<typeof roleSchema>
type RoleForm = Pick<Role, 'code' | 'name' | 'description' | 'active' | 'version'> & { remarks: string }
const blankRole: RoleForm = { code: '', name: '', description: '', active: true, version: 0, remarks: '' }
const readableError = (error: unknown) =>
  error instanceof z.ZodError ? 'The server returned invalid role data. Reload and try again.' :
    error instanceof Error ? error.message.replaceAll('_', ' ') : 'The request failed. Try again.'

export function RolesPage() {
  const [roles, setRoles] = useState<Role[]>([])
  const [loading, setLoading] = useState(true)
  const [available, setAvailable] = useState(false)
  const [error, setError] = useState('')
  const [formError, setFormError] = useState('')
  const [notice, setNotice] = useState('')
  const [open, setOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editing, setEditing] = useState<Role | null>(null)
  const [form, setForm] = useState<RoleForm>(blankRole)

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    setAvailable(false)
    try {
      const parsed = rolesSchema.parse(await apiJSON('/api/v1/roles'))
      setRoles(parsed.roles)
      setAvailable(true)
    } catch (loadError) {
      setError(readableError(loadError))
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => { void load() }, [load])

  function create() {
    setEditing(null)
    setForm({ ...blankRole })
    setFormError('')
    setOpen(true)
  }
  function edit(role: Role) {
    setEditing(role)
    setForm({ ...role, remarks: '' })
    setFormError('')
    setOpen(true)
  }
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      const response = await apiJSON(`/api/v1/roles${editing ? `/${editing.id}` : ''}`, {
        method: editing ? 'PATCH' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...form, ...(editing ? { version: editing.version } : {}) }),
      })
      z.object({ role: roleSchema }).parse(response)
      setOpen(false)
      setNotice(`Role ${editing ? 'updated' : 'created'} successfully.`)
      await load()
    } catch (saveError) {
      setFormError(readableError(saveError))
    } finally {
      setSaving(false)
    }
  }

  return <>
    <div className="page-heading">
      <div><div className="page-eyebrow">ADMIN CONTROL</div><h1>Roles<span className="heading-dot">.</span></h1>
        <p>Create named roles and assign multiple roles to each user.</p></div>
      {available && <Button onClick={create}><Plus size={16} />Add role</Button>}
    </div>
    <MasterLinks />
    <div className="notice notice-blue"><Info size={17} /><span>Roles are labels for organizing user accounts. They do not grant application permissions.</span></div>
    {notice && <div className="notice notice-blue" role="status">{notice}</div>}
    {error && <div className="notice notice-amber" role="alert">{error}<Button variant="outline" size="sm" onClick={() => void load()}>Try again</Button></div>}
    {loading ? <LoadingState /> : !available ?
      <section className="panel employee-panel"><div className="employee-empty"><ShieldCheck size={25} /><h2>Role data is unavailable</h2><p>Sign in and ensure the database service is available.</p><Button asChild variant="outline"><Link to="/login">Go to sign in</Link></Button></div></section> :
      <section className="panel employee-panel access-directory">
        <div className="panel-header"><div><h2>Role master</h2><p>{roles.length} roles</p></div></div>
        <div className="table-scroll"><table><caption className="sr-only">Roles available for user assignment</caption>
          <thead><tr>{['Code', 'Role', 'Description', 'Status', 'Actions'].map(label => <th scope="col" key={label}>{label}</th>)}</tr></thead>
          <tbody>{roles.map(role => <tr key={role.id}>
            <td>{role.code}</td><td><strong>{role.name}</strong></td><td>{role.description || '—'}</td>
            <td><span className={`badge ${role.active ? 'badge-teal' : 'badge-neutral'}`}>{role.active ? 'Active' : 'Inactive'}</span></td>
            <td><Button size="sm" variant="outline" onClick={() => edit(role)} aria-label={`Edit ${role.name}`}><Pencil size={14} />Edit</Button></td>
          </tr>)}
          {!roles.length && <tr><td colSpan={5}><div className="employee-empty"><p>No roles have been created.</p><Button variant="outline" onClick={create}>Add role</Button></div></td></tr>}
          </tbody></table></div>
      </section>}
    <Dialog open={open} onOpenChange={next => { if (!saving) setOpen(next) }} title={editing ? 'Edit role' : 'Add role'} description="Create a role that can be assigned to user accounts." className="employee-dialog">
      <form className="employee-form" onSubmit={save}>
        <fieldset disabled={saving} className="access-fieldset">
          <div className="employee-form-grid">
            <label className="form-field">Role code<input required maxLength={100} pattern="[A-Za-z0-9._-]+" value={form.code} onChange={event => setForm({ ...form, code: event.target.value })} /></label>
            <label className="form-field">Role name<input required maxLength={191} value={form.name} onChange={event => setForm({ ...form, name: event.target.value })} /></label>
            <label className="form-field">Status<select value={form.active ? 'active' : 'inactive'} onChange={event => setForm({ ...form, active: event.target.value === 'active' })}><option value="active">Active</option><option value="inactive">Inactive</option></select></label>
          </div>
          <label className="form-field">Description<textarea maxLength={4000} value={form.description} onChange={event => setForm({ ...form, description: event.target.value })} /></label>
          <label className="form-field">Reason for change<textarea maxLength={1000} value={form.remarks} onChange={event => setForm({ ...form, remarks: event.target.value })} /></label>
        </fieldset>
        {formError && <p role="alert" className="field-error">{formError}</p>}
        <div className="employee-form-actions"><Button type="button" variant="outline" onClick={() => setOpen(false)}>Cancel</Button><Button type="submit" disabled={saving}>{saving ? 'Saving…' : editing ? 'Save changes' : 'Create role'}</Button></div>
      </form>
    </Dialog>
  </>
}
