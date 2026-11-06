import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { z } from 'zod'
import { apiJSON } from '@/lib/mir'
import { captureDetailsSchema, captureSaleSchema, type CaptureRow } from '@/lib/gds-capture'
import { Dialog } from './ui/dialog'
import { Button } from './ui/button'

export function GdsTicketDetails({ row, initialMode, timezone, busy, actionError, onClose, onPrepare, onRefresh }: {
  row: CaptureRow; initialMode: 'ticket' | 'sale'; timezone: string; busy: boolean; actionError: string;
  onClose: () => void; onPrepare: (row: CaptureRow) => void; onRefresh: (row: CaptureRow) => void;
}) {
  const [mode, setMode] = useState(initialMode)
  const [details, setDetails] = useState<z.infer<typeof captureDetailsSchema> | null>(null)
  const [sale, setSale] = useState<z.infer<typeof captureSaleSchema> | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  const refreshCallback = useRef(onRefresh)
  useEffect(() => { refreshCallback.current = onRefresh }, [onRefresh])
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setError(''); setDetails(null); setSale(null)
    apiJSON(`/api/v1/mir/capture/${row.passenger_id}/${mode === 'sale' ? 'sale' : 'details'}?record_id=${row.record_id}`, { signal: controller.signal }).then(body => {
      if (controller.signal.aborted) return
      if (mode === 'sale') setSale(captureSaleSchema.parse(body))
      else { const detail = captureDetailsSchema.parse(body); setDetails(detail); refreshCallback.current(detail.ticket) }
    }).catch(error => { if (!controller.signal.aborted) setError(error instanceof z.ZodError ? 'Ticket details could not be read. Refresh and try again.' : error instanceof Error ? error.message : 'Ticket details could not be loaded. Try again.') })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [row.record_id, row.passenger_id, mode, attempt])
  const current = details?.ticket ?? row
  return <Dialog open onOpenChange={open => { if (!open) onClose() }} title={mode === 'sale' ? 'Linked sale' : `Ticket details · ${row.ticket_number || 'Missing ticket number'}`} description={mode === 'sale' ? 'Saved sale details. Existing approval and posting controls remain in effect.' : 'Review the passenger, itinerary, imported fare and validation issues before preparing a draft.'} className="capture-detail-dialog">
    {loading && <p role="status">Loading {mode === 'sale' ? 'sale' : 'ticket'} details…</p>}
    {error && <div role="alert" className="notice notice-amber">{error}<Button size="sm" variant="outline" onClick={() => setAttempt(value => value + 1)}>Retry details</Button></div>}
    {actionError && <p className="notice notice-amber" role="alert">{actionError}</p>}
    {details && <>
      <span className={`badge badge-${current.progress.status === 'Needs Review' ? 'amber' : 'blue'}`}>{current.progress.status}</span>
      <dl className="capture-detail-grid">{[
        ['Ticket number', current.ticket_number], ['Passenger', current.passenger_name], ['PNR', current.pnr], ['GDS', current.gds], ['Route', current.sector],
        ['Issue date', current.issue_date || current.issue_date_source], ['Staff', current.staff], ['Branch', current.branch], ['Client reference', current.client_reference],
        ['Invoice number', current.invoice_number], ['Web reference', current.web_reference], ['Imported date', current.imported_at ? `${new Intl.DateTimeFormat('en-GB', { timeZone: timezone, dateStyle: 'medium', timeStyle: 'short' }).format(new Date(current.imported_at))} (${timezone})` : ''],
      ].map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value || 'Not supplied'}</dd></div>)}</dl>
      <section className="capture-detail-section"><h3>Fare and taxes from source</h3><dl className="capture-detail-grid"><div><dt>Basic fare</dt><dd>{details.draft?.fields.basicCurrency?.value || ''} {details.draft?.fields.basicFare?.value || 'Not supplied'}</dd></div><div><dt>Imported ticket fare</dt><dd>{current.currency || 'Currency missing'} {current.fare || 'Not supplied'}</dd></div></dl>{details.taxes.length ? <ul>{details.taxes.map((tax, index) => <li key={index}>{tax.code || 'Tax code not supplied'}: {tax.currency || 'Currency not supplied'} {tax.amount}</li>)}</ul> : <p>No separate tax breakdown was extracted for this ticket.</p>}<p>{details.accounting_note}</p></section>
      <section className="capture-detail-section"><h3>Customer mapping</h3><p>{details.customer_mapping}</p></section>
      <section className="capture-detail-section"><h3>Validation issues</h3>{current.progress.issues.length ? <ul>{current.progress.issues.map((issue, index) => <li key={index}>{issue}</li>)}</ul> : <p>No imported-ticket validation issues found. Complete the accounting review in Entry.</p>}{details.draft_error && <p>{details.draft_error}</p>}</section>
      <section className="capture-detail-section"><h3>Linked sale</h3>{current.progress.linked_sale ? <><p>{current.progress.linked_sale.number || `Sale ${current.progress.linked_sale.id}`} · {current.progress.linked_sale.status}</p><Button size="sm" variant="outline" disabled={!current.progress.can_view_sale} onClick={() => setMode('sale')}>View Sale</Button>{!current.progress.can_view_sale && <p>Your role does not have permission to view sales records.</p>}</> : <p>No existing sale is linked to this ticket.</p>}</section>
      <div className="capture-detail-actions"><Button asChild variant="outline"><Link to="/app/administration/mir-report">MIR Import Report</Link></Button>{!current.progress.linked_sale && <Button disabled={busy || !current.progress.can_prepare_draft || !details.draft} onClick={() => onPrepare(current)}>{current.progress.status === 'Ready for Sale' ? 'Create Sale' : 'Open sales draft'}</Button>}</div>
    </>}
    {sale && <><dl className="capture-detail-grid">{[['Document', sale.sale.number || sale.sale.id], ['Status', sale.sale.status], ['Ticket', sale.ticket_number], ['Document date', sale.document_date], ['Currency', sale.currency], ['Customer', sale.customer || 'Not mapped'], ['Narration', sale.narration || 'Not supplied']].map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl><Button variant="outline" onClick={() => setMode('ticket')}>Back to ticket details</Button></>}
  </Dialog>
}
