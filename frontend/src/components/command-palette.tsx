import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Command } from 'cmdk'
import { ArrowUpRight, Search } from 'lucide-react'
import { Dialog } from './ui/dialog'
import { allPages, pagePath } from '@/lib/navigation'

export function CommandPalette({ open, setOpen }: { open: boolean; setOpen: (value: boolean) => void }) {
  const navigate = useNavigate()
  const [search, setSearch] = useState('')
  useEffect(() => { if (!open) setSearch('') }, [open])
  return <Dialog open={open} onOpenChange={setOpen} title="Search your workspace" description="Navigate to a screen. Ticket, PNR, passenger and invoice lookup will be enabled with sales." className="command-dialog">
    <Command label="Workspace navigation"><div className="command-input"><Search size={18} /><Command.Input value={search} onValueChange={setSearch} placeholder="Find a screen, master or report…" /></div>
      <Command.List><Command.Empty>No matching screens. Try “sales” or “customer”.</Command.Empty><Command.Group heading="SCREENS">{allPages.map(page => <Command.Item key={page.id} value={`${page.group} ${page.label}`} onSelect={() => { navigate(pagePath(page)); setOpen(false) }}><span><b>{page.label}</b><small>{page.group}{page.deferred ? ' · Deferred' : ''}</small></span><ArrowUpRight size={16} /></Command.Item>)}</Command.Group></Command.List>
      <div className="command-footer"><span><kbd>↑</kbd><kbd>↓</kbd> to navigate</span><span><kbd>↵</kbd> to open <kbd>Esc</kbd> to close</span></div>
    </Command>
  </Dialog>
}
