import { useState } from 'react'
import { useFieldArray, useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Banknote, CheckCircle2, CircleAlert, Landmark, LockKeyhole, Plus, Trash2 } from 'lucide-react'
import { Dialog } from './ui/dialog'
import { Button } from './ui/button'
import { StatusBadge } from './status-badge'
import { formatPaymentAmount, newPaymentLine, newPaymentPreview, parsePaymentAmount, paymentPreviewSchema } from '@/lib/payment-preview'
import type { PaymentPreviewFields } from '@/lib/payment-preview'
import '@/payment-entry.css'

export function PaymentEntry({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [validated, setValidated] = useState(false)
  const [discardOpen, setDiscardOpen] = useState(false)
  const { register, control, handleSubmit, reset, resetField, formState: { errors, isDirty } } = useForm<PaymentPreviewFields>({
    resolver: zodResolver(paymentPreviewSchema), defaultValues: newPaymentPreview(),
  })
  const { fields, append, remove } = useFieldArray({ control, name: 'lines' })
  const accountType = useWatch({ control, name: 'accountType' })
  const bankMethod = useWatch({ control, name: 'bankMethod' })
  const lines = useWatch({ control, name: 'lines' })
  const total = (lines ?? []).reduce((sum, line) => sum + (parsePaymentAmount(line.amount ?? '') ?? 0n), 0n)
  const invalidAmounts = (lines ?? []).some(line => { const value = parsePaymentAmount(line.amount ?? ''); return value === null || value <= 0n })

  const close = () => { reset(newPaymentPreview()); setValidated(false); setDiscardOpen(false); onOpenChange(false) }
  const requestClose = (next: boolean) => { if (next) onOpenChange(true); else if (isDirty) setDiscardOpen(true); else close() }
  const clearBankDetails = () => { resetField('transferReference'); resetField('chequeNumber'); resetField('chequeDate') }

  return <>
    <Dialog open={open} onOpenChange={requestClose} title="Add payment record" description="Bank or cash payment with embedded payment lines. Preview only — nothing is saved or posted." className="payment-dialog">
      <form noValidate onSubmit={handleSubmit(() => setValidated(true), () => setValidated(false))} onChangeCapture={() => setValidated(false)}>
        <div className="payment-header-strip"><span><Landmark size={17} />Payment details</span><StatusBadge status="Draft" /></div>
        <div className="payment-fields">
          <div className="form-field"><label htmlFor="payment-number">Payment number</label><input id="payment-number" value="Assigned on save" readOnly aria-describedby="payment-number-help" /><small id="payment-number-help">Unique number will be generated when saved.</small></div>
          <div className="form-field"><label htmlFor="payment-date">Payment date <span aria-hidden="true">*</span></label><input id="payment-date" type="date" aria-invalid={!!errors.paymentDate} aria-describedby={errors.paymentDate ? 'payment-date-error' : undefined} {...register('paymentDate')} /><FieldError id="payment-date-error" message={errors.paymentDate?.message} /></div>
          <div className="form-field"><label htmlFor="posting-date">Posting date <span aria-hidden="true">*</span></label><input id="posting-date" type="date" aria-invalid={!!errors.postingDate} aria-describedby={errors.postingDate ? 'posting-date-error' : 'posting-date-help'} {...register('postingDate')} /><small id="posting-date-help">Accounting period validation will apply at posting.</small><FieldError id="posting-date-error" message={errors.postingDate?.message} /></div>
          <div className="form-field"><label htmlFor="payment-status">Status</label><input id="payment-status" value="Draft" readOnly /><small>Status changes follow the approval and posting workflow.</small></div>
          <div className="form-field"><label htmlFor="payment-type">Payment from <span aria-hidden="true">*</span></label><select id="payment-type" {...register('accountType', { onChange: clearBankDetails })}><option value="bank">Bank account</option><option value="cash">Cash account</option></select></div>
          <div className="form-field"><label htmlFor="payment-account">{accountType === 'bank' ? 'Bank account' : 'Cash account'} <span aria-hidden="true">*</span></label><input id="payment-account" placeholder="Account name or code" maxLength={100} aria-invalid={!!errors.paymentAccount} aria-describedby={errors.paymentAccount ? 'payment-account-error' : 'payment-account-help'} {...register('paymentAccount')} /><small id="payment-account-help">Account selection will use configured masters.</small><FieldError id="payment-account-error" message={errors.paymentAccount?.message} /></div>
          <div className="form-field"><label htmlFor="payment-currency">Currency</label><input id="payment-currency" value="INR" readOnly /><small>Additional currencies require configured precision and rates.</small></div>
        </div>
        {accountType === 'bank' ? <section className="payment-bank-details" aria-label="Bank payment details"><div className="payment-bank-heading"><Landmark size={18} /><h3>Bank payment method</h3></div><div className="payment-fields">
          <div className="form-field"><label htmlFor="bank-method">Payment method</label><select id="bank-method" {...register('bankMethod', { onChange: clearBankDetails })}><option value="transfer">Transfer</option><option value="cheque">Cheque</option></select></div>
          {bankMethod === 'transfer' ? <div className="form-field"><label htmlFor="transfer-reference">Transfer reference / UTR</label><input id="transfer-reference" placeholder="Optional while preparing a draft" maxLength={100} {...register('transferReference')} /></div> : <>
            <div className="form-field"><label htmlFor="cheque-number">Cheque number <span aria-hidden="true">*</span></label><input id="cheque-number" placeholder="Enter cheque number" maxLength={40} aria-invalid={!!errors.chequeNumber} aria-describedby={errors.chequeNumber ? 'cheque-number-error' : undefined} {...register('chequeNumber')} /><FieldError id="cheque-number-error" message={errors.chequeNumber?.message} /></div>
            <div className="form-field"><label htmlFor="cheque-date">Cheque date <span aria-hidden="true">*</span></label><input id="cheque-date" type="date" aria-invalid={!!errors.chequeDate} aria-describedby={errors.chequeDate ? 'cheque-date-error' : undefined} {...register('chequeDate')} /><FieldError id="cheque-date-error" message={errors.chequeDate?.message} /></div>
          </>}
        </div></section> : <div className="payment-cash-note"><Banknote size={18} />Cash payment selected. Bank transfer and cheque details do not apply.</div>}
        <section className="payment-lines-section" aria-label="Payment lines"><div className="payment-lines-heading"><div><h3>Payment lines</h3><p>Add multiple payees or ledger allocations within this payment.</p></div><Button variant="outline" size="sm" onClick={() => { append(newPaymentLine(), { focusName: `lines.${fields.length}.payee` }); setValidated(false) }}><Plus size={16} />Add line</Button></div>
          <div className="payment-lines-scroll"><table className="payment-lines-table"><caption className="sr-only">Embedded payment lines. Each line requires a payee, ledger account and positive INR amount.</caption><thead><tr><th scope="col">Line</th><th scope="col">Payee *</th><th scope="col">Ledger account *</th><th scope="col">Invoice / reference</th><th scope="col" className="money-column">Amount · INR *</th><th scope="col">Line narration</th><th scope="col"><span className="sr-only">Remove line</span></th></tr></thead><tbody>
            {fields.map((field, index) => <tr key={field.id}><td className="payment-line-number">{index + 1}</td><td><input aria-label={`Line ${index + 1} payee`} maxLength={160} placeholder="Payee name" aria-invalid={!!errors.lines?.[index]?.payee} aria-describedby={errors.lines?.[index]?.payee ? `line-${field.id}-payee-error` : undefined} {...register(`lines.${index}.payee`)} /><FieldError id={`line-${field.id}-payee-error`} message={errors.lines?.[index]?.payee?.message} /></td><td><input aria-label={`Line ${index + 1} ledger account`} maxLength={100} placeholder="Account code / name" aria-invalid={!!errors.lines?.[index]?.ledgerAccount} aria-describedby={errors.lines?.[index]?.ledgerAccount ? `line-${field.id}-ledger-error` : undefined} {...register(`lines.${index}.ledgerAccount`)} /><FieldError id={`line-${field.id}-ledger-error`} message={errors.lines?.[index]?.ledgerAccount?.message} /></td><td><input aria-label={`Line ${index + 1} invoice reference`} maxLength={100} placeholder="Optional reference" {...register(`lines.${index}.invoiceReference`)} /></td><td><input aria-label={`Line ${index + 1} amount INR`} inputMode="decimal" maxLength={15} placeholder="0.00" className="amount-input" aria-invalid={!!errors.lines?.[index]?.amount} aria-describedby={errors.lines?.[index]?.amount ? `line-${field.id}-amount-error` : undefined} {...register(`lines.${index}.amount`)} /><FieldError id={`line-${field.id}-amount-error`} message={errors.lines?.[index]?.amount?.message} /></td><td><input aria-label={`Line ${index + 1} narration`} maxLength={500} placeholder="Line narration" {...register(`lines.${index}.narration`)} /></td><td><Button variant="ghost" size="icon" aria-label={`Remove line ${index + 1}`} disabled={fields.length === 1} onClick={() => { remove(index); setValidated(false) }}><Trash2 size={16} /></Button></td></tr>)}
          </tbody></table></div>
          <div className="payment-total"><span>{fields.length} payment {fields.length === 1 ? 'line' : 'lines'}</span><div><span>Total payment</span><output aria-label="Total payment" aria-live="polite">{formatPaymentAmount(total)}</output>{invalidAmounts && <small>Incomplete or invalid amounts are excluded.</small>}</div></div>
        </section>
        <div className="form-field payment-narration"><label htmlFor="payment-narration">Narration</label><textarea id="payment-narration" rows={3} maxLength={2000} placeholder="Describe the purpose of this payment…" {...register('narration')} /></div>
        {validated && <div role="status" className="notice payment-validation"><CheckCircle2 size={19} /><span>Form fields are valid. This payment has not been saved or posted. Account mappings, duplicates, permissions and accounting period checks require the connected backend.</span></div>}
        <div className="payment-actions"><p><LockKeyhole size={15} />Unsaved preview · No ledger entries</p><div><Button variant="outline" onClick={() => requestClose(false)}>Cancel</Button><Button type="submit"><CheckCircle2 size={16} />Validate payment</Button><Button disabled title="Saving requires the cloud database, permissions and audit logging">Save draft</Button><Button variant="outline" disabled title="Posting requires validated accounts, approval and balanced ledger entries">Post</Button></div></div>
      </form>
    </Dialog>
    <Dialog open={discardOpen} onOpenChange={setDiscardOpen} title="Discard this payment preview?" description="Your payment details and lines have not been saved. Closing will discard the form."><div className="discard-payment-actions"><Button variant="outline" onClick={() => setDiscardOpen(false)}>Keep editing</Button><Button onClick={close}><CircleAlert size={16} />Discard preview</Button></div></Dialog>
  </>
}

function FieldError({ id, message }: { id: string; message?: string }) {
  return message ? <p id={id} className="field-error" role="alert">{message}</p> : null
}
