import { useEffect, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { ArrowRight, LoaderCircle, RotateCcw, Search, Settings2 } from 'lucide-react'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { GdsTicketDetails } from '@/components/gds-ticket-details'
import { useSalesDraft } from '@/components/sales-draft-context'
import { apiJSON, mirRecordSchema, type MirRecord, type SalesField } from '@/lib/mir'
import { salesLookupsSchema, type SalesLookups } from '@/lib/sales-lookups'
import { captureColumns, captureRowSchema, captureSettingsSchema, loadCaptureColumns, saveCaptureColumns, type CaptureColumn, type CaptureRow } from '@/lib/gds-capture'
import './gds-capture.css'

interface Filter { from_date: string; to_date: string; date_basis: 'processing' | 'issue'; search: string }
const emptyFilter = (today = ''): Filter => ({ from_date: today, to_date: today, date_basis: 'processing', search: '' })
const errorText = (error: unknown) => error instanceof z.ZodError ? 'Ticket data could not be read. Refresh and try again.' : error instanceof Error ? (error.message === 'gds_capture_unavailable' ? 'Tickets could not be loaded. Check your connection and try again.' : error.message.replaceAll('_', ' ')) : 'The request failed. Please try again.'

export function GdsCapture() {
  const { draft, load } = useSalesDraft()
  const [, setParams] = useSearchParams()
  const [settings, setSettings] = useState<z.infer<typeof captureSettingsSchema> | null>(null)
  const [settingsError, setSettingsError] = useState('')
  const [settingsAttempt, setSettingsAttempt] = useState(0)
  const [form, setForm] = useState<Filter>(() => emptyFilter())
  const [filter, setFilter] = useState<Filter | null>(null)
  const [page, setPage] = useState(1)
  const [attempt, setAttempt] = useState(0)
  const [records, setRecords] = useState<CaptureRow[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [searched, setSearched] = useState(false)
  const [error, setError] = useState('')
  const [formError, setFormError] = useState('')
  const [actionError, setActionError] = useState('')
  const [visible, setVisible] = useState(loadCaptureColumns)
  const [selected, setSelected] = useState<CaptureRow | null>(null)
  const [proceeding, setProceeding] = useState(false)
  const [pending, setPending] = useState<{ source: MirRecord; mapped: Partial<Record<SalesField, string>> } | null>(null)
  const [detail, setDetail] = useState<{ row: CaptureRow; mode: 'ticket' | 'sale' } | null>(null)
  const proceedController = useRef<AbortController | null>(null)
  useEffect(() => () => proceedController.current?.abort(), [])
  useEffect(() => { saveCaptureColumns(visible) }, [visible])
  useEffect(() => {
    const controller = new AbortController()
    setSettingsError('')
    apiJSON('/api/v1/mir/capture/settings', { signal: controller.signal }).then(body => {
      const result = captureSettingsSchema.parse(body)
      new Intl.DateTimeFormat('en', { timeZone: result.timezone }).format(new Date())
      if (!controller.signal.aborted) { setSettings(result); setForm(previous => ({ ...previous, from_date: previous.from_date || result.today, to_date: previous.to_date || result.today })) }
    }).catch(error => { if (!controller.signal.aborted) setSettingsError(errorText(error)) })
    return () => controller.abort()
  }, [settingsAttempt])
  useEffect(() => {
    if (!filter) return
    const controller = new AbortController()
    setLoading(true); setError(''); setActionError(''); setRecords([]); setSearched(false); setSelected(null); setDetail(null)
    const query = new URLSearchParams({ ...filter, page: String(page), page_size: '25' })
    apiJSON(`/api/v1/mir/capture?${query}`, { signal: controller.signal }).then(body => {
      if (controller.signal.aborted) return
      const result = z.object({ records: z.array(captureRowSchema), total: z.number(), timezone: z.string() }).parse(body)
      if (settings && result.timezone !== settings.timezone) throw new Error('The business timezone changed. Refresh the page before searching again.')
      setRecords(result.records); setTotal(result.total); setSearched(true)
    }).catch(error => { if (!controller.signal.aborted) setError(errorText(error)) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [filter, page, attempt, settings])
  const shown = captureColumns.filter(([key]) => visible.includes(key))
  function openManual(source: MirRecord, mapped: Partial<Record<SalesField, string>>) {
    load(source, mapped); setDetail(null); setPending(null)
    setParams(previous => { const next = new URLSearchParams(previous); next.set('mode', 'manual'); return next })
    requestAnimationFrame(() => document.getElementById('manual-tab')?.focus())
  }
  async function proceed(row = selected) {
    if (!row || !row.progress.can_prepare_draft || row.progress.linked_sale || loading || proceeding) return
    const controller = new AbortController(); proceedController.current = controller
    setProceeding(true); setActionError('')
    try {
      const [sourceBody, masterBody] = await Promise.all([
        apiJSON(`/api/v1/mir/capture/${row.passenger_id}/draft?record_id=${row.record_id}`, { signal: controller.signal }),
        apiJSON('/api/v1/masters/sales-lookups', { signal: controller.signal }).catch(() => null),
      ])
      if (controller.signal.aborted) return
      const source = mirRecordSchema.parse(sourceBody)
      const masters = salesLookupsSchema.safeParse(masterBody)
      const mapped: Partial<Record<SalesField, string>> = {}
      if (masters.success) {
        const fields: Array<[SalesField, keyof SalesLookups]> = [['airline', 'airlines'], ['basicCurrency', 'currencies'], ['fareCurrency', 'currencies']]
        for (const [field, list] of fields) {
          const sourceValue = source.fields[field]?.value.trim().toLowerCase()
          if (!sourceValue) continue
          const matches = masters.data[list].filter(option => option.code.toLowerCase() === sourceValue || option.name.toLowerCase() === sourceValue)
          if (matches.length === 1) mapped[field] = matches[0].id
        }
      }
      if (draft.source?.id !== source.id && Object.values(draft.values).some(value => value?.trim())) { setDetail(null); setPending({ source, mapped }) }
      else openManual(source, mapped)
    } catch (error) { if (!controller.signal.aborted) setActionError(errorText(error)) }
    finally { if (!controller.signal.aborted) setProceeding(false) }
  }
  function review(row: CaptureRow, mode: 'ticket' | 'sale' = 'ticket') { setActionError(''); setDetail({ row, mode }) }
  function reset() { setForm(emptyFilter(settings?.today)); setFilter(null); setPage(1); setRecords([]); setTotal(0); setSearched(false); setLoading(false); setError(''); setFormError(''); setActionError(''); setSelected(null); setDetail(null) }
  function display(row: CaptureRow, key: CaptureColumn) {
    if (key === 'fare') return row.fare ? <span className="capture-amount">{row.currency || 'Currency missing'} {row.fare}<small>Imported fare</small></span> : '—'
    if (key === 'issue_date') return row.issue_date || row.issue_date_source || '—'
    if (key === 'imported_at') return row.imported_at && settings ? new Intl.DateTimeFormat('en-GB', { timeZone: settings.timezone, dateStyle: 'medium', timeStyle: 'short' }).format(new Date(row.imported_at)) : '—'
    if (key === 'ticket_number') return <button className="capture-ticket-link" onClick={() => review(row)}>{row.ticket_number || 'Review missing ticket number'}</button>
    return row[key] || '—'
  }
  function rowAction(row: CaptureRow) {
    if (row.progress.linked_sale) return <Button variant="outline" size="sm" disabled={!row.progress.can_view_sale || proceeding} title={!row.progress.can_view_sale ? 'Your role needs permission to view sales records.' : undefined} onClick={() => review(row, 'sale')}>View Sale</Button>
    return <Button variant={row.progress.status === 'Ready for Sale' ? 'default' : 'outline'} size="sm" disabled={proceeding} onClick={() => row.progress.status === 'Ready for Sale' ? void proceed(row) : review(row)}>{row.progress.status === 'Ready for Sale' ? 'Create Sale' : 'Review'}</Button>
  }
  return <section className="gds-capture" aria-labelledby="capture-title">
    <div className="capture-heading"><div><h2 id="capture-title">GDS Capture</h2><p>Search imported tickets, review details, and prepare a sales draft.</p></div><Button asChild variant="outline" size="sm"><Link to="/app/administration/mir-report">MIR Import Report</Link></Button></div>
    {settingsError && <div role="alert" className="notice notice-amber">{settingsError}<Button size="sm" variant="outline" onClick={() => setSettingsAttempt(value => value + 1)}>Retry filter settings</Button></div>}
    <form className="capture-filters" onSubmit={event => { event.preventDefault(); if (!settings) return; if (!form.from_date || !form.to_date || form.from_date > form.to_date) { setFormError('From Date must be on or before To Date.'); return }; setFormError(''); setFilter({ ...form }); setPage(1) }}>
      <label>Filter by<select aria-label="Filter by" aria-describedby="capture-date-help" value={form.date_basis} onChange={event => setForm(previous => ({ ...previous, date_basis: event.target.value as Filter['date_basis'] }))}><option value="processing">Imported Date</option><option value="issue">Ticket Issue Date</option></select></label>
      <label>From Date<input type="date" required aria-invalid={!!formError} aria-describedby={formError ? 'capture-filter-error' : undefined} value={form.from_date} onChange={event => { setFormError(''); setForm(previous => ({ ...previous, from_date: event.target.value })) }} /></label>
      <label>To Date<input type="date" required aria-invalid={!!formError} aria-describedby={formError ? 'capture-filter-error' : undefined} value={form.to_date} onChange={event => { setFormError(''); setForm(previous => ({ ...previous, to_date: event.target.value })) }} /></label>
      <label className="capture-search">Search<input type="search" placeholder="Ticket, PNR or passenger" maxLength={255} value={form.search} onChange={event => setForm(previous => ({ ...previous, search: event.target.value }))} /></label>
      <div className="capture-filter-actions"><Button type="submit" disabled={!settings || loading || proceeding}>{loading ? <LoaderCircle size={16} className="spin" aria-hidden="true" /> : <Search size={16} aria-hidden="true" />}Search Tickets</Button><Button variant="outline" disabled={proceeding} onClick={reset}><RotateCcw size={16} aria-hidden="true" />Reset</Button></div>
    </form>
    {formError && <p id="capture-filter-error" className="field-error" role="alert">{formError}</p>}
    <p id="capture-date-help" className="capture-help">{settings ? <>Business timezone: <strong>{settings.timezone}</strong>. {form.date_basis === 'processing' ? 'Imported Date is when the ticket was first extracted. Both dates are included.' : 'Ticket Issue Date is printed on the ticket and may be earlier than its import date.'}</> : settingsError ? 'Date filtering is unavailable until the timezone settings can be loaded.' : 'Loading business timezone…'}</p>
    <div className="capture-grid-heading"><p role="status" aria-live="polite">{loading ? 'Searching tickets…' : error ? 'Search failed.' : searched ? `${total} matching ${total === 1 ? 'ticket' : 'tickets'}` : 'Select a date range and search for tickets.'}</p><details className="capture-columns"><summary><Settings2 size={16} aria-hidden="true" />Visible Columns</summary><div>{captureColumns.map(([key, label]) => <label key={key}><input type="checkbox" checked={visible.includes(key)} disabled={key === 'ticket_number'} onChange={event => setVisible(previous => event.target.checked ? [...previous, key] : previous.filter(column => column !== key))} />{label}</label>)}</div></details></div>
    {error && <div className="notice notice-amber" role="alert"><span>{error === 'authentication required' ? 'Sign in to view imported tickets.' : error}</span><Button size="sm" variant="outline" onClick={() => setAttempt(value => value + 1)}>Retry search</Button>{error === 'authentication required' && <Link to="/login?returnTo=%2Fapp%2Fsales%2Fworkspace%3Fmode%3Dgds">Sign in</Link>}</div>}
    {actionError && !detail && <div className="notice notice-amber" role="alert">{actionError}<Button size="sm" variant="outline" onClick={() => setAttempt(value => value + 1)}>Refresh tickets</Button></div>}
    <div className="table-scroll capture-table-scroll" role="region" aria-label="Ticket results" tabIndex={0} aria-busy={loading}><table><caption className="sr-only">Imported tickets. Amount is imported fare, not the final sale amount.</caption><thead><tr><th scope="col">Selection</th>{shown.map(([key, label]) => <th key={key} scope="col" title={key === 'fare' ? 'Imported fare; review the final sale amount in Entry.' : undefined}>{label}</th>)}<th scope="col">Status</th><th scope="col">Actions</th></tr></thead><tbody>{records.map(row => <tr key={`${row.record_id}:${row.passenger_id}`} className={selected?.passenger_id === row.passenger_id ? 'capture-selected' : undefined}><td><label className="capture-select"><input type="radio" name="capture-ticket" aria-label={`Select ticket ${row.ticket_number || row.passenger_name || row.passenger_id}`} checked={selected?.passenger_id === row.passenger_id} disabled={proceeding || !row.progress.can_prepare_draft || !!row.progress.linked_sale} onChange={() => { setSelected(row); setActionError('') }} /></label></td>{shown.map(([key]) => <td key={key}>{display(row, key)}</td>)}<td><span className={`badge badge-${row.progress.status === 'Sale Created' ? 'blue' : row.progress.status === 'Ready for Sale' ? 'teal' : 'amber'}`}>{row.progress.status}</span>{row.progress.issues.length > 0 && <small className="capture-row-issue">{row.progress.issues[0]}{row.progress.issues.length > 1 ? ` (+${row.progress.issues.length - 1})` : ''}</small>}</td><td>{rowAction(row)}</td></tr>)}{!records.length && <tr><td colSpan={shown.length + 3} className="capture-empty">{loading ? <><LoaderCircle size={20} className="spin" aria-hidden="true" />Loading tickets…</> : error ? 'Tickets could not be loaded. Use Retry search above.' : !filter ? 'Select a date range and search for tickets.' : 'No tickets found for the selected filters.'}{searched && total === 0 && filter?.date_basis === 'issue' && <><br /><Button variant="outline" size="sm" onClick={() => { const next: Filter = { ...filter, date_basis: 'processing' }; setForm(next); setFilter(next); setPage(1) }}>Search by Imported Date</Button></>}</td></tr>}</tbody></table></div>
    <div className="capture-proceed"><p>{selected ? `Selected: ${selected.ticket_number || selected.passenger_name}. Review the draft before saving.` : 'Select one ticket to prepare a sales draft.'}</p><Button disabled={!selected || loading || proceeding || !selected.progress.can_prepare_draft || !!selected.progress.linked_sale} onClick={() => void proceed()}>{proceeding ? 'Loading ticket…' : selected?.progress.status === 'Ready for Sale' ? 'Create Sale' : 'Open sales draft'}<ArrowRight size={16} aria-hidden="true" /></Button></div>
    <p className="capture-help">Ready for Sale means the imported ticket is ready to open as a draft. Customer mapping and commercial amounts still need review in Entry. Existing sales cannot be created again.</p>
    {searched && filter && <div className="capture-pagination"><span>{total ? `${(page - 1) * 25 + 1}–${Math.min(page * 25, total)} of ${total}` : '0 tickets'}</span><Button variant="outline" size="sm" disabled={page === 1 || loading || proceeding} onClick={() => setPage(value => value - 1)}>Previous</Button><span>Page {page} of {Math.max(1, Math.ceil(total / 25))}</span><Button variant="outline" size="sm" disabled={page * 25 >= total || loading || proceeding} onClick={() => setPage(value => value + 1)}>Next</Button></div>}
    {detail && settings && <GdsTicketDetails key={`${detail.row.record_id}:${detail.row.passenger_id}:${detail.mode}`} row={detail.row} initialMode={detail.mode} timezone={settings.timezone} busy={proceeding} actionError={actionError} onClose={() => { setDetail(null); setActionError('') }} onPrepare={row => void proceed(row)} onRefresh={row => { setRecords(previous => previous.map(item => item.passenger_id === row.passenger_id ? row : item)); setSelected(previous => previous?.passenger_id === row.passenger_id ? row : previous) }} />}
    <Dialog open={!!pending} onOpenChange={open => { if (!open) setPending(null) }} title="Replace current draft?" description="Your current unsaved entry will be replaced with the selected ticket. Its unsaved changes will be discarded."><div className="sales-dialog-actions"><Button variant="outline" onClick={() => setPending(null)}>Keep current draft</Button><Button onClick={() => { if (pending) openManual(pending.source, pending.mapped) }}>Replace and open draft</Button></div></Dialog>
  </section>
}
