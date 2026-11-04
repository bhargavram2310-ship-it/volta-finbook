import { Link, useSearchParams } from 'react-router-dom'
import { ArrowDownLeft, ArrowRight, ArrowUpRight, Banknote, BarChart3, BookOpenCheck, CalendarDays, ChartNoAxesCombined, CheckCheck, CircleDollarSign, FilePlus2, Globe2, Landmark, Layers3, Plane, ReceiptText, ShieldCheck, TrendingUp, Wallet } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ErrorState, LoadingState } from '@/components/screen-state'

const metrics: Array<{ title: string; to: string; icon: LucideIcon; hint: string; tone: string; count?: boolean }> = [
  { title: "Today's sales", to: '/app/reports/sales', icon: ReceiptText, hint: 'Posted sales · today', tone: 'blue' },
  { title: 'Month-to-date sales', to: '/app/reports/sales', icon: CalendarDays, hint: 'Posted sales · this month', tone: 'blue' },
  { title: 'Gross margin', to: '/app/reports/profitability', icon: TrendingUp, hint: 'Client revenue less supplier cost', tone: 'teal' },
  { title: 'Customer outstanding', to: '/app/reports/outstanding', icon: Wallet, hint: 'Receivables after allocations', tone: 'amber' },
  { title: 'Supplier outstanding', to: '/app/reports/outstanding', icon: ArrowDownLeft, hint: 'Payables after settlements', tone: 'violet' },
  { title: 'Cash balance', to: '/app/reports/cash-bank', icon: Banknote, hint: 'Posted cash ledger balance', tone: 'teal' },
  { title: 'Bank balance', to: '/app/cash-bank/reconciliation', icon: Landmark, hint: 'Posted bank ledger balance', tone: 'blue' },
  { title: 'Pending approvals', to: '/app/administration/approvals', icon: CheckCheck, hint: 'Approval workflow not configured', tone: 'amber', count: true },
]

export function Dashboard() {
  const [params, setParams] = useSearchParams()
  if (params.get('state') === 'loading') return <LoadingState />
  if (params.get('state') === 'error') return <ErrorState retry={() => { const next = new URLSearchParams(params); next.delete('state'); setParams(next) }} />
  return <>
    <div className="page-heading"><div><div className="page-eyebrow">YOUR BUSINESS AT A GLANCE</div><h1>Overview<span className="heading-dot">.</span></h1><p>Every journey accounted for. Every number connected.</p></div><Button asChild><Link to="/app/international/sales"><FilePlus2 size={16} />Manual sales entry<ArrowUpRight size={16} /></Link></Button></div>
    <div className="dashboard-callout"><span className="callout-icon"><BookOpenCheck size={23} /></span><div><strong>A clear start for your books</strong><p>Your workspace is ready to explore. Connect your cloud database in Phase 2 to begin setting up the company.</p></div><span className="badge badge-blue">Foundation ready</span></div>
    <div className="section-heading"><h2>Financial snapshot</h2><span className="subtle-label"><span className="status-dot" />Awaiting posted ledger data</span></div>
    <div className="metric-grid">{metrics.map(metric => <Link to={metric.to} className="metric-card" key={metric.title}><div className="metric-top"><span className={`metric-icon tone-${metric.tone}`}><metric.icon size={17} /></span><ArrowUpRight size={15} /></div><h3>{metric.title}</h3><div className="metric-value">—{!metric.count && <small>INR</small>}</div><p>{metric.hint}</p><span className="metric-state">Not connected</span></Link>)}</div>
    <div className="dashboard-charts"><section className="panel"><div className="panel-header"><div><h2>Sales performance</h2><p>Daily posted sales across your services</p></div><Link to="/app/reports/sales" className="text-button">View report<ArrowUpRight size={14} /></Link></div><div className="chart-empty"><div className="chart-grid" aria-hidden="true"><i /><i /><i /><i /></div><div className="chart-empty-content"><span className="empty-icon"><ChartNoAxesCombined size={27} /></span><h3>Your sales story starts here</h3><p>Daily sales trends will appear once ledger-backed reporting is connected.</p></div></div><div className="chart-legend"><span><i className="legend-blue" />International</span><span><i className="legend-teal" />Domestic</span><span><i className="legend-violet" />Miscellaneous</span></div></section>
      <section className="panel"><div className="panel-header"><div><h2>Sales by service</h2><p>One common sales structure</p></div><Layers3 size={18} className="text-muted" /></div><div className="service-list">{[{ name: 'International', icon: Globe2, to: 'international/sales', tone: 'blue' }, { name: 'Domestic', icon: Plane, to: 'domestic/sales', tone: 'teal' }, { name: 'Miscellaneous', icon: CircleDollarSign, to: 'miscellaneous/sales', tone: 'violet' }].map(service => <Link to={`/app/${service.to}`} key={service.name}><span className={`metric-icon tone-${service.tone}`}><service.icon size={18} /></span><div><strong>{service.name}</strong><small>Not connected</small></div><span className="service-amount">— <small>INR</small></span><Chevron /></Link>)}</div><div className="panel-note"><ShieldCheck size={15} /><span>Financial values will reconcile with posted entries.</span></div></section>
    </div>
    <div className="dashboard-bottom"><section className="panel"><div className="panel-header"><div><h2>Outstanding ageing</h2><p>Customer receivables by due date</p></div><Link to="/app/reports/outstanding" className="text-button">View ageing<ArrowUpRight size={14} /></Link></div><div className="ageing-placeholder"><BarChart3 size={27} /><p>Ageing will appear after invoices and allocations are posted.</p><span className="text-muted">Buckets will be configurable in reporting.</span></div></section><section className="panel"><div className="panel-header"><div><h2>Needs your attention</h2><p>Keep the working day moving</p></div></div><div className="attention-list"><Link to="/app/international/refunds"><span className="attention-icon"><ArrowDownLeft size={17} /></span><span><strong>Pending refunds</strong><small>Workflow not connected</small></span><b>—</b><ArrowUpRight size={14} /></Link><Link to="/app/international/cancellation"><span className="attention-icon"><ReceiptText size={17} /></span><span><strong>Cancelled tickets</strong><small>Register not connected</small></span><b>—</b><ArrowUpRight size={14} /></Link><Link to="/app/international/mir"><span className="attention-icon"><Layers3 size={17} /></span><span><strong>MIR import</strong><small>Fetch from your locked folder</small></span><span className="badge badge-neutral">MIR</span></Link></div></section></div>
    <div className="dashboard-endnote"><ShieldCheck size={15} />No sample financial balances. Figures will come from posted records only.<Link to="/app/administration/audit">Audit trail<ArrowRight size={13} /></Link></div>
  </>
}

function Chevron() { return <ArrowUpRight size={14} className="text-muted" /> }
