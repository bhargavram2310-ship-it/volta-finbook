import { useEffect, useState } from 'react'

const labels: Record<string, string> = {
  checking: 'Checking cloud database…', connected: 'Cloud connected · schema ready',
  connected_schema_pending: 'Cloud connected · migrations pending',
  schema_incompatible: 'Cloud schema needs review', not_configured: 'Cloud database not configured',
  unavailable: 'Cloud database unavailable',
}
export function useDatabaseStatus() {
  const [state, setState] = useState('checking')
  useEffect(() => {
    let alive = true
    let controller: AbortController | null = null
    const check = async () => {
      controller?.abort(); controller = new AbortController()
      try {
        const response = await fetch('/api/v1/status', { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(7_000)]) })
        if (!response.ok) throw new Error('unavailable')
        const body: unknown = await response.json()
        if (!alive) return
        if (body && typeof body === 'object' && 'database' in body && typeof body.database === 'string' && body.database in labels) setState(body.database)
        else setState('unavailable')
      } catch { if (alive) setState('unavailable') }
    }
    void check()
    const interval = window.setInterval(() => void check(), 60_000)
    return () => { alive = false; controller?.abort(); window.clearInterval(interval) }
  }, [])
  return { state, label: labels[state] ?? labels.unavailable }
}
