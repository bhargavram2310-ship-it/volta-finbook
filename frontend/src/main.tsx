import { Component, StrictMode } from 'react'
import type { ErrorInfo, ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
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
import { InternationalSales } from './pages/international-sales'
import { Employees } from './pages/employees'
import { UsersPage } from './pages/users'
import { RolesPage } from './pages/roles'

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
  <Route path="/app/mir/import" element={<Navigate to="/app/international/mir" replace />} />
  <Route path="/app" element={<AppShell />}><Route index element={<Navigate to="dashboard" replace />} /><Route path="dashboard" element={<Dashboard />} />{allPages.filter(p => p.id !== 'dashboard').map(page => <Route key={page.id} path={page.id} element={page.id === 'administration/mir-source' ? <MirSourceSettings /> : page.id === 'administration/employees' ? <Employees /> : page.id === 'administration/users' ? <UsersPage /> : page.id === 'administration/roles' ? <RolesPage /> : page.id === 'international/sales' || page.id === 'international/mir' ? <InternationalSales /> : <ModulePage key={page.id} page={page} />} />)}<Route path="*" element={<NotFound />} /></Route>
  <Route path="*" element={<NotFound />} />
</Routes></BrowserRouter></ErrorBoundary></StrictMode>)
