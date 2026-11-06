import { Component, StrictMode } from 'react'
import type { ErrorInfo, ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes, useSearchParams } from 'react-router-dom'
import '@fontsource/inter/400.css'
import '@fontsource/inter/500.css'
import '@fontsource/inter/600.css'
import '@fontsource/inter/700.css'
import './styles.css'
import { AppShell } from './components/app-shell'
import { ErrorState } from './components/screen-state'
import { Login } from './pages/login'
import { Dashboard } from './pages/dashboard'
import { ModulePage, NotFound } from './pages/module-page'
import { allPages } from './lib/navigation'
import { readPreference } from './lib/utils'
import { MirSourceSettings } from './pages/mir-source-settings'
import { SalesWorkspace } from './pages/sales-workspace'
import { ReconciliationUpload } from './pages/reconciliation-upload'
import type { SalesClass } from './lib/sales-types'
import { Employees } from './pages/employees'
import { UsersPage } from './pages/users'
import { RolesPage } from './pages/roles'
import { MirImport } from './pages/mir-import'
import { MastersPage } from './pages/masters'

function LegacySalesRedirect({ salesClass }: { salesClass: SalesClass }) {
  const [params] = useSearchParams()
  const next = new URLSearchParams(params); next.set('type', salesClass)
  return <Navigate to={`/app/sales/workspace?${next}`} replace />
}

document.documentElement.dataset.theme = readPreference('volta.theme', 'light')

class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }
  static getDerivedStateFromError() { return { failed: true } }
  componentDidCatch(_error: Error, _info: ErrorInfo) { /* Never log credentials or financial payloads. */ }
  render() { return this.state.failed ? <ErrorState retry={() => window.location.reload()} /> : this.props.children }
}

createRoot(document.getElementById('root')!).render(<StrictMode><ErrorBoundary><BrowserRouter><Routes>
  <Route path="/" element={<Navigate to="/login" replace />} />
  <Route path="/login" element={<Login />} />
  <Route path="/app/mir/import" element={<Navigate to="/app/administration/mir-report" replace />} />
  <Route path="/app/international/mir" element={<Navigate to="/app/administration/mir-report" replace />} /><Route path="/app/mir/reports" element={<Navigate to="/app/administration/mir-report" replace />} />
  <Route path="/app" element={<AppShell />}><Route path="international/sales" element={<LegacySalesRedirect salesClass="international" />} /><Route path="domestic/sales" element={<LegacySalesRedirect salesClass="domestic" />} /><Route path="miscellaneous/sales" element={<LegacySalesRedirect salesClass="miscellaneous" />} /><Route index element={<Navigate to="dashboard" replace />} /><Route path="dashboard" element={<Dashboard />} />{allPages.filter(p => p.id !== 'dashboard').map(page => <Route key={page.id} path={page.id} element={page.group === 'Masters' ? <MastersPage key={page.id} page={page} /> : page.id === 'administration/mir-source' ? <MirSourceSettings /> : page.id === 'administration/employees' ? <Employees /> : page.id === 'administration/users' ? <UsersPage /> : page.id === 'administration/roles' ? <RolesPage /> : page.id === 'administration/mir-report' ? <MirImport /> : page.id === 'reconciliation/upload' ? <ReconciliationUpload /> : page.id === 'sales/workspace' ? <SalesWorkspace /> : <ModulePage key={page.id} page={page} />} />)}<Route path="*" element={<NotFound />} /></Route>
  <Route path="*" element={<NotFound />} />
</Routes></BrowserRouter></ErrorBoundary></StrictMode>)
