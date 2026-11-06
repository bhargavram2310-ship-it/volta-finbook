import { createContext, useContext, useState } from 'react'
import type { ReactNode } from 'react'
import type { MirRecord, SalesField } from '@/lib/mir'
import type { SalesClass, SalesType } from '@/lib/sales-types'

export interface Correction { field: SalesField; oldValue: string; newValue: string; reason: string; at: string; actor: string }
interface Draft { values: Partial<Record<SalesField, string>>; source: MirRecord | null; corrections: Correction[]; salesTypeId: string; salesClass: SalesClass }
interface DraftContext { draft: Draft; selectSalesType: (type: SalesType) => void; editManual: (key: SalesField, value: string) => void; correct: (field: SalesField, value: string, reason: string) => void; load: (record: MirRecord, mappedValues?: Partial<Record<SalesField, string>>) => void }
const Context = createContext<DraftContext | null>(null)
export function SalesDraftProvider({ children }: { children: ReactNode }) {
  const [draft, setDraft] = useState<Draft>({ values: {}, source: null, corrections: [], salesTypeId: '', salesClass: 'international' })
  const value: DraftContext = {
    draft,
    selectSalesType: type => setDraft(previous => ({ ...previous, salesTypeId: type.id, salesClass: type.salesClass })),
    editManual: (key, value) => setDraft(previous => ({ ...previous, values: { ...previous.values, [key]: value } })),
    correct: (field, value, reason) => setDraft(previous => {
      const oldValue = previous.values[field] ?? ''
      if (oldValue === value || !reason.trim()) return previous
      return { ...previous, values: { ...previous.values, [field]: value }, corrections: [...previous.corrections, { field, oldValue, newValue: value, reason: reason.trim(), at: new Date().toISOString(), actor: 'Interface preview (not signed in)' }] }
    }),
    load: (record, mappedValues) => setDraft(previous => previous.source?.id === record.id ? previous : {
      ...previous,
      source: structuredClone(record), corrections: [],
      values: { ...Object.fromEntries(Object.entries(record.fields).map(([key, field]) => [key, field.value])), ...mappedValues },
    }),
  }
  return <Context.Provider value={value}>{children}</Context.Provider>
}
export function useSalesDraft() { const context = useContext(Context); if (!context) throw new Error('Sales draft provider required'); return context }
