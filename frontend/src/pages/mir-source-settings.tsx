import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { FolderLock, LockKeyhole, RefreshCw } from 'lucide-react'
import { apiJSON, mirSourceSchema } from '@/lib/mir'
import type { MirSource } from '@/lib/mir'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import '@/sales-entry.css'

export function MirSourceSettings() {
  const [source, setSource] = useState<MirSource | null>(null)
  const [sourceLoading, setSourceLoading] = useState(true)
  const [sourceError, setSourceError] = useState('')
  const [path, setPath] = useState('')
  const [confirmLock, setConfirmLock] = useState(false)
  const [saving, setSaving] = useState(false)
  const mounted = useRef(false)
  const refreshSource = async () => {
    setSourceLoading(true); setSourceError('')
    try { const result = mirSourceSchema.parse(await apiJSON('/api/v1/mir/source')); if (mounted.current) setSource(result) }
    catch { if (mounted.current) setSourceError('MIR folder settings could not load. Start the Go API and retry.') }
    finally { if (mounted.current) setSourceLoading(false) }
  }

  useEffect(() => { mounted.current = true; void refreshSource(); return () => { mounted.current = false } }, [])
  const lockSource = async () => {
    setSaving(true); setSourceError('')
    try {
      const result = mirSourceSchema.parse(await apiJSON('/api/v1/mir/source', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ path: path.trim() }) }))
      setSource(result); setConfirmLock(false);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Folder could not be locked.'
      try { setSource(mirSourceSchema.parse(await apiJSON('/api/v1/mir/source'))) } catch { /* Keep the original error visible. */ }
      setSourceError(message); setConfirmLock(false)
    }
    finally { setSaving(false) }
  }

  return <>
    <div className="page-heading"><div><div className="page-eyebrow">ADMIN CONTROL</div><h1>MIR File Reader Location<span className="heading-dot">.</span></h1><p>Manage the local source used by International Sales MIR import.</p></div><span className="badge badge-blue">Local configuration</span></div>
    <p className="notice notice-amber">Admin Control organizes settings. Administrator access restrictions are not yet implemented.</p>
    <div className="mir-source-card"><div className="mir-section-heading"><FolderLock size={20} /><div><h2>MIR source folder</h2><p>Set the folder once. FinBook will read future files from the same locked location.</p></div></div>
      {sourceLoading ? <p role="status">Loading source folder…</p> : source?.locked ? <div className="locked-mir-path"><label htmlFor="locked-mir-folder">Locked folder</label><input id="locked-mir-folder" value={source.path} readOnly /><span className="badge badge-teal"><LockKeyhole size={13} />Locked</span></div> : <form className="mir-folder-form" onSubmit={e => { e.preventDefault(); if (path.trim()) setConfirmLock(true) }}><div><label htmlFor="mir-folder">Local MIR folder path</label><input id="mir-folder" placeholder="D:\MIR\Incoming" value={path} onChange={e => setPath(e.target.value)} maxLength={1000} required disabled={!source} /></div><Button type="submit" disabled={!path.trim() || !source}>Save & lock folder</Button></form>}
      {sourceError && <div className="mir-error" role="alert"><p>{sourceError}</p><Button variant="outline" size="sm" onClick={() => void refreshSource()}><RefreshCw size={14} />Retry settings</Button></div>}
      <p className="mir-help">The folder is on the machine running the Go API. Files are read only; they are never moved, renamed or deleted.</p>
    </div>

    <p className="mir-help"><Link className="text-button" to="/app/international/sales?mode=mir">Go to MIR import</Link></p>
    <Dialog open={confirmLock} onOpenChange={setConfirmLock} title="Lock this MIR folder?" description="Once saved, the app will always fetch MIR records from this folder. The path cannot be edited in the app."><p className="mir-confirm-path">{path}</p><div className="sales-dialog-actions"><Button variant="outline" disabled={saving} onClick={() => setConfirmLock(false)}>Cancel</Button><Button disabled={saving} onClick={() => void lockSource()}>{saving ? 'Saving…' : 'Confirm & lock folder'}</Button></div></Dialog>

  </>
}
