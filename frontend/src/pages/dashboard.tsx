import { Link, useSearchParams } from 'react-router-dom'
import { ArrowDownLeft, ArrowRight, ArrowUpRight, Banknote, BarChart3, CalendarDays, ChartNoAxesCombined, CheckCheck, CircleDollarSign, FilePlus2, Globe2, Landmark, Layers3, Plane, ReceiptText, ShieldCheck, TrendingUp, Wallet } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ErrorState, LoadingState } from '@/components/screen-state'

interface Metric { title: string; to: string; icon: LucideIcon; hint: string; count?: boolean }
const primaryMetrics: Metric[] = [
  { title: 'Month-to-date sales', to: '/app/reports/sales', icon: CalendarDays, hint: 'Posted sales · this month' },
  { title: 'Gross margin', to: '/app/reports/profitability', icon: TrendingUp, hint: 'Client revenue less supplier cost' },
  { title: 'Customer outstanding', to: '/app/reports/outstanding', icon: Wallet, hint: 'Receivables after allocations' },
  { title: 'Supplier outstanding', to: '/app/reports/outstanding', icon: ArrowDownLeft, hint: 'Payables after settlements' },
]
const secondaryMetrics: Metric[] = [
  { title: "Today's sales", to: '/app/reports/sales', icon: ReceiptText, hint: 'Posted sales · today' },
  { title: 'Cash balance', to: '/app/reports/cash-bank', icon: Banknote, hint: 'Posted cash ledger balance' },
  { title: 'Bank balance', to: '/app/cash-bank/reconciliation', icon: Landmark, hint: 'Posted bank ledger balance' },
  { title: 'Pending approvals', to: '/app/administration/approvals', icon: CheckCheck, hint: 'Approval workflow not configured', count: true },
]
function FinancialCard({ metric }: { metric: Metric }) {
  return <Link to={metric.to} className="financial-card">
    <div className="financial-card-label"><metric.icon size={18} aria-hidden="true" /><h3>{metric.title}</h3><ArrowUpRight size={15} aria-hidden="true" /></div>
    <div className="financial-card-value"><span aria-hidden="true">—</span><span className="sr-only">Unavailable</span>{!metric.count && <small>INR</small>}</div>
    <p>{metric.hint}</p>
  </Link>
}
export function Dashboard() {
  const [params, setParams] = useSearchParams()
  if (params.get('state') === 'loading') return <LoadingState />
  if (params.get('state') === 'error') return <ErrorState retry={() => { const next = new URLSearchParams(params); next.delete('state'); setParams(next) }} />
  return <div className="erp-dashboard">
    <div className="page-heading"><div><div className="page-eyebrow">BUSINESS OVERVIEW</div><h1>Overview</h1><p>Your sales, outstanding balances and daily priorities.</p></div><Button asChild><Link to="/app/sales/workspace"><FilePlus2 size={17} aria-hidden="true" />Sales Workspace</Link></Button></div>
    <section aria-labelledby="financial-snapshot-heading">
      <div className="section-heading"><h2 id="financial-snapshot-heading">Financial snapshot</h2><span className="snapshot-caption">Reporting currency · INR</span></div>
      <p className="financial-availability"><ShieldCheck size={16} aria-hidden="true" />Figures are unavailable until posted-ledger reporting is connected. A dash does not represent zero.</p>
      <div className="financial-grid">{primaryMetrics.map(metric => <FinancialCard key={metric.title} metric={metric} />)}</div>
      <div className="financial-grid secondary-financial-grid">{secondaryMetrics.map(metric => <FinancialCard key={metric.title} metric={metric} />)}</div>
    </section>
    <div className="dashboard-work-grid">
      <section className="panel" aria-labelledby="sales-performance-heading"><div className="panel-header"><div><h2 id="sales-performance-heading">Sales performance</h2><p>Daily posted sales across your services</p></div><Link to="/app/reports/sales" className="text-button">View report<ArrowUpRight size={15} aria-hidden="true" /></Link></div><div className="dashboard-data-empty"><ChartNoAxesCombined size={32} aria-hidden="true" /><h3>Sales trends are not available yet</h3><p>Compare International, Domestic and Miscellaneous sales here when reporting is connected.</p></div></section>
      <section className="panel" aria-labelledby="attention-heading"><div className="panel-header"><div><h2 id="attention-heading">Needs your attention</h2><p>Approvals, refunds and source imports</p></div></div><div className="dashboard-task-list">
        {[
          { to: '/app/administration/approvals', title: 'Pending approvals', detail: 'Workflow not configured', icon: CheckCheck },
          { to: '/app/international/refunds', title: 'Pending refunds', detail: 'Workflow not connected', icon: ArrowDownLeft },
          { to: '/app/international/cancellation', title: 'Cancelled tickets', detail: 'Register not connected', icon: ReceiptText },
          { to: '/app/administration/mir-report', title: 'MIR Report', detail: 'Fetch from your configured folder', icon: Layers3 },
        ].map(task => <Link to={task.to} key={task.to}><span className="dashboard-task-icon"><task.icon size={18} aria-hidden="true" /></span><span><strong>{task.title}</strong><small>{task.detail}</small></span><ArrowRight size={16} aria-hidden="true" /></Link>)}
      </div></section>
      <section className="panel" aria-labelledby="ageing-heading"><div className="panel-header"><div><h2 id="ageing-heading">Outstanding ageing</h2><p>Customer receivables by due date</p></div><Link to="/app/reports/outstanding" className="text-button">View ageing<ArrowUpRight size={15} aria-hidden="true" /></Link></div><div className="dashboard-data-empty ageing-empty"><BarChart3 size={28} aria-hidden="true" /><h3>Awaiting invoices and allocations</h3><p>Ageing buckets will appear here once their posted balances are available.</p></div></section>
      <section className="panel" aria-labelledby="services-heading"><div className="panel-header"><div><h2 id="services-heading">Sales by service</h2><p>Open a service workspace</p></div></div><div className="dashboard-task-list dashboard-service-list">
        {[
          { name: 'International', icon: Globe2, to: 'sales/workspace?type=international' },
          { name: 'Domestic', icon: Plane, to: 'sales/workspace?type=domestic' },
          { name: 'Miscellaneous', icon: CircleDollarSign, to: 'sales/workspace?type=miscellaneous' },
        ].map(service => <Link to={'/app/' + service.to} key={service.name}><span className="dashboard-task-icon"><service.icon size={18} aria-hidden="true" /></span><span><strong>{service.name}</strong><small>Sales reporting unavailable</small></span><span className="service-value"><span aria-hidden="true">—</span><span className="sr-only">Unavailable</span> <small>INR</small></span><ArrowUpRight size={15} aria-hidden="true" /></Link>)}
      </div></section>
    </div>
    <div className="dashboard-audit-link"><ShieldCheck size={16} aria-hidden="true" /><span>Financial figures will reconcile with posted entries.</span><Link to="/app/administration/audit">Audit trail<ArrowRight size={15} aria-hidden="true" /></Link></div>
  </div>
}
