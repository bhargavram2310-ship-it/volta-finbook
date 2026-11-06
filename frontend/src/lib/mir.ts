import { z } from 'zod'

export const fieldKeys = ['ticketNumber', 'passengerName', 'pnr', 'airline', 'ticketDate', 'travelDate', 'sector', 'basicFare', 'basicCurrency', 'mirTotalFare', 'fareCurrency', 'billTo', 'supplier', 'clientTotal', 'supplierTotal', 'narration', 'serviceDescription'] as const
export type SalesField = typeof fieldKeys[number]
const extractedField = z.object({ value: z.string(), lines: z.array(z.number().int().positive()), fieldIds: z.array(z.string()) })
export const mirRecordSchema = z.object({
  id: z.string().min(1), fileName: z.string().min(1), sha256: z.string().regex(/^[a-f0-9]{64}$/),
  recordDate: z.string(), status: z.enum(['Extracted', 'Validation required', 'Failed', 'Duplicate']),
  fields: z.record(z.string(), extractedField),
  rawLines: z.array(z.object({ number: z.number().int().positive(), text: z.string(), rawHex: z.string(), terminatorHex: z.string() })),
  issues: z.array(z.string()),
})
export type MirRecord = z.infer<typeof mirRecordSchema>
export const mirResponseSchema = z.object({ records: z.array(mirRecordSchema), issues: z.array(z.string()), matchedFiles: z.number(), dateBasis: z.string() })
export const mirSourceSchema = z.object({ path: z.string(), locked: z.boolean(), configuredAt: z.string().optional() })
export type MirSource = z.infer<typeof mirSourceSchema>

export async function apiJSON(url: string, init?: RequestInit) {
  const response = await fetch(url, { ...init, signal: init?.signal ?? AbortSignal.timeout(70_000), cache: 'no-store' })
  let body: unknown
  try { body = await response.json() } catch { throw new Error('The Go API is unavailable. Start the API and try again.') }
  if (!response.ok) {
    const error = z.object({ error: z.string() }).safeParse(body)
    throw new Error(error.success ? error.data.error : 'The MIR request could not be completed.')
  }
  return body
}
