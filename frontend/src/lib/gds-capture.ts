import { z } from 'zod'
import { mirRecordSchema } from './mir'

export const linkedSaleSchema = z.object({ id: z.string(), number: z.string(), status: z.string() })
export const captureProgressSchema = z.object({ status: z.enum(['Needs Review', 'Ready for Sale', 'Sale Created']), issues: z.array(z.string()), linked_sale: linkedSaleSchema.nullable(), can_prepare_draft: z.boolean(), can_view_sale: z.boolean() })
export const captureRowSchema = z.object({
  record_id: z.number(), passenger_id: z.number(), ticket_number: z.string(), pnr: z.string(), gds: z.string(), staff: z.string(),
  client_reference: z.string(), invoice_number: z.string(), branch: z.string(), passenger_name: z.string(), issue_date: z.string(),
  issue_date_source: z.string(), sector: z.string(), web_reference: z.string(), fare: z.string(), currency: z.string(), processing_date: z.string(),
  imported_at: z.string(), progress: captureProgressSchema,
})
export type CaptureRow = z.infer<typeof captureRowSchema>
export const captureSettingsSchema = z.object({ timezone: z.string(), today: z.string(), sales_save_enabled: z.literal(false), workflow_note: z.string() })
export const captureDetailsSchema = z.object({ ticket: captureRowSchema, draft: mirRecordSchema.nullable(), draft_error: z.string(), taxes: z.array(z.object({ code: z.string(), amount: z.string(), currency: z.string() })), customer_mapping: z.string(), accounting_note: z.string() })
export const captureSaleSchema = z.object({ sale: linkedSaleSchema, document_date: z.string(), currency: z.string(), customer: z.string(), narration: z.string(), ticket_number: z.string() })
export const captureColumns = [
  ['ticket_number', 'Ticket Number'], ['passenger_name', 'Passenger Name'], ['pnr', 'PNR'], ['gds', 'GDS'], ['sector', 'Route'], ['issue_date', 'Issue Date'], ['fare', 'Amount'],
  ['staff', 'Staff'], ['client_reference', 'Client Reference'], ['invoice_number', 'Invoice Number'], ['branch', 'Branch'], ['web_reference', 'Web Reference'], ['imported_at', 'Imported Date'],
] as const
export type CaptureColumn = typeof captureColumns[number][0]
const defaultColumns = captureColumns.slice(0, 7).map(([key]) => key)
export function loadCaptureColumns(): CaptureColumn[] {
  try {
    const saved: unknown = JSON.parse(sessionStorage.getItem('volta.gds-capture.columns.v2') ?? 'null')
    if (Array.isArray(saved)) return captureColumns.map(([key]) => key).filter(key => key === 'ticket_number' || saved.includes(key))
  } catch { /* Use the readable default when browser storage is unavailable. */ }
  return defaultColumns
}
export function saveCaptureColumns(columns: CaptureColumn[]) {
  try { sessionStorage.setItem('volta.gds-capture.columns.v2', JSON.stringify(columns)) } catch { /* In-memory selection still works. */ }
}
