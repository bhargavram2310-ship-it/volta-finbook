export type Status = 'Draft' | 'Verified' | 'Approved' | 'Posted' | 'Cancelled' | 'Refunded' | 'Duplicate' | 'Failed'
const classes: Record<Status, string> = {
  Draft: 'neutral', Verified: 'blue', Approved: 'teal', Posted: 'teal',
  Cancelled: 'red', Refunded: 'blue', Duplicate: 'amber', Failed: 'red',
}
export function StatusBadge({ status }: { status: Status }) {
  return <span className={`badge badge-${classes[status]}`}><span className="status-dot" />{status}</span>
}
