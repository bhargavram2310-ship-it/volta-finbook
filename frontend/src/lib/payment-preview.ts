import { z } from 'zod'

// INR-only preview. Additional currencies require configured currency precision.
// Parse money as integer paise; never add financial amounts as binary floats.
export function parsePaymentAmount(value: string): bigint | null {
  if (!/^\d{1,12}(\.\d{1,2})?$/.test(value)) return null
  const [whole, fraction = ''] = value.split('.')
  return BigInt(whole) * 100n + BigInt(fraction.padEnd(2, '0'))
}

export function formatPaymentAmount(paise: bigint): string {
  return `${(paise / 100n).toLocaleString('en-IN')}.${(paise % 100n).toString().padStart(2, '0')} INR`
}

function validDate(value: string) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false
  const date = new Date(`${value}T00:00:00Z`)
  return !Number.isNaN(date.getTime()) && date.toISOString().slice(0, 10) === value
}

const dateField = z.string().refine(validDate, 'Enter a valid date.')
const lineSchema = z.object({
  payee: z.string().trim().min(1, 'Enter the payee.').max(160),
  ledgerAccount: z.string().trim().min(1, 'Enter the ledger account.').max(100),
  invoiceReference: z.string().trim().max(100),
  amount: z.string().refine(value => {
    const amount = parsePaymentAmount(value)
    return amount !== null && amount > 0n
  }, 'Enter an amount greater than zero, with up to 2 decimals.'),
  narration: z.string().trim().max(500),
})

export const paymentPreviewSchema = z.object({
  paymentDate: dateField,
  postingDate: dateField,
  accountType: z.enum(['bank', 'cash']),
  paymentAccount: z.string().trim().min(1, 'Enter the paying account.').max(100),
  bankMethod: z.enum(['transfer', 'cheque']),
  transferReference: z.string().trim().max(100),
  chequeNumber: z.string().trim().max(40),
  chequeDate: z.string(),
  narration: z.string().trim().max(2000),
  lines: z.array(lineSchema).min(1, 'Add at least one payment line.'),
}).superRefine((data, ctx) => {
  if (data.accountType === 'bank' && data.bankMethod === 'cheque') {
    if (!data.chequeNumber) ctx.addIssue({ code: 'custom', path: ['chequeNumber'], message: 'Enter the cheque number.' })
    if (!validDate(data.chequeDate)) ctx.addIssue({ code: 'custom', path: ['chequeDate'], message: 'Enter a valid cheque date.' })
  }
})

export type PaymentPreviewFields = z.infer<typeof paymentPreviewSchema>
export function newPaymentLine(): PaymentPreviewFields['lines'][number] {
  return { payee: '', ledgerAccount: '', invoiceReference: '', amount: '', narration: '' }
}

export function newPaymentPreview(): PaymentPreviewFields {
  // No default posting date, payment reference, account or cheque data is guessed.
  const now = new Date()
  const today = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
  return {
    paymentDate: today, postingDate: '', accountType: 'bank', paymentAccount: '',
    bankMethod: 'transfer', transferReference: '', chequeNumber: '', chequeDate: '',
    narration: '', lines: [newPaymentLine()],
  }
}
