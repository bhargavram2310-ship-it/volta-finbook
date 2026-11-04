import { useEffect, useRef, useState } from 'react'
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { ArrowRight, Check, FileText, History, LoaderCircle, LockKeyhole, Pencil, RefreshCw, ScanLine } from 'lucide-react'
import { ModulePage } from './module-page'
import { allPages } from '@/lib/navigation'
import { apiJSON, mirRecordSchema, mirResponseSchema, mirSourceSchema } from '@/lib/mir'
import type { MirRecord, MirSource, SalesField } from '@/lib/mir'
import { useSalesDraft } from '@/components/sales-draft-context'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { EmptyState, LoadingState } from '@/components/screen-state'
import '@/sales-entry.css'

export function InternationalSales() {
  const location = useLocation()
  const [params, setParams] = useSearchParams()
  const isImport = params.get('mode') ? params.get('mode') === 'mir' : location.pathname.endsWith('/mir')
  const page = allPages.find(p => `/app/${p.id}` === location.pathname)!
  const selectMode = (mode: 'manual' | 'mir') => { const next = new URLSearchParams(params); next.set('mode', mode); setParams(next) }
  return <ModulePage page={page}><section className="international-entry" aria-label="International sales entry modes"><div className="sales-mode-tabs" role="tablist" aria-label="Sales entry method" onKeyDown={event => {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const mode = event.key === 'Home' ? 'manual' : event.key === 'End' ? 'mir' : isImport ? 'manual' : 'mir'
    selectMode(mode); requestAnimationFrame(() => document.getElementById(`${mode}-tab`)?.focus())
  }}>
    <button id="manual-tab" role="tab" tabIndex={isImport ? -1 : 0} aria-selected={!isImport} aria-controls="manual-panel" onClick={() => selectMode('manual')}><Pencil size={17} />Manual entry</button>
    <button id="mir-tab" role="tab" tabIndex={isImport ? 0 : -1} aria-selected={isImport} aria-controls="mir-panel" onClick={() => selectMode('mir')}><ScanLine size={17} />MIR import</button>
  </div>{isImport ? <div id="mir-panel" role="tabpanel" aria-labelledby="mir-tab"><MirPicker /></div> : <div id="manual-panel" role="tabpanel" aria-labelledby="manual-tab"><SalesEditor /></div>}</section></ModulePage>
}

function MirPicker() {
  const navigate = useNavigate()
  const { draft, load } = useSalesDraft()
  const [source, setSource] = useState<MirSource | null>(null)
  const [sourceLoading, setSourceLoading] = useState(true)
  const [sourceError, setSourceError] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [dateError, setDateError] = useState('')
  const [error, setError] = useState('')
  const [records, setRecords] = useState<MirRecord[] | null>(null)
  const [issues, setIssues] = useState<string[]>([])
  const [selected, setSelected] = useState('')
  const [loading, setLoading] = useState(false)
  const [proceeding, setProceeding] = useState(false)
  const [replacement, setReplacement] = useState<MirRecord | null>(null)
  const [notice, setNotice] = useState('')
  const abort = useRef<AbortController | null>(null)
  const generation = useRef(0)
  const mounted = useRef(false)
  const selectedRecord = records?.find(record => `${record.fileName}:${record.id}` === selected)

  const refreshSource = async () => {
    setSourceLoading(true); setSourceError('')
    try { const result = mirSourceSchema.parse(await apiJSON('/api/v1/mir/source')); if (mounted.current) setSource(result) }
    catch { if (mounted.current) setSourceError('MIR folder settings could not load. Start the Go API and retry.') }
    finally { if (mounted.current) setSourceLoading(false) }
  }
  useEffect(() => { mounted.current = true; void refreshSource(); return () => { mounted.current = false; abort.current?.abort() } }, [])

  const invalidate = () => { generation.current++; abort.current?.abort(); setLoading(false); setProceeding(false); setRecords(null); setSelected(''); setError(''); setDateError(''); setNotice(''); setIssues([]) }
  const fetchRecords = async () => {
    invalidate()
    if (!from || !to || from > to || !/^\d{4}-\d{2}-\d{2}$/.test(from) || !/^\d{4}-\d{2}-\d{2}$/.test(to)) { setDateError('Choose valid From and To dates, with From on or before To.'); return }
    const current = ++generation.current
    const controller = new AbortController(); abort.current = controller
    setLoading(true)
    try {
      const response = mirResponseSchema.parse(await apiJSON(`/api/v1/mir/records?${new URLSearchParams({ from, to })}`, { signal: AbortSignal.any([controller.signal, AbortSignal.timeout(70_000)]) }))
      if (current !== generation.current || !mounted.current) return
      setRecords(response.records); setIssues(response.issues)
    } catch (error) { if (current === generation.current && !controller.signal.aborted) setError(error instanceof Error ? error.message : 'MIR records could not load.') }
    finally { if (current === generation.current) setLoading(false) }
  }
  const openRecord = (record: MirRecord) => { load(record); navigate('/app/international/sales?mode=manual') }
  const proceed = async () => {
    if (!selectedRecord || proceeding) return
    if (draft.source?.id === selectedRecord.id || (draft.values.ticketNumber && draft.values.ticketNumber === selectedRecord.fields.ticketNumber?.value)) { setNotice('This ticket is already open in sales. Existing values and corrections have been preserved.'); return }
    setProceeding(true); setError('')
    const current = generation.current
    try {
      const response = mirRecordSchema.parse(await apiJSON(`/api/v1/mir/record?${new URLSearchParams({ file: selectedRecord.fileName, sha256: selectedRecord.sha256, id: selectedRecord.id })}`))
      if (current !== generation.current || !mounted.current) return
      if (response.id !== selectedRecord.id || response.sha256 !== selectedRecord.sha256 || response.fileName !== selectedRecord.fileName || ['Failed', 'Duplicate'].includes(response.status)) throw new Error('MIR source changed. Fetch the records again.')
      if (Object.values(draft.values).some(Boolean)) setReplacement(response); else openRecord(response)
    } catch (error) { if (current === generation.current && mounted.current) setError(error instanceof Error ? error.message : 'Could not open this MIR record.') }
    finally { if (current === generation.current && mounted.current) setProceeding(false) }
  }
  return <>
    <div className="notice notice-blue">
      <div>{sourceLoading ? <span role="status">Checking MIR source configuration...</span> : source?.locked ? <span>MIR source folder is configured and locked.</span> : !sourceError ? <span>Configure the MIR source folder in Admin Control before fetching files.</span> : null}
      <Button asChild variant="outline" size="sm"><Link to="/app/administration/mir-source">Admin Control: MIR File Reader Location</Link></Button>
      {sourceError && <div role="alert"><p>{sourceError}</p><Button variant="outline" size="sm" onClick={() => void refreshSource()}>Retry settings</Button></div>}</div>
    </div>
    <div className="mir-date-filter"><div><label htmlFor="mir-from">From date</label><input id="mir-from" type="date" value={from} onChange={e => { invalidate(); setFrom(e.target.value) }} /></div><div><label htmlFor="mir-to">To date</label><input id="mir-to" type="date" value={to} onChange={e => { invalidate(); setTo(e.target.value) }} /></div><Button disabled={!source?.locked || loading || proceeding} onClick={() => void fetchRecords()}>{loading ? <LoaderCircle size={16} className="spin" /> : <RefreshCw size={16} />}{loading ? 'Fetching…' : 'Fetch MIR files'}</Button></div>
    <p className="mir-help">Date range includes both days and uses each file’s modified date on the API machine. Only .MIR files directly inside the locked folder are read.</p>
    {dateError && <p role="alert" className="field-error">{dateError}</p>}
    {error && <div className="notice notice-amber" role="alert">{error}</div>}
    {notice && <div className="notice notice-blue" role="status">{notice}</div>}
    {issues.length > 0 && <div className="mir-issues" role="status"><h3>Files requiring attention</h3><ul>{issues.map((issue, index) => <li key={index}>{issue}</li>)}</ul></div>}
    <div className="mir-records"><div className="mir-section-heading"><FileText size={19} /><h2>MIR records</h2><span className="badge badge-neutral">{records ? `${records.length} records` : 'No fetch yet'}</span></div>
      {loading ? <LoadingState /> : records?.length ? <div className="table-scroll"><table><caption className="sr-only">Choose one MIR record to populate sales entry.</caption><thead><tr>{['Select', 'Source file / date', 'Ticket number', 'Passenger', 'PNR', 'Status'].map(label => <th scope="col" key={label}>{label}</th>)}</tr></thead><tbody>{records.map(record => {
        const key = `${record.fileName}:${record.id}`
        const blocked = ['Failed', 'Duplicate'].includes(record.status)
        return <tr key={key} className={selected === key ? 'mir-selected' : ''}><td><input type="radio" name="mir-record" aria-label={`Select ${record.fileName} ${record.fields.ticketNumber?.value || record.id}`} checked={selected === key} disabled={blocked || proceeding} onChange={() => { setSelected(key); setNotice('') }} /></td><td><strong>{record.fileName}</strong><small>{record.recordDate}</small></td><td>{record.fields.ticketNumber?.value || 'Missing'}</td><td>{record.fields.passengerName?.value || 'Missing'}</td><td>{record.fields.pnr?.value || 'Missing'}</td><td><span className={`badge badge-${blocked ? 'red' : record.status === 'Extracted' ? 'teal' : 'amber'}`}>{record.status}</span>{record.issues.length > 0 && <small>{record.issues.length} source issues</small>}</td></tr>
      })}</tbody></table></div> : <EmptyState title={records ? 'No MIR files in this date range' : 'Choose a date range to fetch MIR files'} description={records ? 'Check the date range and the files in your locked folder.' : 'Select From and To dates, fetch records, then choose one ticket and Proceed.'} />}
    </div>
    <div className="mir-proceed"><p>{selectedRecord ? `Selected: ${selectedRecord.fileName} · ${selectedRecord.fields.ticketNumber?.value || 'Ticket needs review'}` : 'Select a record to continue.'}</p><Button disabled={!selectedRecord || proceeding || loading} onClick={() => void proceed()}>{proceeding ? <LoaderCircle size={16} className="spin" /> : <ArrowRight size={16} />}Proceed to sales entry</Button></div>
    <Dialog open={!!replacement} onOpenChange={open => { if (!open) setReplacement(null) }} title="Replace the unsaved sales entry?" description="The current ticket has unsaved values. Replacing it discards this unsaved preview; no posted records are changed."><div className="sales-dialog-actions"><Button variant="outline" onClick={() => setReplacement(null)}>Keep current entry</Button><Button onClick={() => { if (replacement) openRecord(replacement) }}>Replace with selected MIR</Button></div></Dialog>
  </>
}

const groups: Array<{ title: string; fields: Array<[SalesField, string]> }> = [
  { title: 'Ticket & passenger', fields: [['ticketNumber', 'Ticket number'], ['pnr', 'PNR'], ['passengerName', 'Passenger name'], ['airline', 'Airline'], ['ticketDate', 'Ticket date (source text)'], ['travelDate', 'Travel date (source text)'], ['sector', 'Sector']] },
  { title: 'Fare components from source', fields: [['basicFare', 'Basic fare'], ['basicCurrency', 'Basic fare currency'], ['mirTotalFare', 'MIR ticket fare total'], ['fareCurrency', 'Ticket fare currency']] },
  { title: 'Client accounting', fields: [['billTo', 'Bill to'], ['clientTotal', 'Client total']] },
  { title: 'Supplier accounting', fields: [['supplier', 'Supplier'], ['supplierTotal', 'Supplier total']] },
  { title: 'Notes', fields: [['narration', 'Narration']] },
]

function SalesEditor() {
  const { draft, editManual, correct } = useSalesDraft()
  const [history, setHistory] = useState<SalesField | null>(null)
  const [editing, setEditing] = useState<SalesField | null>(null)
  const [newValue, setNewValue] = useState('')
  const [reason, setReason] = useState('')
  const [sourceOpen, setSourceOpen] = useState(false)
  const source = draft.source
  return <div className="sales-editor"><div className="sales-editor-heading"><div><h2>{source ? 'Sales entry · MIR populated' : 'Manual ticket entry'}</h2><p>{source ? 'Only extracted fields are populated. Missing values remain blank for review.' : 'Enter the ticket details directly, or use MIR import to populate extracted values.'}</p></div><span className="badge badge-neutral">{source ? '1 loaded ticket' : 'Unsaved draft'}</span></div>
    {source && <div className="sales-source-strip"><span><FileText size={16} />{source.fileName}</span><span>{source.status}</span><Button size="sm" variant="outline" onClick={() => setSourceOpen(true)}>View source & issues</Button></div>}
    {groups.map(group => <section key={group.title} className="sales-field-group"><h3>{group.title}</h3><div className="sales-fields">{group.fields.map(([key, label]) => {
      const value = draft.values[key] ?? ''
      const extracted = source?.fields[key]
      const overridden = draft.corrections.some(change => change.field === key)
      const isMoney = ['basicFare', 'mirTotalFare', 'clientTotal', 'supplierTotal'].includes(key)
      const invalidMoney = isMoney && value !== '' && !/^-?\d+(\.\d+)?$/.test(value)
      return <div className="form-field" key={key}><div className="sales-field-label"><label htmlFor={`sale-${key}`}>{label}</label><button className={`badge badge-${overridden ? 'amber' : extracted ? 'blue' : 'neutral'}`} onClick={() => setHistory(key)} aria-label={`View ${label} history`}>{overridden ? 'Overridden' : extracted ? 'MIR' : 'Manual'}<History size={11} /></button></div><div className="sales-value-control"><input id={`sale-${key}`} value={value} placeholder={extracted ? '' : 'Not supplied'} readOnly={!!source} inputMode={isMoney ? 'decimal' : undefined} aria-invalid={invalidMoney} aria-describedby={invalidMoney ? `sale-${key}-error` : undefined} maxLength={key === 'narration' ? 2000 : 250} onChange={event => editManual(key, event.target.value)} />{source && <button aria-label={`Correct ${label}`} onClick={() => { setEditing(key); setNewValue(value); setReason('') }}><Pencil size={15} /></button>}</div>{invalidMoney && <p id={`sale-${key}-error`} className="field-error" role="alert">Enter an exact decimal amount. No value has been rounded or inferred.</p>}{extracted && <small>Source {extracted.lines.length === 1 ? 'line' : 'lines'} {extracted.lines.join(', ')}</small>}</div>
    })}</div></section>)}
    <p className="sales-entry-note"><LockKeyhole size={15} />Unsaved preview. Account mapping, tax verification, permissions and posting are not connected. MIR ticket fare is not treated as client revenue or supplier cost.</p>
    <Dialog open={!!editing} onOpenChange={open => { if (!open) setEditing(null) }} title="Correct extracted value" description="The original MIR value and raw source remain unchanged. Each correction keeps its previous value, new value, time and reason."><form onSubmit={event => { event.preventDefault(); if (editing && reason.trim()) { correct(editing, newValue, reason); setEditing(null) } }}><label htmlFor="correction-value">New value</label><input id="correction-value" value={newValue} maxLength={2000} onChange={event => setNewValue(event.target.value)} /><label htmlFor="correction-reason">Reason for correction</label><textarea id="correction-reason" value={reason} onChange={event => setReason(event.target.value)} maxLength={1000} required /><div className="sales-dialog-actions"><Button type="submit" disabled={!reason.trim()}><Check size={15} />Apply correction</Button></div></form></Dialog>
    <Dialog open={!!history} onOpenChange={open => { if (!open) setHistory(null) }} title="Field history" description="Original source and corrections in this unsaved entry. Persistent audit logging is enabled with database-backed saving.">{history && <><p>Original: <strong>{source?.fields[history]?.value ?? 'Not supplied'}</strong></p>{draft.corrections.filter(change => change.field === history).map((change, index) => <div className="sales-correction" key={index}><p>{change.oldValue || '(blank)'} → <strong>{change.newValue || '(blank)'}</strong></p><p>{change.reason}</p><small>{change.at} · {change.actor}</small></div>)}{!draft.corrections.some(change => change.field === history) && <p className="text-muted">No corrections in this entry.</p>}</>}</Dialog>
    <Dialog open={sourceOpen} onOpenChange={setSourceOpen} title="MIR source evidence" description="Raw lines are retained with original bytes. Unknown records and missing values are not guessed." className="sales-source-dialog">{source && <><p className="mir-source-hash">SHA-256: {source.sha256}</p><ul className="mir-issues-list">{source.issues.map((issue, index) => <li key={index}>{issue}</li>)}</ul><pre>{source.rawLines.map(line => `${line.number}: ${line.text}`).join('\n')}</pre><details><summary>Original line bytes</summary><pre>{source.rawLines.map(line => `${line.number}: ${line.rawHex} [end: ${line.terminatorHex}]`).join('\n')}</pre></details></>}</Dialog>
  </div>
}
