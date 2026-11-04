import { LayoutDashboard, Globe2, Plane, BriefcaseBusiness, Landmark, BookOpen, UsersRound, ChartNoAxesCombined, Settings2, ShieldCheck } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

export interface PageDefinition {
  id: string; label: string; description: string; group: string; phase: number;
  columns: string[]; deferred?: boolean
}
export interface NavSection { id: string; label: string; icon: LucideIcon; pages: PageDefinition[] }

function pages(group: string, phase: number, items: Array<[string, string, string, string[]]>): PageDefinition[] {
  return items.map(([id, label, description, columns]) => ({ id, label, description, columns, group, phase }))
}

export const dashboardPage: PageDefinition = { id: 'dashboard', label: 'Overview', description: 'Your travel business, in balance.', group: 'Workspace', phase: 7, columns: [] }
const ticketColumns = ['Document / ticket', 'Passenger / PNR', 'Customer', 'Supplier', 'Date', 'Sale · currency', 'Cost · currency', 'Status']
const moneyColumns = ['Reference', 'Date', 'Party / account', 'Amount · currency', 'Allocation', 'Status']
const masterColumns = ['Code', 'Name', 'Status', 'Last modified', 'Modified by']
const reportColumns = ['Report', 'Classification', 'Available filters', 'Access']

export const sections: NavSection[] = [
  { id: 'international', label: 'International', icon: Globe2, pages: pages('International', 4, [
    ['international/sales', 'Sales entry', 'Enter tickets manually or select date-filtered MIR records to populate sales.', ticketColumns],
    ['international/mir', 'MIR import', 'Filter MIR records by date and proceed to international sales entry.', ['Select', 'Source file', 'Record date', 'Ticket', 'Passenger', 'PNR', 'Status']],
    ['international/tickets', 'Ticket register', 'Find international tickets, passengers and their linked sales documents.', ticketColumns],
    ['international/cancellation', 'Cancellation', 'Track cancellation requests separately from confirmed cancellations.', ticketColumns],
    ['international/refunds', 'Refunds', 'Follow approval, receipt and payment of ticket refunds.', moneyColumns],
    ['international/reissue', 'Reissue & exchange', 'Trace a replacement ticket to its original issue and financial adjustments.', ticketColumns],
  ]).map(p => p.id.endsWith('sales') || p.id.endsWith('tickets') ? p : { ...p, phase: 6 }) },
  { id: 'domestic', label: 'Domestic', icon: Plane, pages: pages('Domestic', 4, [
    ['domestic/sales', 'Sales entry', 'Use the common sales structure for domestic manual ticket entry.', ticketColumns],
    ['domestic/tickets', 'Ticket register', 'Review domestic ticket records and their document links.', ticketColumns],
    ['domestic/cancellation', 'Cancellation', 'Review and authorize domestic cancellation requests.', ticketColumns],
    ['domestic/refunds', 'Refunds', 'Track domestic refunds through approval and settlement.', moneyColumns],
    ['domestic/reissue', 'Reissue & exchange', 'Link domestic ticket changes to the original transaction.', ticketColumns],
  ]).map(p => p.id.endsWith('sales') || p.id.endsWith('tickets') ? p : { ...p, phase: 6 }) },
  { id: 'miscellaneous', label: 'Miscellaneous', icon: BriefcaseBusiness, pages: pages('Miscellaneous', 4, [
    ['miscellaneous/bookings', 'Booking entry', 'Organize hotels, visas, insurance, transfers and other travel services.', ['Reference', 'Service', 'Customer', 'Supplier', 'Start date', 'Status']],
    ['miscellaneous/sales', 'Sales entry', 'Record travel service sales with a separate supplier cost.', moneyColumns],
    ['miscellaneous/cancellation', 'Cancellation & refund', 'Manage controlled cancellation and refund of travel services.', moneyColumns],
    ['miscellaneous/costs', 'Supplier cost entry', 'Review supplier costs linked to service bookings.', moneyColumns],
  ]) },
  { id: 'cash-bank', label: 'Cash & bank', icon: Landmark, pages: pages('Cash & bank', 7, [
    ['cash-bank/receipts', 'Receipts', 'Allocate customer collections across outstanding invoices.', moneyColumns],
    ['cash-bank/payments', 'Payments', 'Prepare bank or cash payments with multiple lines and narration.', ['Payment number', 'Payment date', 'Posting date', 'Account', 'Method', 'Amount · currency', 'Status']],
    ['cash-bank/transfers', 'Transfers', 'Track movements between authorized cash and bank accounts.', moneyColumns],
    ['cash-bank/reconciliation', 'Bank reconciliation', 'Compare bank statement lines with posted account movements.', moneyColumns],
  ]) },
  { id: 'vouchers', label: 'Accounting & ledger', icon: BookOpen, pages: pages('Accounting & ledger', 6, [
    ['vouchers/journal', 'Journal vouchers', 'Prepare balanced multi-line debit and credit entries.', ['Voucher', 'Date', 'Type', 'Debit · currency', 'Credit · currency', 'Status']],
    ['vouchers/receipt', 'Receipt vouchers', 'Review receipt vouchers and their financial source documents.', moneyColumns],
    ['vouchers/payment', 'Payment vouchers', 'Review payment vouchers and their financial source documents.', moneyColumns],
    ['vouchers/contra', 'Contra vouchers', 'Review transfers between cash and bank ledger accounts.', moneyColumns],
    ['vouchers/sales-purchase', 'Sales / purchase', 'Review sales and supplier vouchers generated from source transactions.', moneyColumns],
    ['vouchers/notes', 'Credit / debit notes', 'Trace approved financial adjustments to their source documents.', moneyColumns],
    ['vouchers/general-ledger', 'General ledger', 'Drill into posted entries by account, period and document.', ['Date', 'Account', 'Voucher', 'Debit · currency', 'Credit · currency', 'Balance · currency']],
  ]) },
  { id: 'masters', label: 'Masters', icon: UsersRound, pages: pages('Masters', 3, [
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
    ['masters/periods', 'Voucher types & financial years', 'Configure accounting periods and voucher types.', masterColumns],
  ]) },
  { id: 'reports', label: 'Reports', icon: ChartNoAxesCombined, pages: pages('Reports', 7, [
    ['reports/sales', 'Sales reports', 'Daily sales, service, airline, customer, passenger and invoice registers.', reportColumns],
    ['reports/outstanding', 'Outstanding & ageing', 'Customer, supplier and agent outstanding with configurable ageing.', reportColumns],
    ['reports/cash-bank', 'Cash & bank reports', 'Cash book, bank book, collections, payments and reconciliation.', reportColumns],
    ['reports/accounting', 'Accounting reports', 'Trial balance, profit and loss, balance sheet and tax summaries from posted entries.', reportColumns],
    ['reports/refunds', 'Cancellation & refund reports', 'Follow cancellations, refund balances and settlement.', reportColumns],
    ['reports/profitability', 'Profitability reports', 'Analyze margin by ticket, booking, customer, airline and branch.', reportColumns],
    ['mir/reports', 'MIR processing reports', 'Review MIR processing, missing data and duplicate-source reports when reporting is connected.', reportColumns],
  ]) },
  { id: 'administration', label: 'Admin Control', icon: Settings2, pages: pages('Admin Control', 3, [
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
