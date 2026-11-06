import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { ArrowRight, Eye, EyeOff, LockKeyhole, MoveUpRight, Plane, ShieldCheck, CircleCheck } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'

const loginSchema = z.object({ username: z.string().trim().min(1, 'Enter your username.'), password: z.string().min(1, 'Enter your password.') })
const passwordSchema = z.object({
  currentPassword: z.string().min(1, 'Enter your current password.'),
  newPassword: z.string().min(6, 'Use at least 6 characters.').max(72, 'Password must be 72 characters or fewer.'),
  confirmPassword: z.string(),
}).refine(value => value.newPassword === value.confirmPassword, { path: ['confirmPassword'], message: 'Passwords do not match.' })
type LoginFields = z.infer<typeof loginSchema>
type PasswordFields = z.infer<typeof passwordSchema>

export function Login() {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  // Accept only application destinations on this origin.
  const requestedPath = params.get('returnTo') ?? ''
  let returnTo = '/app/dashboard'
  try {
    const target = new URL(requestedPath, window.location.origin)
    if (target.origin === window.location.origin && target.pathname.startsWith('/app/')) {
      returnTo = target.pathname + target.search + target.hash
    }
  } catch { /* An invalid return address uses the overview. */ }
  const [showPassword, setShowPassword] = useState(false)
  const [forgot, setForgot] = useState(false)
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)
  const [passwordChangeRequired, setPasswordChangeRequired] = useState(false)
  const { register, handleSubmit, resetField, formState: { errors } } = useForm<LoginFields>({ resolver: zodResolver(loginSchema) })
  const passwordForm = useForm<PasswordFields>({ resolver: zodResolver(passwordSchema) })

  async function submitLogin(fields: LoginFields) {
    setBusy(true)
    setMessage('')
    try {
      const response = await fetch('/api/v1/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify(fields),
      })
      if (!response.ok) {
        if (response.status === 403) {
          setPasswordChangeRequired(true)
          resetField('password')
          return
        }
        setMessage(response.status === 401 ? 'Username or password is incorrect.' : 'Sign-in is temporarily unavailable. Try again shortly.')
        resetField('password')
        return
      }
      const result: { user?: { passwordChangeRequired?: boolean } } = await response.json()
      if (result.user?.passwordChangeRequired) {
        setPasswordChangeRequired(true)
        passwordForm.setValue('currentPassword', fields.password)
        resetField('password')
        return
      }
      navigate(returnTo, { replace: true })
    } catch {
      setMessage('Could not reach the sign-in service. Check the API and try again.')
      resetField('password')
    } finally {
      setBusy(false)
    }
  }

  async function submitPasswordChange(fields: PasswordFields) {
    setBusy(true)
    setMessage('')
    try {
      const response = await fetch('/api/v1/auth/password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({ currentPassword: fields.currentPassword, newPassword: fields.newPassword }),
      })
      if (!response.ok) {
        const result: { error?: string } = await response.json().catch(() => ({}))
        if (result.error === 'invalid_current_password') {
          setMessage('Current password is incorrect.')
        } else if (result.error === 'authentication_required') {
          setMessage('Your sign-in session expired. Sign in again to update your password.')
          setPasswordChangeRequired(false)
          passwordForm.reset()
        } else {
          setMessage('Password update is temporarily unavailable.')
        }
        return
      }
      navigate(returnTo, { replace: true })
    } catch {
      setMessage('Could not reach the sign-in service. Try again.')
    } finally {
      setBusy(false)
    }
  }

  return <main className="login-page">
    <section className="login-story" aria-label="About VOLTA FinBook">
      <Link to="/login" className="brand"><span className="brand-mark">V<MoveUpRight size={14} /></span><span>VOLTA<span className="brand-sub">FinBook</span></span></Link>
      <div className="story-content"><div className="eyebrow"><span /> BUILT FOR THE BUSINESS OF TRAVEL</div>
        <h1>Every journey.<br />Every number.<br /><span>In balance.</span></h1>
        <p>One workspace for your sales, operations and financial accounting. A clearer view of where your business is going.</p>
        <div className="journey-visual" aria-hidden="true"><div className="journey-stop"><span>SALES</span><b>Ticket to opportunity</b></div><div className="journey-path"><i /><Plane size={23} /><i /></div><div className="journey-stop"><span>FINANCE</span><b>Clarity at every step</b></div></div>
        <div className="story-features"><span><CircleCheck size={16} />Client & supplier clarity</span><span><CircleCheck size={16} />Traceable by design</span></div>
      </div>
      <footer>VOLTA Tours and Travels <span>Sales · Operations · Accounting</span></footer>
    </section>
    <section className="login-form-side">
      <span className="badge badge-blue login-security">{passwordChangeRequired ? 'Secure account setup' : 'Secure sign in'}</span>
      <div className="login-form-content"><div className="login-lock"><LockKeyhole size={24} /></div>
        <h2>{passwordChangeRequired ? 'Update your password' : 'Welcome to FinBook'}</h2>
        <p className="text-muted">{passwordChangeRequired ? 'Choose a new password with at least 6 characters to continue.' : 'Enter the username or email selected for your account. Initial login: SuperAdmin / SuperAdmin.'}</p>
        {passwordChangeRequired
          ? <form noValidate onSubmit={passwordForm.handleSubmit(submitPasswordChange)}>
            <div className="form-field"><label htmlFor="currentPassword">Current password</label><input id="currentPassword" type="password" autoComplete="current-password" {...passwordForm.register('currentPassword')} />{passwordForm.formState.errors.currentPassword && <p className="field-error" role="alert">{passwordForm.formState.errors.currentPassword.message}</p>}</div>
            <div className="form-field"><label htmlFor="newPassword">New password</label><input id="newPassword" type="password" autoComplete="new-password" {...passwordForm.register('newPassword')} />{passwordForm.formState.errors.newPassword && <p className="field-error" role="alert">{passwordForm.formState.errors.newPassword.message}</p>}</div>
            <div className="form-field"><label htmlFor="confirmPassword">Confirm new password</label><input id="confirmPassword" type="password" autoComplete="new-password" {...passwordForm.register('confirmPassword')} />{passwordForm.formState.errors.confirmPassword && <p className="field-error" role="alert">{passwordForm.formState.errors.confirmPassword.message}</p>}</div>
            <Button type="submit" className="login-submit" disabled={busy}>{busy ? 'Updating…' : 'Update password'}<ArrowRight size={17} /></Button>
          </form>
          : <form noValidate onSubmit={handleSubmit(submitLogin)}>
            <div className="form-field"><label htmlFor="username">Username or email</label><input id="username" autoComplete="username" placeholder="Enter your username or email" aria-invalid={!!errors.username} aria-describedby={errors.username ? 'username-error' : undefined} {...register('username')} />{errors.username && <p id="username-error" className="field-error" role="alert">{errors.username.message}</p>}</div>
            <div className="form-field"><div className="field-label-row"><label htmlFor="password">Password</label><button type="button" className="text-button" onClick={() => setForgot(true)}>Forgot password?</button></div><div className="password-input"><input id="password" type={showPassword ? 'text' : 'password'} autoComplete="current-password" placeholder="Enter your password" aria-invalid={!!errors.password} aria-describedby={errors.password ? 'password-error' : undefined} {...register('password')} /><button type="button" aria-label={showPassword ? 'Hide password' : 'Show password'} aria-pressed={showPassword} onClick={() => setShowPassword(v => !v)}>{showPassword ? <EyeOff size={18} /> : <Eye size={18} />}</button></div>{errors.password && <p id="password-error" className="field-error" role="alert">{errors.password.message}</p>}</div>
            <Button type="submit" className="login-submit" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}<ArrowRight size={17} /></Button>
          </form>}
        {message && <p role="alert" className="notice notice-amber">{message}</p>}
        <div className="preview-divider"><span>REVIEW THE FOUNDATION</span></div>
        <Button asChild variant="outline" className="login-submit"><Link to="/app/dashboard">Explore interface preview<ArrowRight size={16} /></Link></Button>
        <p className="login-note"><ShieldCheck size={16} />Preview screens contain no real business data. Account access is protected by secure server sessions.</p>
      </div><footer>VOLTA FinBook <span>One company. One connected workspace.</span></footer>
    </section>
    <Dialog open={forgot} onOpenChange={setForgot} title="Password recovery" description="Self-service recovery is not available. Contact your system administrator to reset your account password."><Button onClick={() => setForgot(false)}>Back to sign in</Button></Dialog>
  </main>
}
