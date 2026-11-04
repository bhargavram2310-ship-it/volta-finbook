import { AlertCircle, Inbox, RefreshCw } from 'lucide-react'
import { Button } from './ui/button'

export function EmptyState({ title, description }: { title: string; description: string }) {
  return <div className="empty-state"><span className="empty-icon"><Inbox size={25} strokeWidth={1.5} /></span><h3>{title}</h3><p>{description}</p></div>
}

export function LoadingState() {
  return <div className="state-card" role="status" aria-label="Loading screen"><div className="skeleton skeleton-title" /><div className="skeleton" /><div className="skeleton" /><div className="skeleton" /><p>Loading workspace…</p></div>
}

export function ErrorState({ retry }: { retry: () => void }) {
  return <div className="state-card error-state" role="alert"><AlertCircle size={28} /><h3>This screen could not load</h3><p>Try again. No financial records have been changed.</p><Button variant="outline" onClick={retry}><RefreshCw size={15} />Try again</Button></div>
}
