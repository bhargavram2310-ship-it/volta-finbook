import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Button } from './ui/button'
import { useSalesDraft } from './sales-draft-context'
import { apiJSON } from '@/lib/mir'
import { salesClassSchema, salesTypeRecordsSchema, type SalesType } from '@/lib/sales-types'

export function SalesTypeSelect() {
  const { draft, selectSalesType } = useSalesDraft()
  const [params, setParams] = useSearchParams()
  const requested = salesClassSchema.safeParse(params.get('type'))
  const requestedClass = requested.success ? requested.data : null
  const [types, setTypes] = useState<SalesType[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setError('')
    apiJSON('/api/v1/masters/sales-types', { signal: controller.signal }).then(body => {
      if (controller.signal.aborted) return
      const records = salesTypeRecordsSchema.parse(body).records
      setTypes(records.flatMap(record => {
        const classification = salesClassSchema.safeParse(record.values.sales_class)
        return record.values.active === 'true' && classification.success
          ? [{ id: record.id, code: record.values.code, name: record.values.name, salesClass: classification.data }] : []
      }))
    }).catch(() => { if (!controller.signal.aborted) setError('Sales types could not be loaded.') })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [attempt])
  useEffect(() => {
    const targetClass = requestedClass ?? draft.salesClass
    const selected = types.find(type => type.salesClass === targetClass)
    if (selected && (selected.id !== draft.salesTypeId || selected.salesClass !== draft.salesClass)) selectSalesType(selected)
  }, [types, requestedClass, draft.salesClass, draft.salesTypeId, selectSalesType])
  const selected = types.find(type => type.id === draft.salesTypeId)
  return <div className="sales-type-header"><div className="form-field"><label htmlFor="sales-type">Sales type</label><select id="sales-type" value={selected?.id ?? ''} disabled={loading || !!error || !types.length} aria-describedby="sales-type-help" onChange={event => {
    const type = types.find(type => type.id === event.target.value)
    if (!type) return
    selectSalesType(type)
    setParams(previous => { const next = new URLSearchParams(previous); next.set('type', type.salesClass); return next }, { replace: true })
  }}><option value="">{loading ? 'Loading sales types…' : error ? 'Sales types unavailable' : 'Select sales type…'}</option>{types.map(type => <option value={type.id} key={type.id}>{type.code} — {type.name}</option>)}</select></div><div id="sales-type-help" className="sales-type-help">{error ? <><span role="status">{error}</span><Button variant="outline" size="sm" onClick={() => setAttempt(value => value + 1)}>Retry sales types</Button></> : !loading && !types.length ? <span>No active sales types. Add or activate a type in Masters.</span> : null}<Link to="/app/masters/sales-types">Manage sales types</Link></div></div>
}
