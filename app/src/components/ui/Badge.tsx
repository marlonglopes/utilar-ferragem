import { type HTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

export type BadgeVariant = 'default' | 'success' | 'warning' | 'danger' | 'info' | 'orange' | 'blue'

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: BadgeVariant
}

const variants: Record<BadgeVariant, string> = {
  default: 'bg-gray-100 text-gray-700',
  success: 'bg-green-100 text-green-700',
  warning: 'bg-yellow-100 text-yellow-800',
  danger: 'bg-red-100 text-red-700',
  info: 'bg-blue-100 text-blue-700',
  orange: 'bg-orange-100 text-brand-orange',
  blue: 'bg-brand-blue-light text-brand-blue',
}

function Badge({ variant = 'default', className, ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold',
        variants[variant],
        className
      )}
      {...props}
    />
  )
}

export { Badge }
