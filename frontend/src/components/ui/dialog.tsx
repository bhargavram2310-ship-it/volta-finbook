import type { ReactNode } from 'react'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import { X } from 'lucide-react'

export function Dialog({ open, onOpenChange, title, description, children, className = '' }: {
  open: boolean; onOpenChange: (open: boolean) => void; title: string; description: string;
  children: ReactNode; className?: string
}) {
  return <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="dialog-overlay" />
      <DialogPrimitive.Content className={`dialog-content ${className}`}>
        <DialogPrimitive.Title className="dialog-title">{title}</DialogPrimitive.Title>
        <DialogPrimitive.Description className="dialog-description">{description}</DialogPrimitive.Description>
        {children}
        <DialogPrimitive.Close className="dialog-close" aria-label="Close dialog"><X size={18} /></DialogPrimitive.Close>
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  </DialogPrimitive.Root>
}
