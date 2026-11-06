import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useLocation, useSearchParams } from 'react-router-dom'
import { Pencil, Plus, Search, Trash2 } from 'lucide-react'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { LoadingState } from '@/components/screen-state'
import { apiJSON } from '@/lib/mir'
import type { PageDefinition } from '@/lib/navigation'
import './masters.css'

const fieldSchema = z.object({
  name: z.string(), label: z.string(), type: z.string(), required: z.boolean(),
  options: z.array(z.string()).optional(), ref: z.string().optional(), default: z.string().optional(),
  max: z.number().optional(), immutable: z.boolean().optional(),
})
const recordSchema = z.object({ id: z.string(), revision: z.string(), values: z.record(z.string(), z.string()) })
const responseSchema = z.object({ title: z.string(), fields: z.array(fieldSchema), records: z.array(recordSchema) })
type MasterRecord = z.infer<typeof recordSchema>
type MasterResponse = z.infer<typeof responseSchema>

const masterTabs: Record<string, Array<[string, string]>> = {
  'sales-types': [['sales-types', 'Sales types']],
  'company': [['company', 'Company'], ['branches', 'Branches']],
  'customers': [['customers', 'Customers']],
  'agents': [['agents', 'Travel agents']],
  'suppliers': [['suppliers', 'Suppliers']],
  'travel': [['airlines', 'Airlines'], ['locations', 'Airports & geography']],
  'currencies': [['currencies', 'Currencies'], ['exchange-rates', 'Exchange rates']],
  'accounts': [['accounts', 'Chart of accounts']],
  'bank-cash': [['bank-cash', 'Bank & cash accounts']],
  'tax': [['tax', 'Tax rules'], ['commercial-rules', 'Commission & markup']],
  'refund-rules': [['commercial-rules', 'Cancellation & refund rules']],
  'periods': [['financial-years', 'Financial years'], ['periods', 'Accounting periods']],
}

function recordName(record: MasterRecord) {
  return record.values.name || record.values.legal_name || record.values.label || record.values.code || record.values.bank_name || record.id
}
function message(error: unknown) { return error instanceof Error ? error.message.replaceAll('_', ' ') : 'The master request failed.' }

export function MastersPage({ page }: { page: PageDefinition }) {
  const location = useLocation()
  const signInPath = `/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`
  const tabs = masterTabs[page.id.split('/')[1]]
  const [params, setParams] = useSearchParams()
  const kind = tabs.some(([key]) => key === params.get('tab')) ? params.get('tab')! : tabs[0][0]
  const [data, setData] = useState<MasterResponse | null>(null)
  const [references, setReferences] = useState<Record<string, MasterRecord[]>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [attempt, setAttempt] = useState(0)
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('all')
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<MasterRecord | null>(null)
  const [deleting, setDeleting] = useState<MasterRecord | null>(null)
  const [values, setValues] = useState<Record<string, string>>({})
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [referenceError, setReferenceError] = useState('')

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setError(''); setData(null); setReferences({}); setReferenceError('')
    apiJSON(`/api/v1/masters/${kind}`, { signal: controller.signal })
      .then(async body => {
        const result = responseSchema.parse(body)
        if (controller.signal.aborted) return
        setData(result)
        const refs = [...new Set(result.fields.flatMap(field => field.ref ? [field.ref] : []))]
        const results = await Promise.allSettled(refs.map(async ref => {
          const response = ref === kind ? result : responseSchema.parse(await apiJSON(`/api/v1/masters/${ref}`, { signal: controller.signal }))
          return [ref, response.records] as const
        }))
        if (controller.signal.aborted) return
        const loaded: Record<string, MasterRecord[]> = {}
        for (const result of results) {
          if (result.status === 'fulfilled') loaded[result.value[0]] = result.value[1]
          else setReferenceError('Some lookup lists could not be loaded. Retry before creating or editing a record.')
        }
        setReferences(loaded)
      })
      .catch(error => { if (!controller.signal.aborted) setError(message(error)) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [kind, attempt])

  function startEdit(record: MasterRecord | null) {
    setEditing(record)
    setValues(record ? { ...record.values } : Object.fromEntries((data?.fields ?? []).map(field => [field.name,
      page.id === 'masters/refund-rules' && field.name === 'kind' ? 'cancellation' : field.default ?? ''])))
    setFormError(''); setOpen(true)
  }
  async function submit(event: FormEvent) {
    event.preventDefault()
    if (saving || !data) return
    setSaving(true); setFormError('')
    try {
      const response = await apiJSON(`/api/v1/masters/${kind}${editing ? `/${editing.id}` : ''}`, {
        method: editing ? 'PATCH' : 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ values, revision: editing?.revision ?? '' }),
      })
      const saved = z.object({ record: recordSchema }).parse(response).record
      setData(previous => previous && { ...previous, records: [...previous.records.filter(record => record.id !== saved.id), saved] })
      setOpen(false); setNotice(editing ? 'Record updated.' : 'Record created.'); setAttempt(value => value + 1)
    } catch (error) { setFormError(message(error)) }
    finally { setSaving(false) }
  }
  async function remove() {
    if (!deleting || saving) return
    setSaving(true); setFormError('')
    try {
      await apiJSON(`/api/v1/masters/${kind}/${deleting.id}`, { method: 'DELETE', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ revision: deleting.revision }) })
      setData(previous => previous && { ...previous, records: previous.records.filter(record => record.id !== deleting.id) })
      setDeleting(null); setNotice('Record deleted.'); setAttempt(value => value + 1)
    } catch (error) { setFormError(message(error)) }
    finally { setSaving(false) }
  }
  const shown = (data?.records ?? []).filter(record => {
    if (page.id === 'masters/refund-rules' && !['cancellation', 'refund'].includes(record.values.kind)) return false
    if (page.id === 'masters/tax' && kind === 'commercial-rules' && ['cancellation', 'refund'].includes(record.values.kind)) return false
    return Object.values(record.values).some(value => value.toLowerCase().includes(query.trim().toLowerCase())) &&
      (status === 'all' || record.values.active === status)
  }).sort((a, b) => recordName(a).localeCompare(recordName(b)))
  const columns = data?.fields.filter(field => !['json', 'boolean'].includes(field.type)).slice(0, 4) ?? []
  function display(record: MasterRecord, name: string) {
    const field = data?.fields.find(field => field.name === name)
    const value = record.values[name]
    if (field?.ref && value) {
      const related = references[field.ref]?.find(option => option.id === value)
      return related ? recordName(related) : 'Unavailable record'
    }
    return value || '—'
  }
  const ready = !!data && !loading && !error
  const taxNotice = kind === 'tax' || kind === 'commercial-rules'
  return <>
    <div className="page-heading"><div><div className="page-eyebrow">MASTERS</div><h1>{page.label}<span className="heading-dot">.</span></h1><p>{page.description}</p></div><Button onClick={() => startEdit(null)} disabled={!ready || !!referenceError}><Plus size={16} />Create record</Button></div>
    {tabs.length > 1 && <nav className="master-tabs" aria-label="Master record types">{tabs.map(([key, label]) => <button key={key} aria-current={kind === key ? 'page' : undefined} onClick={() => { setParams({ tab: key }); setQuery(''); setStatus('all'); setNotice(''); setOpen(false); setDeleting(null) }}>{label}</button>)}</nav>}
    {taxNotice && <div className="notice notice-amber">Rules require finance verification before use. Creating or editing a rule leaves it inactive and awaiting verification.</div>}
    {notice && <div className="notice notice-blue" role="status">{notice}</div>}
    {(error || referenceError) && <div className="notice notice-amber" role="alert">{error === 'authentication required' ? <>Your session expired or you are not signed in. Sign in to manage master records.<Button asChild size="sm"><Link to={signInPath}>Sign in</Link></Button></> : error === 'password change required' ? <>Update your password before managing master records.<Button asChild size="sm"><Link to={signInPath}>Update password</Link></Button></> : <>{error || referenceError}<Button size="sm" variant="outline" onClick={() => setAttempt(value => value + 1)}>Retry</Button></>}</div>}
    {loading ? <LoadingState /> : data && <section className="panel register-panel" aria-label={data.title}>
      <div className="register-heading"><div><h2>{tabs.find(([key]) => key === kind)?.[1]}</h2><span className="text-muted">{shown.length} records</span></div></div>
      <div className="table-toolbar"><label className="table-search"><Search size={16} /><span className="sr-only">Search master records</span><input type="search" placeholder="Search records…" value={query} onChange={event => setQuery(event.target.value)} /></label>{data.fields.some(field => field.name === 'active') && <label className="status-filter"><span className="sr-only">Status filter</span><select value={status} onChange={event => setStatus(event.target.value)}><option value="all">All statuses</option><option value="true">Active</option><option value="false">Inactive</option></select></label>}</div>
      <div className="table-scroll"><table><caption className="sr-only">{data.title} records</caption><thead><tr>{columns.map(field => <th key={field.name} scope="col">{field.label}</th>)}{data.fields.some(field => field.name === 'active') && <th scope="col">Status</th>}<th scope="col">Actions</th></tr></thead><tbody>{shown.map(record => <tr key={record.id}>{columns.map(field => <td key={field.name}>{display(record, field.name)}</td>)}{data.fields.some(field => field.name === 'active') && <td><span className={`badge badge-${record.values.active === 'true' ? 'teal' : 'neutral'}`}>{record.values.active === 'true' ? 'Active' : 'Inactive'}</span></td>}<td><div className="master-row-actions"><Button size="sm" variant="outline" disabled={!!referenceError} aria-label={`Edit ${recordName(record)}`} onClick={() => startEdit(record)}><Pencil size={14} />Edit</Button><Button size="sm" variant="outline" aria-label={`Delete ${recordName(record)}`} onClick={() => { setDeleting(record); setFormError('') }}><Trash2 size={14} />Delete</Button></div></td></tr>)}{!shown.length && <tr><td colSpan={columns.length + 2}><div className="master-empty"><h3>{query || status !== 'all' ? 'No matching records' : 'No records yet'}</h3><p>{query || status !== 'all' ? 'Change your search or status filter.' : 'Create a record to make it available in the relevant lookup lists.'}</p></div></td></tr>}</tbody></table></div>
    </section>}
    <Dialog open={open} onOpenChange={value => { if (!saving) setOpen(value) }} title={editing ? 'Edit record' : 'Create record'} description={data?.title ?? 'Master record'} className="master-dialog">
      <form onSubmit={submit}><div className="master-form-grid">{data?.fields.map(field => {
        const options = field.options?.filter(value => field.name !== 'kind' || kind !== 'commercial-rules' || (page.id === 'masters/refund-rules' ? ['cancellation', 'refund'].includes(value) : !['cancellation', 'refund'].includes(value)))
        const candidates = field.ref ? (references[field.ref] ?? []).filter(record => field.name !== 'parent_id' || record.id !== editing?.id) : []
        const value = values[field.name] ?? ''
        const unavailable = field.ref && value && !candidates.some(record => record.id === value)
        const id = `master-${field.name}`
        return <div key={field.name} className={`form-field ${field.type === 'json' ? 'master-field-wide' : ''}`}><label htmlFor={id}>{field.label}{field.required && ' *'}</label>{field.type === 'boolean' ? <select id={id} value={value} disabled={saving || (!!editing && !!field.immutable)} onChange={event => setValues(previous => ({ ...previous, [field.name]: event.target.value }))}><option value="true">Yes</option><option value="false">No</option></select> : field.type === 'reference' || field.type === 'select' ? <><select id={id} required={field.required} value={value} disabled={saving || (!!editing && !!field.immutable)} onChange={event => setValues(previous => ({ ...previous, [field.name]: event.target.value }))}><option value="">Select {field.label.toLowerCase()}…</option>{unavailable && <option value={value}>Unavailable record — choose another</option>}{field.ref ? candidates.map(record => <option key={record.id} value={record.id}>{record.values.code ? `${record.values.code} — ` : ''}{recordName(record)}{record.values.active === 'false' ? ' (inactive)' : ''}</option>) : options?.map(option => <option key={option} value={option}>{option}</option>)}</select>{field.ref && !candidates.length && <small>No records available. Create the related master first.</small>}</> : field.type === 'json' ? <textarea id={id} value={value} required={field.required} disabled={saving} onChange={event => setValues(previous => ({ ...previous, [field.name]: event.target.value }))} /> : <input id={id} type={field.type === 'date' ? 'date' : 'text'} inputMode={['decimal', 'rate', 'percent'].includes(field.type) ? 'decimal' : ['integer', 'precision'].includes(field.type) ? 'numeric' : undefined} required={field.required} readOnly={!!editing && field.immutable} disabled={saving} maxLength={field.max} value={value} onChange={event => setValues(previous => ({ ...previous, [field.name]: event.target.value }))} />}</div>
      })}</div>{formError && <p className="field-error" role="alert">{formError}</p>}<div className="master-dialog-actions"><Button type="button" variant="outline" disabled={saving} onClick={() => setOpen(false)}>Cancel</Button><Button type="submit" disabled={saving || !!referenceError}>{saving ? 'Saving…' : editing ? 'Save changes' : 'Create record'}</Button></div></form>
    </Dialog>
    <Dialog open={!!deleting} onOpenChange={value => { if (!value && !saving) setDeleting(null) }} title="Delete record" description="Records used by other records or transactions cannot be deleted."><p>Delete <strong>{deleting && recordName(deleting)}</strong>?</p><p className="text-muted">This removes the master record. Its audit history is retained.</p>{formError && <p className="field-error" role="alert">{formError}</p>}<div className="master-dialog-actions"><Button variant="outline" disabled={saving} onClick={() => setDeleting(null)}>Cancel</Button><Button disabled={saving} onClick={() => void remove()}>{saving ? 'Deleting…' : 'Delete record'}</Button></div></Dialog>
  </>
}
