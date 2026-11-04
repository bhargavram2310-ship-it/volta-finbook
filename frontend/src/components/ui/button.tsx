import type { ButtonHTMLAttributes } from 'react'
import { Slot } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

const buttonVariants = cva('button', {
  variants: {
    variant: { default: 'button-primary', outline: 'button-outline', ghost: 'button-ghost', secondary: 'button-secondary' },
    size: { default: '', sm: 'button-sm', icon: 'button-icon' },
  },
  defaultVariants: { variant: 'default', size: 'default' },
})

export function Button({ className, variant, size, asChild = false, type = 'button', ...props }:
  ButtonHTMLAttributes<HTMLButtonElement> & VariantProps<typeof buttonVariants> & { asChild?: boolean }) {
  const Component = asChild ? Slot : 'button'
  return <Component type={asChild ? undefined : type} className={cn(buttonVariants({ variant, size }), className)} {...props} />
}
