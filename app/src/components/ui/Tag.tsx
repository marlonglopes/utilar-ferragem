import { type HTMLAttributes, type MouseEvent } from 'react'
import { cn } from '@/lib/cn'
import { X } from 'lucide-react'

export interface TagProps extends HTMLAttributes<HTMLSpanElement> {
  onRemove?: () => void
  removable?: boolean
}

function Tag({ onRemove, removable, className, children, ...props }: TagProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-md bg-gray-100 px-2 py-1 text-xs font-medium text-gray-700',
        className
      )}
      {...props}
    >
      {children}
      {(removable || onRemove) && (
        <button
          type="button"
          onClick={(e: MouseEvent) => {
            e.stopPropagation()
            onRemove?.()
          }}
          className="ml-0.5 rounded-sm text-gray-400 hover:text-gray-700 focus:outline-none focus:ring-1 focus:ring-brand-orange"
          aria-label="Remover"
        >
          <X className="h-3 w-3" aria-hidden />
        </button>
      )}
    </span>
  )
}

export { Tag }
