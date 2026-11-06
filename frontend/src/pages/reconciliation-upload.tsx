import { useEffect, useRef, useState } from 'react'
import { FileUp, RefreshCw, CheckCircle2 } from 'lucide-react'
import { z } from 'zod'
import { apiJSON } from '@/lib/mir'
import { Button } from '@/components/ui/button'
import './reconciliation-upload.css'

const statementSchema = z.object({ id: z.string(), fileName: z.string(), kind: z.enum(['bank', 'supplier']), rowCount: z.number().int(), uploadedBy: z.string(), createdAt: z.string() })
const detailSchema = z.object({ statement: statementSchema, rows: z.array(z.object({ number: z.number(), section: z.string(), sourceRow: z.number(), cells: z.array(z.string()) })), offset: z.number() })
type Statement = z.infer<typeof statementSchema>

export function ReconciliationUpload() {
  const [statements, setStatements] = useState<Statement[]>([])
  const [file, setFile] = useState<File | null>(null)
  const [kind, setKind] = useState('bank')
  const [busy, setBusy] = useState(false)
  const [loading, setLoading] = useState(true)
  const [listError, setListError] = useState('')
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [revision, setRevision] = useState(0)
  const [selected, setSelected] = useState('')
  const [offset, setOffset] = useState(0)
  const [detail, setDetail] = useState<z.infer<typeof detailSchema> | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState('')
  const input = useRef<HTMLInputElement>(null)
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setListError('')
    apiJSON('/api/v1/reconciliation/statements', { signal: controller.signal })
      .then(body => { if (!controller.signal.aborted) setStatements(z.object({ statements: z.array(statementSchema) }).parse(body).statements) })
      .catch(error => { if (!controller.signal.aborted) setListError(error instanceof Error ? error.message : 'Statements could not be loaded.') })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [revision])
  useEffect(() => {
    if (!selected) return
    const controller = new AbortController()
    setDetailLoading(true); setDetailError(''); setDetail(null)
    apiJSON(`/api/v1/reconciliation/statements/${selected}?offset=${offset}`, { signal: controller.signal })
      .then(body => { if (!controller.signal.aborted) setDetail(detailSchema.parse(body)) })
      .catch(error => { if (!controller.signal.aborted) setDetailError(error instanceof Error ? error.message : 'Statement rows could not be loaded.') })
      .finally(() => { if (!controller.signal.aborted) setDetailLoading(false) })
    return () => controller.abort()
  }, [selected, offset, revision])
  const upload = async () => {
    if (!file || busy) return
    setBusy(true); setError(''); setMessage('')
    const body = new FormData(); body.set('file', file); body.set('kind', kind)
    try {
      const result = z.object({ statement: statementSchema, duplicate: z.boolean() }).parse(await apiJSON('/api/v1/reconciliation/statements', { method: 'POST', body }))
      setMessage(result.duplicate ? `This file was already imported. Showing the existing ${result.statement.rowCount} saved rows.` : `${result.statement.fileName}: ${result.statement.rowCount} rows extracted and saved to the database.`)
      setSelected(result.statement.id); setOffset(0); setRevision(value => value + 1)
      setFile(null); if (input.current) input.current.value = ''
    } catch (error) { setError(error instanceof Error ? error.message : 'The statement could not be imported. Try again.') }
    finally { setBusy(false) }
  }
  return <div className="statement-upload">
    <header className="page-heading"><div><div className="page-eyebrow">RECONCILIATION</div><h1>Upload statements<span className="heading-dot">.</span></h1><p>Extract PDF and Excel statement data and save it for reconciliation.</p></div><Button variant="outline" disabled={busy || loading} onClick={() => setRevision(value => value + 1)}><RefreshCw size={16} aria-hidden="true" />Refresh</Button></header>
    <section className="panel statement-upload-form" aria-labelledby="statement-upload-heading"><h2 id="statement-upload-heading"><FileUp size={21} aria-hidden="true" />Upload a statement</h2>
      <form onSubmit={event => { event.preventDefault(); void upload() }}>
        <div className="statement-upload-fields"><div className="form-field"><label htmlFor="statement-kind">Statement type</label><select id="statement-kind" value={kind} disabled={busy} onChange={event => setKind(event.target.value)}><option value="bank">Bank statement</option><option value="supplier">Airline / supplier statement</option></select></div>
          <div className="form-field"><label htmlFor="statement-file">PDF or Excel statement</label><input ref={input} id="statement-file" type="file" accept=".pdf,.xlsx,.csv" disabled={busy} aria-describedby="statement-file-help" onChange={event => { const chosen = event.target.files?.[0]; setError(''); setMessage(''); if (chosen && (!/\.(pdf|xlsx|csv)$/i.test(chosen.name) || chosen.size === 0 || chosen.size > 10 * 1024 * 1024)) { setFile(null); event.target.value = ''; setError('Choose a nonempty PDF, .xlsx or CSV file up to 10 MB. Save older .xls workbooks as .xlsx first.'); return } setFile(chosen ?? null) }} required /></div></div>
        <p id="statement-file-help" className="statement-help">PDF, Excel (.xlsx), or CSV · Up to 10 MB and 10,000 rows. Use searchable PDFs; scanned documents need OCR. Save older .xls workbooks as .xlsx.</p>
        <Button type="submit" disabled={!file || busy}><FileUp size={16} aria-hidden="true" />{busy ? 'Extracting and saving…' : 'Upload, extract & save'}</Button>
      </form>
      {error && <p role="alert" className="mir-error">{error}</p>}
      {message && <p role="status" className="statement-success"><CheckCircle2 size={18} aria-hidden="true" />{message}</p>}
    </section>
    <section className="panel statement-register" aria-labelledby="saved-statements-heading"><h2 id="saved-statements-heading">Saved statements</h2><p className="statement-help">Latest 50 uploads. Select a statement to review its saved rows.</p>
      {loading && <p role="status">Loading statements…</p>}
      {listError && <p role="alert" className="mir-error">{listError} <Button variant="outline" size="sm" onClick={() => setRevision(value => value + 1)}>Retry statements</Button></p>}
      {!loading && !listError && !statements.length && <p className="statement-help">No statements uploaded yet.</p>}
      {!!statements.length && <div className="table-scroll"><table><caption className="sr-only">Statements saved for reconciliation</caption><thead><tr><th scope="col">Statement</th><th scope="col">Type</th><th scope="col">Rows</th><th scope="col">Uploaded</th></tr></thead><tbody>{statements.map(statement => <tr key={statement.id} className={selected === statement.id ? 'statement-selected' : ''}><td><button className="statement-link" onClick={() => { setSelected(statement.id); setOffset(0) }} aria-pressed={selected === statement.id}>{statement.fileName}</button></td><td>{statement.kind === 'bank' ? 'Bank' : 'Airline / supplier'}</td><td>{statement.rowCount}</td><td>{new Date(statement.createdAt).toLocaleString()}</td></tr>)}</tbody></table></div>}
    </section>
    {selected && <section className="panel statement-register" aria-labelledby="statement-rows-heading" aria-busy={detailLoading}><h2 id="statement-rows-heading">Extracted rows{detail ? ` · ${detail.statement.fileName}` : ''}</h2><p className="statement-help">Source values, headers and balances are preserved. Review these rows before matching transactions. Uploading does not post accounting entries.</p>
      {detailLoading && <p role="status">Loading saved rows…</p>}
      {detailError && <p role="alert" className="mir-error">{detailError} <Button size="sm" variant="outline" onClick={() => setRevision(value => value + 1)}>Retry rows</Button></p>}
      {detail && <><div className="table-scroll"><table><caption className="sr-only">Extracted data from {detail.statement.fileName}</caption><thead><tr><th scope="col">Row</th><th scope="col">Sheet / page</th><th scope="col">Source row</th><th scope="col">Extracted values</th></tr></thead><tbody>{detail.rows.map(row => <tr key={row.number}><td>{row.number}</td><td>{row.section}</td><td>{row.sourceRow}</td><td><div className="statement-cells">{row.cells.map((cell, index) => <span key={index} title={`Source cell ${index + 1}`}>{cell || '—'}</span>)}</div></td></tr>)}</tbody></table></div><div className="statement-pagination"><span>{detail.offset + 1}–{detail.offset + detail.rows.length} of {detail.statement.rowCount} rows</span><Button size="sm" variant="outline" disabled={detail.offset === 0} onClick={() => setOffset(value => Math.max(0, value - 100))}>Previous</Button><Button size="sm" variant="outline" disabled={detail.offset + detail.rows.length >= detail.statement.rowCount} onClick={() => setOffset(value => value + 100)}>Next</Button></div></>}
    </section>}
  </div>
}
