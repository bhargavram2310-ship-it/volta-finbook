import { z } from 'zod'
import type { SalesField } from './mir'

const option = z.object({ id: z.string().min(1), code: z.string(), name: z.string() })
export const salesLookupsSchema = z.object({
  airlines: z.array(option), currencies: z.array(option), customers: z.array(option), suppliers: z.array(option),
})
export type SalesLookups = z.infer<typeof salesLookupsSchema>
export type MasterOption = z.infer<typeof option>
export const salesMasterFields: Partial<Record<SalesField, { list: keyof SalesLookups; label: string }>> = {
  airline: { list: 'airlines', label: 'airline' },
  basicCurrency: { list: 'currencies', label: 'currency' },
  fareCurrency: { list: 'currencies', label: 'currency' },
  billTo: { list: 'customers', label: 'customer' },
  supplier: { list: 'suppliers', label: 'supplier' },
}
