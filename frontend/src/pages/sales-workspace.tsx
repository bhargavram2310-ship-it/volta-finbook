import { useEffect, useState } from 'react'
import { useLocation, useSearchParams } from 'react-router-dom'
import { Check, FileText, History, LockKeyhole, Pencil, ScanLine, Undo2, CircleX } from 'lucide-react'
import { ModulePage } from './module-page'
import { GdsCapture } from './gds-capture'
import { SalesTypeSelect } from '@/components/sales-type-select'
import { allPages } from '@/lib/navigation'
import { salesClassSchema } from '@/lib/sales-types'

import { apiJSON, type SalesField } from '@/lib/mir'
import { salesLookupsSchema, salesMasterFields, type SalesLookups } from '@/lib/sales-lookups'
import { SalesMasterSelect } from '@/components/sales-master-select'
import { useSalesDraft } from '@/components/sales-draft-context'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'

import '@/sales-entry.css'

const salesTabs = [
  { mode: 'gds', label: 'GDS Capture', icon: ScanLine },
  { mode: 'manual', label: 'Entry', icon: Pencil },
  { mode: 'refund', label: 'Refund', icon: Undo2 },
  { mode: 'cancellation', label: 'Cancellation', icon: CircleX },
] as const

export function SalesWorkspace() {
  const location = useLocation()
  const [params, setParams] = useSearchParams()
  const { draft } = useSalesDraft()
  const requestedClass = salesClassSchema.safeParse(params.get('type'))
  const isMisc = (requestedClass.success ? requestedClass.data : draft.salesClass) === 'miscellaneous'
  const tabs = salesTabs.filter(tab => !isMisc || tab.mode !== 'gds')
  const mode = params.get('mode')
  const activeMode = tabs.find(tab => tab.mode === mode)?.mode ?? (isMisc ? 'manual' : 'gds')
  const page = allPages.find(p => `/app/${p.id}` === location.pathname)!
  const selectMode = (mode: typeof salesTabs[number]['mode']) => { const next = new URLSearchParams(params); next.set('mode', mode); setParams(next) }
  useEffect(() => {
    if (isMisc && ['gds', 'mir'].includes(mode ?? '')) setParams(previous => { const next = new URLSearchParams(previous); next.set('mode', 'manual'); return next }, { replace: true })
  }, [isMisc, mode, setParams])
  return <ModulePage page={page} workspaceOnly={activeMode !== 'manual'} headerControl={<SalesTypeSelect />}><section className="international-entry sales-workspace" aria-label="Sales workspace modes"><div className="sales-mode-tabs" role="tablist" aria-label="Sales activities" aria-orientation="horizontal" onKeyDown={event => {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const index = tabs.findIndex(tab => tab.mode === activeMode)
    const nextIndex = event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1 : (index + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length
    const nextMode = tabs[nextIndex].mode
    selectMode(nextMode); requestAnimationFrame(() => document.getElementById(`${nextMode}-tab`)?.focus())
  }}>
    {tabs.map(tab => <button key={tab.mode} id={`${tab.mode}-tab`} role="tab" tabIndex={activeMode === tab.mode ? 0 : -1} aria-selected={activeMode === tab.mode} aria-controls={`${tab.mode}-panel`} onClick={() => selectMode(tab.mode)}><tab.icon size={17} aria-hidden="true" />{tab.label}</button>)}
  </div>{tabs.map(tab => <div key={tab.mode} id={`${tab.mode}-panel`} role="tabpanel" aria-labelledby={`${tab.mode}-tab`} hidden={activeMode !== tab.mode} tabIndex={0}>{activeMode === tab.mode && (tab.mode === 'gds' ? <GdsCapture /> : tab.mode === 'manual' ? <SalesEditor /> : <div className="sales-activity-empty"><tab.icon size={28} aria-hidden="true" /><h2>{tab.label}</h2><p>{tab.mode === 'refund' ? 'Refund requests and their settlement status will appear here.' : 'Ticket cancellation requests and their status will appear here.'}</p><span className="badge badge-neutral">{tab.label} processing is not connected yet</span></div>)}</div>)}</section></ModulePage>
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
  const [lookups, setLookups] = useState<SalesLookups | null>(null)
  const [lookupError, setLookupError] = useState('')
  const [lookupLoading, setLookupLoading] = useState(true)
  const [lookupAttempt, setLookupAttempt] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    setLookupLoading(true); setLookupError('')
    apiJSON('/api/v1/masters/sales-lookups', { signal: controller.signal })
      .then(body => { if (!controller.signal.aborted) setLookups(salesLookupsSchema.parse(body)) })
      .catch(() => { if (!controller.signal.aborted) setLookupError('Master lists could not be loaded. Check your connection and sign-in, then try again.') })
      .finally(() => { if (!controller.signal.aborted) setLookupLoading(false) })
    return () => controller.abort()
  }, [lookupAttempt])
  const masterSelect = (key: SalesField, id: string, value: string, onChange: (value: string) => void, disabled = false) => {
    const master = salesMasterFields[key]
    return master ? <SalesMasterSelect id={id} value={value} options={lookups?.[master.list] ?? []} label={master.label} loading={lookupLoading} unavailable={!!lookupError} disabled={disabled} onChange={onChange} /> : null
  }
  const correctionMaster = editing ? salesMasterFields[editing] : undefined
  const validCorrection = !correctionMaster || (!lookupLoading && !lookupError && (!newValue || !!lookups?.[correctionMaster.list].some(option => option.id === newValue)))
  const isMisc = draft.salesClass === 'miscellaneous'
  const fieldGroups = isMisc ? [
    { title: 'Service & traveller', fields: [['serviceDescription', 'Service / description'], ['ticketNumber', 'Service reference'], ['pnr', 'Booking reference'], ['passengerName', 'Traveller / guest name'], ['ticketDate', 'Issue date'], ['travelDate', 'Service / travel date'], ['sector', 'Location / itinerary']] as Array<[SalesField, string]> },
    { title: 'Service amounts', fields: [['basicFare', 'Base amount'], ['basicCurrency', 'Base currency'], ['mirTotalFare', 'Source amount'], ['fareCurrency', 'Transaction currency']] as Array<[SalesField, string]> },
    ...groups.slice(2),
  ] : groups
  const source = draft.source
  return <div className="sales-editor"><div className="sales-editor-heading"><div><h2>{source ? 'Captured sales details' : isMisc ? 'Manual service details' : 'Manual sales details'}</h2><p>{source ? 'Only extracted fields are populated. Missing values remain blank for review.' : isMisc ? 'Enter the service details and complete client and supplier accounting.' : 'Enter ticket details manually. Review extracted tickets in GDS Capture.'}</p></div><span className="badge badge-neutral">{source ? '1 loaded ticket' : 'Unsaved draft'}</span></div>
    {lookupError && <div className="mir-error" role="alert">{lookupError} <Button size="sm" variant="outline" onClick={() => setLookupAttempt(value => value + 1)}>Retry master lists</Button></div>}
    {source && <div className="sales-source-strip"><span><FileText size={16} />{source.fileName}</span><span>{source.status}</span><Button size="sm" variant="outline" onClick={() => setSourceOpen(true)}>View source & issues</Button></div>}
    {fieldGroups.map(group => <section key={group.title} className="sales-field-group"><h3>{group.title}</h3><div className="sales-fields">{group.fields.map(([key, label]) => {
      const value = draft.values[key] ?? ''
      const extracted = source?.fields[key]
      const overridden = draft.corrections.some(change => change.field === key)
      const isMoney = ['basicFare', 'mirTotalFare', 'clientTotal', 'supplierTotal'].includes(key)
      const invalidMoney = isMoney && value !== '' && !/^-?\d+(\.\d+)?$/.test(value)
      return <div className="form-field" key={key}><div className="sales-field-label"><label htmlFor={`sale-${key}`}>{label}</label><button className={`badge badge-${overridden ? 'amber' : extracted ? 'blue' : 'neutral'}`} onClick={() => setHistory(key)} aria-label={`View ${label} history`}>{overridden ? 'Overridden' : extracted ? 'MIR' : 'Manual'}<History size={11} /></button></div><div className="sales-value-control">{salesMasterFields[key] ? masterSelect(key, `sale-${key}`, value, next => editManual(key, next), !!extracted) : <input id={`sale-${key}`} value={value} placeholder={extracted ? '' : 'Not supplied'} readOnly={!!extracted} inputMode={isMoney ? 'decimal' : undefined} aria-invalid={invalidMoney} aria-describedby={invalidMoney ? `sale-${key}-error` : undefined} maxLength={key === 'narration' ? 2000 : 250} onChange={event => editManual(key, event.target.value)} />}{extracted && <button aria-label={`Correct ${label}`} onClick={() => { setEditing(key); setNewValue(value); setReason('') }}><Pencil size={15} /></button>}</div>{invalidMoney && <p id={`sale-${key}-error`} className="field-error" role="alert">Enter an exact decimal amount. No value has been rounded or inferred.</p>}{extracted && <small>Source {extracted.lines.length === 1 ? 'line' : 'lines'} {extracted.lines.join(', ')}</small>}</div>
    })}</div></section>)}
    <p className="sales-entry-note"><LockKeyhole size={15} />Unsaved preview. Account mapping, tax verification, permissions and posting are not connected. MIR ticket fare is not treated as client revenue or supplier cost.</p>
    <Dialog open={!!editing} onOpenChange={open => { if (!open) setEditing(null) }} title="Correct extracted value" description="The original MIR value and raw source remain unchanged. Each correction keeps its previous value, new value, time and reason."><form onSubmit={event => { event.preventDefault(); if (editing && reason.trim() && validCorrection) { correct(editing, newValue, reason); setEditing(null) } }}><label htmlFor="correction-value">New value</label>{editing && correctionMaster ? masterSelect(editing, 'correction-value', newValue, setNewValue) : <input id="correction-value" value={newValue} maxLength={2000} onChange={event => setNewValue(event.target.value)} />}<label htmlFor="correction-reason">Reason for correction</label><textarea id="correction-reason" value={reason} onChange={event => setReason(event.target.value)} maxLength={1000} required /><div className="sales-dialog-actions"><Button type="submit" disabled={!reason.trim() || !validCorrection}><Check size={15} />Apply correction</Button></div></form></Dialog>
    <Dialog open={!!history} onOpenChange={open => { if (!open) setHistory(null) }} title="Field history" description="Original source and corrections in this unsaved entry. Persistent audit logging is enabled with database-backed saving.">{history && <><p>Original: <strong>{source?.fields[history]?.value ?? 'Not supplied'}</strong></p>{draft.corrections.filter(change => change.field === history).map((change, index) => <div className="sales-correction" key={index}><p>{change.oldValue || '(blank)'} → <strong>{change.newValue || '(blank)'}</strong></p><p>{change.reason}</p><small>{change.at} · {change.actor}</small></div>)}{!draft.corrections.some(change => change.field === history) && <p className="text-muted">No corrections in this entry.</p>}</>}</Dialog>
    <Dialog open={sourceOpen} onOpenChange={setSourceOpen} title="MIR source evidence" description="Raw lines are retained with original bytes. Unknown records and missing values are not guessed." className="sales-source-dialog">{source && <><p className="mir-source-hash">SHA-256: {source.sha256}</p><ul className="mir-issues-list">{source.issues.map((issue, index) => <li key={index}>{issue}</li>)}</ul><pre>{source.rawLines.map(line => `${line.number}: ${line.text}`).join('\n')}</pre><details><summary>Original line bytes</summary><pre>{source.rawLines.map(line => `${line.number}: ${line.rawHex} [end: ${line.terminatorHex}]`).join('\n')}</pre></details></>}</Dialog>
  </div>
}
