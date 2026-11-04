import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { ArrowLeft, ArrowUpRight, Bookmark, Check, ChevronDown, FilePlus2, Filter, Info, LockKeyhole, Search, SlidersHorizontal, X } from 'lucide-react'
import type { PageDefinition } from '@/lib/navigation'
import { readPreference, savePreference } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { EmptyState, ErrorState, LoadingState } from '@/components/screen-state'
import { PaymentEntry } from '@/components/payment-entry'

interface SavedView { name: string; query: string; status: string }
function loadViews(key: string): SavedView[] {
  try { const value: unknown = JSON.parse(readPreference(key, '[]')); return Array.isArray(value) ? value.filter((v): v is SavedView => !!v && typeof v.name === 'string' && typeof v.query === 'string' && typeof v.status === 'string') : [] } catch { return [] }
}

export function ModulePage({ page, children }: { page: PageDefinition; children?: ReactNode }) {
  const [params, setParams] = useSearchParams()
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('All statuses')
  const [viewsOpen, setViewsOpen] = useState(false)
  const [viewName, setViewName] = useState('')
  const viewKey = `volta.views.${page.id}`
  const [views, setViews] = useState<SavedView[]>(() => loadViews(viewKey))
  const [saved, setSaved] = useState(false)
  const [paymentOpen, setPaymentOpen] = useState(() => page.id === 'cash-bank/payments' && params.get('entry') === 'payment')
  useEffect(() => { setQuery(''); setStatus('All statuses'); setViews(loadViews(viewKey)); setViewsOpen(false); setSaved(false) }, [viewKey])
  const isSales = page.id.endsWith('/sales') && page.group !== 'Reports'
  const isTax = page.id === 'masters/tax'
  const isPayments = page.id === 'cash-bank/payments'
  const isBankAccounts = page.id === 'masters/bank-cash'
  const internationalPreview = page.id === 'international/sales' || page.id === 'international/mir'
  const state = params.get('state')
  const filtered = !!query || status !== 'All statuses'
  const saveView = () => {
    const name = viewName.trim()
    if (!name) return
    const next = [...views.filter(v => v.name !== name), { name, query, status }].slice(-20)
    setViews(next); savePreference(viewKey, JSON.stringify(next)); setViewName(''); setSaved(true)
  }
  return <>
    <div className="page-heading"><div><div className="page-eyebrow">{page.group.toUpperCase()}</div><h1>{page.label}<span className="heading-dot">.</span></h1><p>{page.description}</p></div><span className={`badge badge-${page.deferred ? 'amber' : 'blue'}`}>{internationalPreview ? 'Entry preview' : page.deferred ? 'Deferred' : `Available in Phase ${page.phase}`}</span></div>
    {isSales && <div className="workflow-strip" aria-label="Sales workflow">{['Draft', 'Validation', 'Verification', 'Approval if required', 'Posting', 'Invoice', 'Settlement'].map((step, i) => <span key={step}><b>{i + 1}</b>{step}{i < 6 && <ArrowUpRight size={13} />}</span>)}</div>}
    <div className={`notice ${isTax || page.deferred ? 'notice-amber' : 'notice-blue'}`}><Info size={17} /><span>{page.deferred ? 'MIR import and automatic extraction are on hold. Use the manual sales workspace.' : isTax ? 'To be verified by finance. No GST, TDS, commission or markup rates are seeded. Rules will be configured after finance review.' : `This is the screen foundation. ${page.group === 'Masters' || page.group === 'Admin Control' ? 'Configuration and permissions' : 'Records and actions'} will be connected in Phase ${page.phase}.`}</span>{page.deferred && <Link to="/app/international/sales" className="text-button">Manual entry<ArrowUpRight size={14} /></Link>}</div>
    {children}
    <section className="panel register-panel" aria-label={`${page.label} workspace`}>
      <div className="register-heading"><div><h2>{isSales ? 'Sales documents' : page.label}</h2><span className="text-muted">Cloud data not connected</span></div><Button variant="outline" size="sm" onClick={() => { setViewsOpen(true); setSaved(false) }}><Bookmark size={14} />Saved views<ChevronDown size={13} /></Button></div>
      <div className="table-toolbar"><div className="table-search"><Search size={16} /><input type="search" aria-label={`Search ${page.label.toLowerCase()}`} placeholder={isSales ? 'Search document, ticket, PNR or passenger…' : `Search ${page.label.toLowerCase()}…`} value={query} onChange={e => setQuery(e.target.value)} /></div><label className="status-filter"><Filter size={14} /><span className="sr-only">Status filter</span><select aria-label="Status filter" value={status} onChange={e => setStatus(e.target.value)}>{['All statuses', 'Draft', 'Verified', 'Approved', 'Posted', 'Cancelled', 'Refunded', 'Duplicate', 'Failed'].map(s => <option key={s}>{s}</option>)}</select></label>{filtered && <Button variant="ghost" size="sm" onClick={() => { setQuery(''); setStatus('All statuses') }}><X size={14} />Clear</Button>}{isBankAccounts && <Button asChild size="sm"><Link to="/app/cash-bank/payments?entry=payment"><FilePlus2 size={14} />Add payment</Link></Button>}{isPayments ? <Button size="sm" onClick={() => setPaymentOpen(true)}><FilePlus2 size={14} />Add record</Button> : <Button variant="outline" size="sm" disabled title={`Record creation starts in Phase ${page.phase}`}><FilePlus2 size={14} />{isSales ? 'New document' : 'New record'}</Button>}</div>
      {state === 'loading' ? <LoadingState /> : state === 'error' ? <ErrorState retry={() => { const next = new URLSearchParams(params); next.delete('state'); setParams(next) }} /> : <><div className="table-scroll"><table><caption className="sr-only">{page.label}: no database records are connected.</caption><thead><tr>{page.columns.map(column => <th key={column} className={column.includes('currency') ? 'money-column' : ''} scope="col">{column}</th>)}</tr></thead><tbody><tr><td colSpan={page.columns.length}><EmptyState title={filtered ? 'No matching records' : page.deferred ? 'Import is on hold' : isSales ? 'A workspace for your next journey' : 'Your records will appear here'} description={filtered ? 'Your filter is ready. Records will become searchable when cloud data is connected.' : page.deferred ? 'Manual entry comes first. MIR processing will be resumed separately.' : isSales ? 'Manual ticket entry will connect passengers, journeys and both sides of the accounting in Phase 4.' : `This register will connect to your cloud PostgreSQL database in the upcoming implementation phases.`} /></td></tr></tbody></table></div><div className="table-footer"><span>{filtered ? 'Filters applied · ' : ''}No data connection</span><span>Read-only interface preview</span></div></>}
    </section>
    {isSales && <div className="accounting-preview"><div className="panel"><span className="page-eyebrow">CLIENT ACCOUNTING</span><h3>What your customer pays</h3><p>Client fare, discount, service charges and configured tax components.</p><strong>— <small>INR</small></strong></div><div className="margin-preview"><SlidersHorizontal size={20} /><span>Live margin</span><strong>— INR</strong><small>Available with ticket entry</small></div><div className="panel"><span className="page-eyebrow">SUPPLIER ACCOUNTING</span><h3>What your supplier receives</h3><p>Supplier cost, commission, incentives, charges and configured taxes.</p><strong>— <small>INR</small></strong></div></div>}
    <div className="screen-note"><LockKeyhole size={14} /><span>{isSales ? 'Posted documents will use reversals or adjustments. Direct editing and deletion will be blocked.' : 'Creation, changes and exports will require configured permissions and audit logging.'}</span></div>
    {isSales && <div className="sticky-actions"><span><span className="status-dot" />Manual entry · Phase 4</span><div>{['Save draft', 'Submit', 'Verify', 'Approve', 'Post', 'Print / invoice'].map(action => <Button key={action} variant={action === 'Post' ? 'default' : 'outline'} size="sm" disabled>{action}</Button>)}</div></div>}
    {isPayments && <PaymentEntry open={paymentOpen} onOpenChange={value => { setPaymentOpen(value); if (!value && params.has('entry')) { const next = new URLSearchParams(params); next.delete('entry'); setParams(next, { replace: true }) } }} />}
    <Dialog open={viewsOpen} onOpenChange={setViewsOpen} title="Saved views" description="Save filter preferences for this screen on this browser. No business records are stored."><div className="saved-views">{views.length ? views.map(view => <div key={view.name}><button onClick={() => { setQuery(view.query); setStatus(view.status); setViewsOpen(false) }}><Bookmark size={15} /><span><b>{view.name}</b><small>{view.status}{view.query ? ` · ${view.query}` : ''}</small></span></button><button aria-label={`Delete view ${view.name}`} onClick={() => { const next = views.filter(v => v.name !== view.name); setViews(next); savePreference(viewKey, JSON.stringify(next)) }}><X size={15} /></button></div>) : <p className="text-muted">No saved views for this screen yet.</p>}</div><form onSubmit={e => { e.preventDefault(); saveView() }}><label htmlFor="view-name">View name</label><div className="save-view-row"><input id="view-name" maxLength={60} value={viewName} onChange={e => { setViewName(e.target.value); setSaved(false) }} placeholder="e.g. Draft documents" required /><Button type="submit">Save view</Button></div></form>{saved && <p className="saved-message" role="status"><Check size={15} />View saved for this browser.</p>}</Dialog>
  </>
}

export function NotFound() {
  return <div className="not-found"><span className="page-eyebrow">404 · PAGE NOT FOUND</span><h1>Let’s get you back on course.</h1><p>This screen does not exist in the workspace.</p><Button asChild><Link to="/app/dashboard"><ArrowLeft size={16} />Back to overview</Link></Button></div>
}
