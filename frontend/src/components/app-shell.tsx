import { useEffect, useRef, useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { Bell, ChevronDown, ChevronRight, Command, HelpCircle, LogOut, Menu, Moon, MoveUpRight, PanelLeftClose, Sun, UsersRound, X } from 'lucide-react'
import { allPages, dashboardPage, LayoutDashboard, pagePath, sections } from '@/lib/navigation'
import { readPreference, savePreference } from '@/lib/utils'
import { Button } from './ui/button'
import { Dialog } from './ui/dialog'
import { CommandPalette } from './command-palette'
import { SalesDraftProvider } from './sales-draft-context'
import { useDatabaseStatus } from '@/hooks/use-database-status'
import '@/shell.css'

const sidebarSections = sections.filter(section => !['international', 'domestic', 'miscellaneous'].includes(section.id))

export function AppShell() {
  const location = useLocation()
  const navigate = useNavigate()
  const databaseStatus = useDatabaseStatus()
  const page = allPages.find(p => `/app/${p.id}` === location.pathname)
  const [identity, setIdentity] = useState<{ displayName: string; username: string } | null>(null)
  const [sessionState, setSessionState] = useState<'checking' | 'signed-in' | 'signed-out' | 'unavailable'>('checking')
  const [logoutError, setLogoutError] = useState(false)
  const [collapsed, setCollapsed] = useState(() => readPreference('volta.sidebar', 'expanded') === 'collapsed')
  const [mobileOpen, setMobileOpen] = useState(false)
  const [narrow, setNarrow] = useState(() => window.matchMedia('(max-width: 720px)').matches)
  const [commandOpen, setCommandOpen] = useState(false)
  const [notifications, setNotifications] = useState(false)
  const [help, setHelp] = useState(false)
  const [expandedGroups, setExpandedGroups] = useState<string[]>(() => {
    try {
      const saved: unknown = JSON.parse(readPreference('volta.navigation', '[]'))
      return Array.isArray(saved) ? saved.filter((id): id is string => typeof id === 'string' && sidebarSections.some(section => section.id === id)) : []
    } catch { return [] }
  })
  const [theme, setTheme] = useState(() => readPreference('volta.theme', 'light'))
  const menuTrigger = useRef<HTMLButtonElement>(null)
  const sidebarRef = useRef<HTMLElement>(null)
  const navigationCollapsed = collapsed && !narrow
  useEffect(() => {
    const controller = new AbortController()
    void fetch('/api/v1/auth/me', { cache: 'no-store', signal: controller.signal }).then(async response => {
      if (response.status === 401) { setSessionState('signed-out'); return }
      if (!response.ok) { setSessionState('unavailable'); return }
      const body: unknown = await response.json()
      if (typeof body !== 'object' || body === null || !('user' in body) || typeof body.user !== 'object' || body.user === null ||
        !('displayName' in body.user) || typeof body.user.displayName !== 'string' ||
        !('username' in body.user) || typeof body.user.username !== 'string') {
        setSessionState('unavailable')
        return
      }
      setIdentity({ displayName: body.user.displayName, username: body.user.username })
      setSessionState('signed-in')
    }).catch(() => {
      if (!controller.signal.aborted) setSessionState('unavailable')
    })
    return () => controller.abort()
  }, [])
  useEffect(() => {
    const media = window.matchMedia('(max-width: 720px)')
    const update = () => { setNarrow(media.matches); if (!media.matches) setMobileOpen(false) }
    media.addEventListener('change', update)
    return () => media.removeEventListener('change', update)
  }, [])
  useEffect(() => { document.documentElement.dataset.theme = theme; savePreference('volta.theme', theme) }, [theme])
  useEffect(() => { savePreference('volta.sidebar', collapsed ? 'collapsed' : 'expanded') }, [collapsed])
  useEffect(() => { savePreference('volta.navigation', JSON.stringify(expandedGroups)) }, [expandedGroups])
  useEffect(() => { setMobileOpen(false); document.getElementById('main-content')?.focus({ preventScroll: true }); window.scrollTo(0, 0); document.title = `${page?.label ?? 'Page not found'} · VOLTA FinBook` }, [location.pathname, page?.label])
  useEffect(() => {
    if (!mobileOpen) return
    const sidebar = sidebarRef.current
    const focusable = () => Array.from(sidebar?.querySelectorAll<HTMLElement>('a[href], button:not([disabled])') ?? []).filter(el => el.getClientRects().length > 0)
    focusable()[0]?.focus()
    const trapFocus = (e: KeyboardEvent) => {
      if (e.key !== 'Tab') return
      const items = focusable()
      const first = items[0], last = items.at(-1)
      if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last?.focus() }
      else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first?.focus() }
    }
    document.addEventListener('keydown', trapFocus)
    return () => { document.removeEventListener('keydown', trapFocus); requestAnimationFrame(() => menuTrigger.current?.focus()) }
  }, [mobileOpen])
  useEffect(() => {
    const keydown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setCommandOpen(v => !v) }
      if (e.key === 'Escape' && mobileOpen) { setMobileOpen(false) }
    }
    document.addEventListener('keydown', keydown)
    return () => document.removeEventListener('keydown', keydown)
  }, [mobileOpen])
  useEffect(() => {
    const group = sidebarSections.find(s => s.pages.some(p => `/app/${p.id}` === location.pathname))
    if (group) setExpandedGroups(prev => prev.includes(group.id) ? prev : [...prev, group.id])
  }, [location.pathname])
  const toggleGroup = (id: string) => {
    if (navigationCollapsed) { setCollapsed(false); setExpandedGroups(prev => prev.includes(id) ? prev : [...prev, id]); return }
    setExpandedGroups(prev => prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id])
  }
  const signOut = async () => {
    setLogoutError(false)
    try {
      const response = await fetch('/api/v1/auth/logout', { method: 'POST', cache: 'no-store' })
      if (!response.ok) { setLogoutError(true); return }
      setIdentity(null)
      setSessionState('signed-out')
      navigate('/login')
    } catch {
      setLogoutError(true)
    }
  }

  return <SalesDraftProvider><div className={`app-layout ${collapsed ? 'sidebar-collapsed' : ''}`}>
    <a className="skip-link" href="#main-content">Skip to content</a>
    {mobileOpen && <button className="sidebar-backdrop" aria-label="Close navigation" onClick={() => setMobileOpen(false)} />}
    <aside ref={sidebarRef} inert={narrow && !mobileOpen} className={`sidebar ${mobileOpen ? 'mobile-open' : ''}`} aria-label="Main navigation">
      <Link to={pagePath(dashboardPage)} className="brand" aria-label="VOLTA FinBook overview"><span className="brand-mark">V<MoveUpRight size={12} /></span><span className="brand-label">VOLTA<span className="brand-sub">FinBook</span></span></Link>
      <nav><NavLink to="/app/dashboard" className="nav-item" title="Overview"><LayoutDashboard size={18} /><span>Overview</span></NavLink>
        <p className="nav-heading">OPERATIONS & FINANCE</p>
        {sidebarSections.map(section => ['sales', 'reconciliation'].includes(section.id) ? <NavLink key={section.id} to={pagePath(section.pages[0])} className="nav-item" title={section.pages[0].label}><section.icon size={18} aria-hidden="true" /><span>{section.pages[0].label}</span></NavLink> : <div key={section.id} className={`nav-section ${section.id === 'masters' ? 'nav-divider' : ''}`}>
          <button className={`nav-item nav-group ${section.pages.some(p => `/app/${p.id}` === location.pathname) ? 'group-active' : ''}`} title={section.label} onClick={() => toggleGroup(section.id)} aria-expanded={expandedGroups.includes(section.id) && !navigationCollapsed} aria-label={section.label} aria-controls={!navigationCollapsed && expandedGroups.includes(section.id) ? `nav-${section.id}` : undefined}><section.icon size={18} aria-hidden="true" /><span>{section.label}</span>{section.id === 'mir' && <small className="nav-deferred">Later</small>}<ChevronRight size={14} className={`nav-chevron ${expandedGroups.includes(section.id) ? 'rotated' : ''}`} /></button>
          {!navigationCollapsed && expandedGroups.includes(section.id) && <div className="nav-children" id={`nav-${section.id}`}>{section.pages.map(p => <NavLink key={p.id} to={pagePath(p)}>{p.label}{p.deferred && <span className="deferred-dot" />}</NavLink>)}</div>}
        </div>)}
      </nav>
      <div className="sidebar-bottom"><button className="nav-item collapse-button" onClick={() => setCollapsed(v => !v)} title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'} aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}>{collapsed ? <ChevronRight size={18} /> : <PanelLeftClose size={18} />}<span>Collapse sidebar</span></button><div className="sidebar-version"><span className="status-dot" />Manual entry & MIR <small>v0.1</small></div></div>
    </aside>
    <div className="workspace" inert={narrow && mobileOpen}>
      <header className="topbar">
        <button ref={menuTrigger} className="icon-button mobile-menu" aria-label={mobileOpen ? 'Close navigation' : 'Open navigation'} aria-expanded={mobileOpen} onClick={() => setMobileOpen(v => !v)}>{mobileOpen ? <X size={20} /> : <Menu size={20} />}</button>

        <div className="topbar-controls"><button className="icon-button" onClick={() => setTheme(v => v === 'dark' ? 'light' : 'dark')} aria-label={theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}>{theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}</button><button className="icon-button" aria-label="Notifications" onClick={() => setNotifications(true)}><Bell size={19} /></button>
          <DropdownMenu.Root><DropdownMenu.Trigger asChild><button className="profile-button" aria-label="User menu"><span className="avatar">{identity?.displayName.split(/\s+/).slice(0, 2).map(part => part[0]).join('').toUpperCase() || 'VP'}</span><span className="profile-copy"><strong>{identity?.displayName ?? 'Employee workspace'}</strong><small>{identity?.username ?? (sessionState === 'checking' ? 'Checking session' : sessionState === 'signed-out' ? 'Not signed in' : 'Session unavailable')}</small></span><ChevronDown size={14} /></button></DropdownMenu.Trigger><DropdownMenu.Portal><DropdownMenu.Content className="dropdown-content" align="end" sideOffset={8}><DropdownMenu.Label>{identity ? `Signed in as ${identity.username}` : 'FinBook workspace'}</DropdownMenu.Label><DropdownMenu.Separator /><DropdownMenu.Item asChild><Link to="/app/administration/employees"><UsersRound size={15} />My employee information</Link></DropdownMenu.Item><DropdownMenu.Item onSelect={() => setHelp(true)}><HelpCircle size={15} />Workspace guide</DropdownMenu.Item><DropdownMenu.Item onSelect={() => sessionState === 'signed-out' ? navigate(`/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`) : void signOut()}><LogOut size={15} />{sessionState === 'signed-out' ? 'Sign in' : 'Sign out'}</DropdownMenu.Item></DropdownMenu.Content></DropdownMenu.Portal></DropdownMenu.Root>
        </div>
      </header>
      {logoutError && <p className="session-error" role="alert">Sign-out could not reach the server. Please try again.</p>}
      <main id="main-content" tabIndex={-1}><nav className="breadcrumbs" aria-label="Breadcrumb"><Link to="/app/dashboard">VOLTA FinBook</Link><ChevronRight size={12} />{page && page.id !== 'dashboard' && <><span>{page.group}</span><ChevronRight size={12} /></>}<span aria-current="page">{page?.label ?? 'Page not found'}</span></nav><Outlet /></main>
      <footer className="workspace-footer"><span>VOLTA FinBook <span>·</span> One company workspace</span><span role="status" className={databaseStatus.state === 'connected' ? 'database-connected' : ''}><span className="status-dot" />{databaseStatus.label}</span></footer>
    </div>
    <CommandPalette open={commandOpen} setOpen={setCommandOpen} />
    <Dialog open={notifications} onOpenChange={setNotifications} title="Notifications" description="Approvals, refunds and activity alerts will appear here."><div className="dialog-empty"><Bell size={28} /><h3>No notification feed connected</h3><p>User-specific notifications require configured accounts and workflows.</p></div></Dialog>
    <Dialog open={help} onOpenChange={setHelp} title="Workspace guide" description="Local MySQL workspace · employee and user setup is available."><ul className="guide-list"><li><strong>Workspace foundation</strong><span>Navigation, login, themes, keyboard search, saved views and screen states.</span></li><li><strong>Employee and user setup</strong><span>Create employee records and their linked login accounts.</span></li><li><strong>Manual sales</strong><span>International ticket entry and MIR preview workflows.</span></li><li><strong>Finance controls</strong><span>Posting and reversals remain disabled until their MySQL controls are verified.</span></li><li><strong>Deferred</strong><span>Unconfirmed legacy actions remain unimplemented. MIR reads a locked local folder using the existing parser.</span></li></ul><p className="notice notice-amber">All tax rules: to be verified by finance. No tax rates have been assumed.</p><Button onClick={() => setHelp(false)}>Continue exploring<Command size={15} /></Button></Dialog>
  </div></SalesDraftProvider>
}
