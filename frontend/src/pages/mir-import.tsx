import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { FileUp, RefreshCw, Search } from 'lucide-react'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { apiJSON } from '@/lib/mir'
import './mir-import.css'

const recordSchema = z.object({
  id: z.number(), source_file_id: z.number(), batch_id: z.number(), mir_date: z.string(), first_travel_date: z.string(),
  locator: z.string(), mir_type: z.string(), passenger: z.string(), ticket_number: z.string(), currency: z.string(), fare_amount: z.string(),
  status: z.string(), filename: z.string(), parser_version: z.string(),
})
const issueSchema = z.object({ Severity: z.string(), Code: z.string(), Message: z.string() })
const batchSchema = z.object({ id: z.number(), status: z.string(), imported_count: z.number(), duplicate_count: z.number(), failed_count: z.number(), created_at: z.string(), errors: z.array(z.object({ filename: z.string(), record: z.number(), message: z.string() })) })
const detailSchema = recordSchema.extend({ header: z.record(z.string(), z.string()), typed: z.record(z.string(), z.array(z.record(z.string(), z.unknown()))), relationships: z.array(z.record(z.string(), z.unknown())), sections: z.array(z.record(z.string(), z.unknown())), raw: z.string(), raw_hex: z.string(), issues: z.array(issueSchema), coverage: z.array(z.object({ Section: z.string(), Status: z.string(), Reason: z.string() })) })
type StagedRecord = z.infer<typeof recordSchema>
type Batch = z.infer<typeof batchSchema>
type Detail = z.infer<typeof detailSchema>
const panels = ['Header', 'Passengers', 'Flight / Rail Segments', 'Fare and Fare Basis', 'Fare Construction', 'Taxes and Fees', 'Payments', 'Ticket Exchange', 'Refunds', 'EMD', 'Auxiliary Segments', 'Contact Details', 'Remarks', 'SSR / OSI', 'All Sections', 'Raw MIR', 'Validation Issues']
const typedPanels: Record<string, string[]> = { Passengers: ['passenger', 'frequent_flyer'], 'Flight / Rail Segments': ['segment'], 'Fare and Fare Basis': ['fare', 'fare_basis'], 'Fare Construction': ['fare_construction'], 'Taxes and Fees': ['fare_tax', 'carrier_fee'], Payments: ['payment'], 'Ticket Exchange': ['ticket_exchange'], Refunds: ['refund'], EMD: ['emd', 'emd_coupon'], 'Auxiliary Segments': ['auxiliary_segment'], 'Contact Details': ['phone', 'address'], Remarks: ['remark'], 'SSR / OSI': ['ssr_osi'] }
const title = (text: string) => text.replaceAll('_', ' ').replace(/^\w/, c => c.toUpperCase())
const errorText = (error: unknown) => error instanceof Error ? error.message.replaceAll('_', ' ') : 'MIR request failed. Retry the request.'
const display = (value: unknown) => value === '' || value == null ? '—' : typeof value === 'object' ? JSON.stringify(value) : String(value)

export function MirImport() {
  const [form, setForm] = useState({ from_date: '', to_date: '', status: '', search: '' })
  const [filter, setFilter] = useState(form)
  const [page, setPage] = useState(1)
  const [records, setRecords] = useState<StagedRecord[]>([])
  const [total, setTotal] = useState(0)
  const [batches, setBatches] = useState<Batch[]>([])
  const [batchPage, setBatchPage] = useState(1)
  const [selectedBatch, setSelectedBatch] = useState<Batch | null>(null)
  const [detail, setDetail] = useState<Detail | null>(null)
  const [panel, setPanel] = useState('Header')
  const [files, setFiles] = useState<File[]>([])
  const [busy, setBusy] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const upload = useRef<HTMLInputElement>(null)
  const generation = useRef(0)
  const detailGeneration = useRef(0)
  const load = useCallback(async () => {
    const current = ++generation.current
    setLoading(true); setError('')
    const params = new URLSearchParams({ page: String(page), page_size: '25' })
    Object.entries(filter).forEach(([key, value]) => { if (value) params.set(key, value) })
    try {
      const [list, history] = await Promise.all([apiJSON(`/api/v1/mir/records?${params}`), apiJSON(`/api/v1/mir/batches?page=${batchPage}`)])
      if (current !== generation.current) return
      const result = z.object({ records: z.array(recordSchema), total: z.number() }).parse(list)
      setRecords(result.records); setTotal(result.total)
      setBatches(z.object({ batches: z.array(batchSchema) }).parse(history).batches)
    } catch (e) { if (current === generation.current) { setError(errorText(e)); setRecords([]); setBatches([]); setTotal(0) } }
    finally { if (current === generation.current) setLoading(false) }
  }, [filter, page, batchPage])
  useEffect(() => { void load(); return () => { generation.current++ } }, [load])
  useEffect(() => () => { detailGeneration.current++ }, [])
  async function importFiles() {
    setBusy(true); setError(''); setNotice('')
    try {
      const body = new FormData(); files.forEach(file => body.append('files', file))
      const result = batchSchema.parse(await apiJSON('/api/v1/mir/import', { method: 'POST', body }))
      setNotice(`Batch ${result.id}: ${result.imported_count} staged, ${result.duplicate_count} duplicate, ${result.failed_count} failed. Review batch errors for details.`)
      setSelectedBatch(result); setFiles([]); if (upload.current) upload.current.value = ''
      await load()
    } catch (e) { setError(errorText(e)) } finally { setBusy(false) }
  }
  async function pullFiles() {
    setBusy(true); setError(''); setNotice('')
    try {
      const result = batchSchema.parse(await apiJSON('/api/v1/mir/pull', { method: 'POST' }))
      setNotice(`Batch ${result.id}: ${result.imported_count} extracted, ${result.duplicate_count} duplicate, ${result.failed_count} failed.`)
      setSelectedBatch(result); await load()
    } catch (error) { setError(errorText(error)) } finally { setBusy(false) }
  }
  async function openRecord(id: number, reprocess = false) {
    const current = ++detailGeneration.current
    setBusy(true); setError('')
    try {
      const response = await apiJSON(`/api/v1/mir/records/${id}${reprocess ? '/reprocess' : ''}`, reprocess ? { method: 'POST' } : undefined)
      if (current !== detailGeneration.current) return
      setDetail(detailSchema.parse(response)); if (!reprocess) setPanel('Header')
      if (reprocess) { setNotice('Record reprocessed. Source file retained.'); await load() }
    } catch (e) { if (current === detailGeneration.current) setError(errorText(e)) } finally { if (current === detailGeneration.current) setBusy(false) }
  }
  return <section className="mir-stage" aria-labelledby="mir-title">
    <header className="mir-stage-heading"><div><span className="badge badge-neutral">ADMIN CONTROL</span><h1 id="mir-title">MIR Report</h1><p>Manually pull files from the configured folder or upload MIR files to extract and review their data.</p></div><Button variant="outline" disabled={loading || busy} onClick={() => void load()}><RefreshCw size={16} />Refresh</Button></header>
    <form className="mir-stage-filters" onSubmit={e => { e.preventDefault(); setFilter({ ...form }); setPage(1) }}>
      <label>From Date<input type="date" value={form.from_date} onChange={e => setForm({ ...form, from_date: e.target.value })} /></label>
      <label>To Date<input type="date" min={form.from_date || undefined} value={form.to_date} onChange={e => setForm({ ...form, to_date: e.target.value })} /></label>
      <label>Status<select value={form.status} onChange={e => setForm({ ...form, status: e.target.value })}><option value="">All statuses</option><option value="staged">Staged</option><option value="review">Review required</option><option value="failed">Failed</option></select></label>
      <label className="mir-stage-search">Search<input type="search" value={form.search} onChange={e => setForm({ ...form, search: e.target.value })} placeholder="Locator, passenger, ticket or filename" maxLength={255} /></label>
      <Button variant="outline" type="submit" disabled={loading || busy}><Search size={16} />Apply filters</Button>
    </form><p className="mir-stage-help">Date filters use MIR creation date (T50DTE), inclusive. First Travel Date is shown separately. Search matches the start of a value.</p>
    <div className="mir-stage-upload"><Button disabled={busy || loading} onClick={() => void pullFiles()}>{busy ? 'Working...' : 'Pull from configured folder'}</Button><Link to="/app/administration/mir-source">Source folder settings</Link><label htmlFor="mir-upload"><FileUp size={18} />Upload MIR Files</label><input ref={upload} id="mir-upload" type="file" multiple accept=".mir,.MIR" disabled={busy} onChange={e => { setError(''); const selected = Array.from(e.target.files ?? []); if (selected.length > 100 || selected.some(f => f.size === 0 || f.size > 2 * 1024 * 1024) || selected.reduce((sum, f) => sum + f.size, 0) > 30 * 1024 * 1024) { setError('Select up to 100 nonempty files, 2 MiB each, and 30 MiB total.'); setFiles([]); e.target.value = ''; return } setFiles(selected) }} /><Button disabled={!files.length || busy || loading} onClick={() => void importFiles()}>{busy ? 'Working…' : `Import${files.length ? ` (${files.length})` : ''}`}</Button></div>
    {error && <p role="alert" className="field-error">{error === 'authentication required' ? <>Sign in to extract and review MIR data. <Link to="/login?returnTo=%2Fapp%2Fadministration%2Fmir-report">Sign in</Link></> : error}</p>}{notice && <p role="status" className="mir-stage-notice">{notice}</p>}
    <section aria-labelledby="mir-records-title"><h2 id="mir-records-title">Imported MIR records <span className="badge badge-neutral">{total}</span></h2>{loading ? <p role="status">Loading staged MIR records…</p> : <div className="table-scroll"><table><caption className="sr-only">MIR staging records, filtered by creation date</caption><thead><tr>{['MIR Date', 'Record Locator', 'MIR Type', 'Passenger', 'Ticket / Document Number', 'First Travel Date', 'Currency', 'Fare Amount', 'Import Status', 'Source Filename'].map(c => <th key={c} scope="col">{c}</th>)}</tr></thead><tbody>{records.map(r => <tr key={r.id}><td>{r.mir_date || 'Unresolved'}</td><td><button className="mir-record-link" disabled={busy} onClick={() => void openRecord(r.id)}>{r.locator || `Record ${r.id}`}</button></td><td>{r.mir_type || '—'}</td><td>{r.passenger || '—'}</td><td>{r.ticket_number || '—'}</td><td>{r.first_travel_date || '—'}</td><td>{r.currency || '—'}</td><td>{r.fare_amount || '—'}</td><td>{title(r.status)}</td><td>{r.filename}</td></tr>)}{!records.length && <tr><td colSpan={10}>No staged records match these filters. Upload MIR files or adjust the date range.</td></tr>}</tbody></table></div>}
      <div className="mir-stage-pagination"><Button variant="outline" disabled={page === 1 || loading || busy} onClick={() => setPage(p => p - 1)}>Previous</Button><span>Page {page} · {total} records</span><Button variant="outline" disabled={page * 25 >= total || loading || busy} onClick={() => setPage(p => p + 1)}>Next</Button></div>
    </section>
    <section aria-labelledby="mir-batches-title"><h2 id="mir-batches-title">Import batch history</h2><div className="table-scroll"><table><thead><tr>{['Batch', 'Imported at (UTC)', 'Status', 'Staged', 'Duplicates', 'Failed'].map(c => <th key={c} scope="col">{c}</th>)}</tr></thead><tbody>{batches.map(b => <tr key={b.id}><td><button className="mir-record-link" onClick={() => { void apiJSON(`/api/v1/mir/batches/${b.id}`).then(value => setSelectedBatch(batchSchema.parse(value))).catch(e => setError(errorText(e))) }}>Batch {b.id}</button></td><td>{b.created_at}</td><td>{title(b.status)}</td><td>{b.imported_count}</td><td>{b.duplicate_count}</td><td>{b.failed_count}</td></tr>)}{!batches.length && <tr><td colSpan={6}>No import batches available.</td></tr>}</tbody></table></div><div className="mir-stage-pagination"><Button variant="outline" disabled={batchPage === 1 || loading || busy} onClick={() => setBatchPage(p => p - 1)}>Previous batches</Button><span>Page {batchPage}</span><Button variant="outline" disabled={batches.length < 50 || loading || busy} onClick={() => setBatchPage(p => p + 1)}>Next batches</Button></div></section>
    <Dialog open={!!selectedBatch} onOpenChange={open => { if (!open) setSelectedBatch(null) }} title={`Import batch ${selectedBatch?.id ?? ''}`} description="Batch statistics and record-level errors.">{selectedBatch && <><p>{selectedBatch.imported_count} staged · {selectedBatch.duplicate_count} duplicate · {selectedBatch.failed_count} failed</p>{selectedBatch.errors.length ? <ul>{selectedBatch.errors.map((e, i) => <li key={i}>{e.filename} · Record {e.record || 'file'}: {errorText(new Error(e.message))}</li>)}</ul> : <p>No batch errors.</p>}</>}</Dialog>
    <Dialog open={!!detail} onOpenChange={open => { if (!open) { detailGeneration.current++; setBusy(false); setDetail(null) } }} title={`MIR ${detail?.locator || detail?.id || ''}`} description="Staged source evidence and decoded details. Unresolved fields remain available in All Sections and Raw MIR." className="mir-detail-dialog">
      {detail && <><div className="mir-detail-meta"><span>{detail.filename} · {title(detail.status)} · {detail.parser_version}</span><Button variant="outline" disabled={busy} onClick={() => void openRecord(detail.id, true)}>{busy ? 'Reprocessing…' : 'Reprocess record'}</Button></div>{notice && <p role="status">{notice}</p>}{error && <p role="alert" className="field-error">{error}</p>}<label className="mir-detail-select">Detail section<select value={panel} onChange={e => setPanel(e.target.value)}>{panels.map(p => <option key={p}>{p}</option>)}</select></label><section aria-label={panel}><h2>{panel}</h2>
      {panel === 'Header' && <dl className="mir-header-fields">{Object.entries(detail.header).map(([key, value]) => <div key={key}><dt>{key === 'T50DTE' ? 'MIR creation date (T50DTE)' : key === 'T50DFT' ? 'First travel date (T50DFT)' : key}</dt><dd>{display(value)}</dd></div>)}</dl>}
      {typedPanels[panel]?.map(key => <div key={key}><h3>{title(key)}</h3><BusinessRows rows={detail.typed[key] ?? []} /></div>)}
      {panel === 'Passengers' && <><h3>Explicit relationships</h3><BusinessRows rows={detail.relationships} /></>}
      {panel === 'All Sections' && <><p>All source sections, items, decoded fields and original bytes.</p>{detail.sections.map((s, i) => <details key={i}><summary>{display(s.identifier)} · occurrence {display(s.occurrence)} · {display(s.decoder_status)}</summary><p>{display(s.decoder_reason)}</p><pre>{JSON.stringify(s, null, 2)}</pre></details>)}<h3>Miscellaneous documents and other data</h3><BusinessRows rows={detail.typed.misc_document ?? []} /></>}
      {panel === 'Raw MIR' && <><pre>{detail.raw}</pre><details><summary>Original bytes (hex)</summary><pre>{detail.raw_hex}</pre></details></>}
      {panel === 'Validation Issues' && <>{detail.issues.length ? <ul>{detail.issues.map((issue, i) => <li key={i}><strong>{issue.Severity}: {issue.Code}</strong> — {issue.Message}</li>)}</ul> : <p>No parser validation issues. Section coverage still applies.</p>}<h3>Decoder coverage</h3>{detail.coverage.map(c => <p key={c.Section}><strong>{c.Section}: {c.Status}</strong> — {c.Reason}</p>)}</>}
      </section></>}
    </Dialog>
  </section>
}

function BusinessRows({ rows }: { rows: Record<string, unknown>[] }) {
  if (!rows.length) return <p>No data in this MIR.</p>
  return <div className="mir-business-rows">{rows.map((row, i) => <article key={i}><dl className="mir-header-fields">{Object.entries(row).filter(([key]) => !['decoded_data', 'id', 'record_id', 'section_id', 'source_item_id'].includes(key)).map(([key, value]) => <div key={key}><dt>{title(key)}</dt><dd>{display(value)}</dd></div>)}</dl><details><summary>Decoded fields and source association</summary><pre>{JSON.stringify(row, null, 2)}</pre></details></article>)}</div>
}
