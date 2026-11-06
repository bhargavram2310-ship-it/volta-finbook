import type { MasterOption } from '@/lib/sales-lookups'

export function SalesMasterSelect({ id, value, options, label, loading, unavailable, disabled, onChange }: {
  id: string; value: string; options: MasterOption[]; label: string; loading: boolean; unavailable: boolean; disabled?: boolean; onChange: (value: string) => void
}) {
  const unmatched = !!value && !options.some(option => option.id === value)
  return <>
    <select id={id} value={value} disabled={disabled || loading || unavailable} onChange={event => onChange(event.target.value)} aria-describedby={unmatched ? `${id}-master-hint` : undefined}>
      <option value="">{loading ? 'Loading master list…' : unavailable ? 'Master list unavailable' : options.length ? `Select ${label}…` : `No active ${label} records`}</option>
      {unmatched && <option value={value} disabled>{value} (requires master selection)</option>}
      {options.map(option => <option key={option.id} value={option.id}>{option.code} — {option.name}</option>)}
    </select>
    {unmatched && <small id={`${id}-master-hint`}>Select an active {label} from the master list.{disabled && ' Use the correction button to change this value.'}</small>}
  </>
}
