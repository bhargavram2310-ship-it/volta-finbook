import { LayoutDashboard, Globe2, Plane, BriefcaseBusiness, Landmark, FilePlus2, FileUp, UsersRound, ChartNoAxesCombined, Settings2, ShieldCheck } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

export interface PageDefinition {
  id: string; label: string; description: string; group: string;
  columns: string[]; deferred?: boolean
}
export interface NavSection { id: string; label: string; icon: LucideIcon; pages: PageDefinition[] }

function pages(group: string, items: Array<[string, string, string, string[]]>): PageDefinition[] {
  return items.map(([id, label, description, columns]) => ({ id, label, description, columns, group }))
}

export const dashboardPage: PageDefinition = { id: 'dashboard', label: 'Overview', description: 'Your travel business, in balance.', group: 'Workspace', columns: [] }
const ticketColumns = ['Document / ticket', 'Passenger / PNR', 'Customer', 'Supplier', 'Date', 'Sale · currency', 'Cost · currency', 'Status']
const moneyColumns = ['Reference', 'Date', 'Party / account', 'Amount · currency', 'Allocation', 'Status']
const masterColumns = ['Code', 'Name', 'Status', 'Last modified', 'Modified by']
const reportColumns = ['Report', 'Classification', 'Available filters', 'Access']

export const sections: NavSection[] = [
  { id: 'sales', label: 'Sales', icon: FilePlus2, pages: pages('Sales', [
    ['sales/workspace', 'Sales Workspace', 'Manage International, Domestic and Miscellaneous sales in one workspace.', ticketColumns],
  ]) },
  { id: 'international', label: 'International', icon: Globe2, pages: pages('International', [


    ['international/tickets', 'Ticket register', 'Find international tickets, passengers and their linked sales documents.', ticketColumns],
    ['international/cancellation', 'Cancellation', 'Track cancellation requests separately from confirmed cancellations.', ticketColumns],
    ['international/refunds', 'Refunds', 'Follow approval, receipt and payment of ticket refunds.', moneyColumns],
    ['international/reissue', 'Reissue & exchange', 'Trace a replacement ticket to its original issue and financial adjustments.', ticketColumns],
  ]) },
  { id: 'domestic', label: 'Domestic', icon: Plane, pages: pages('Domestic', [

    ['domestic/tickets', 'Ticket register', 'Review domestic ticket records and their document links.', ticketColumns],
    ['domestic/cancellation', 'Cancellation', 'Review and authorize domestic cancellation requests.', ticketColumns],
    ['domestic/refunds', 'Refunds', 'Track domestic refunds through approval and settlement.', moneyColumns],
    ['domestic/reissue', 'Reissue & exchange', 'Link domestic ticket changes to the original transaction.', ticketColumns],
  ]) },
  { id: 'miscellaneous', label: 'Miscellaneous', icon: BriefcaseBusiness, pages: pages('Miscellaneous', [
    ['miscellaneous/bookings', 'Booking entry', 'Organize hotels, visas, insurance, transfers and other travel services.', ['Reference', 'Service', 'Customer', 'Supplier', 'Start date', 'Status']],

    ['miscellaneous/cancellation', 'Cancellation & refund', 'Manage controlled cancellation and refund of travel services.', moneyColumns],
    ['miscellaneous/costs', 'Supplier cost entry', 'Review supplier costs linked to service bookings.', moneyColumns],
  ]) },
  { id: 'reconciliation', label: 'Reconciliation', icon: FileUp, pages: pages('Reconciliation', [
    ['reconciliation/upload', 'Upload for reconciliation', 'Upload PDF or Excel statements, extract their rows and save them for reconciliation.', moneyColumns],
  ]) },
  { id: 'cash-bank', label: 'Cash & bank', icon: Landmark, pages: pages('Cash & bank', [
    ['cash-bank/receipts', 'Receipts', 'Allocate customer collections across outstanding invoices.', moneyColumns],
    ['cash-bank/payments', 'Payments', 'Prepare bank or cash payments with multiple lines and narration.', ['Payment number', 'Payment date', 'Posting date', 'Account', 'Method', 'Amount · currency', 'Status']],
    ['cash-bank/transfers', 'Transfers', 'Track movements between authorized cash and bank accounts.', moneyColumns],
    ['cash-bank/reconciliation', 'Bank reconciliation', 'Compare bank statement lines with posted account movements.', moneyColumns],
  ]) },
  { id: 'masters', label: 'Masters', icon: UsersRound, pages: pages('Masters', [
    ['masters/sales-types', 'Sales types', 'Maintain the International, Domestic and Miscellaneous sales classifications.', masterColumns],
    ['masters/company', 'Company & branches', 'Configure the single company and its branches.', masterColumns],
    ['masters/customers', 'Customers', 'Maintain customers separately from travel agents and suppliers.', masterColumns],
    ['masters/agents', 'Travel agents', 'Maintain travel agent identities and commercial references.', masterColumns],
    ['masters/suppliers', 'Suppliers / consolidators', 'Maintain airline, BSP and consolidator settlement counterparties.', masterColumns],
    ['masters/travel', 'Airlines, airports & geography', 'Maintain airlines, airports, cities and countries.', masterColumns],
    ['masters/currencies', 'Currencies & exchange rates', 'Configure currencies and validated exchange rates.', masterColumns],
    ['masters/accounts', 'Chart of accounts', 'Configure ledger accounts and financial statement classifications.', masterColumns],
    ['masters/bank-cash', 'Bank & cash accounts', 'Configure cash accounts and access-controlled bank details.', masterColumns],
    ['masters/tax', 'Tax, commission & markup', 'Configure GST, TDS and commercial rules. To be verified by finance.', ['Code', 'Rule', 'Effective dates', 'Finance verification', 'Status']],
    ['masters/refund-rules', 'Cancellation & refund rules', 'Configure supplier-agreed penalties and agency fees.', masterColumns],
    ['masters/periods', 'Financial years & periods', 'Configure financial years and accounting periods.', masterColumns],
  ]) },
  { id: 'reports', label: 'Reports', icon: ChartNoAxesCombined, pages: pages('Reports', [
    ['reports/sales', 'Sales reports', 'Daily sales, service, airline, customer, passenger and invoice registers.', reportColumns],
    ['reports/outstanding', 'Outstanding & ageing', 'Customer, supplier and agent outstanding with configurable ageing.', reportColumns],
    ['reports/cash-bank', 'Cash & bank reports', 'Cash book, bank book, collections, payments and reconciliation.', reportColumns],
    ['reports/accounting', 'Accounting reports', 'Trial balance, profit and loss, balance sheet and tax summaries from posted entries.', reportColumns],
    ['reports/refunds', 'Cancellation & refund reports', 'Follow cancellations, refund balances and settlement.', reportColumns],
    ['reports/profitability', 'Profitability reports', 'Analyze margin by ticket, booking, customer, airline and branch.', reportColumns],

  ]) },
  { id: 'administration', label: 'Admin Control', icon: Settings2, pages: pages('Admin Control', [
    ['administration/mir-report', 'MIR Report', 'Manually extract MIR files and review source records.', ['Source file', 'MIR date', 'Ticket', 'Passenger', 'PNR', 'Status']],
    ['administration/mir-source', 'MIR File Reader Location', 'Set the local MIR source folder once for all future MIR imports.', ['Setting', 'Value', 'Status']],
    ['administration/employees', 'Employees', 'Create and maintain employee records.', masterColumns],
    ['administration/users', 'Users', 'Create login accounts and connect them to employees.', masterColumns],
    ['administration/roles', 'Roles', 'Maintain roles that can be assigned to user accounts.', masterColumns],
    ['administration/approvals', 'Approval configuration', 'Configure approval rules by transaction, amount and role.', masterColumns],
    ['administration/numbering', 'Numbering configuration', 'Configure company document numbering without reusing issued numbers.', masterColumns],
    ['administration/audit', 'Audit trail', 'Inspect immutable history of changes, approvals, postings and reversals.', ['Timestamp', 'Actor', 'Action', 'Entity', 'Old / new values', 'Reason']],
  ]) },
]

export const allPages = [dashboardPage, ...sections.flatMap(s => s.pages)]
export function pagePath(page: PageDefinition) { return `/app/${page.id}` }
export { LayoutDashboard, ShieldCheck }
