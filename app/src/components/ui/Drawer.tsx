import { type ReactNode, useEffect } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/cn'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export type DrawerSide = 'left' | 'right' | 'bottom'

export interface DrawerProps {
  open: boolean
  onClose: () => void
  title?: string
  side?: DrawerSide
  className?: string
  children: ReactNode
}

const sideClasses: Record<DrawerSide, { panel: string; enter: string; exit: string }> = {
  right: {
    panel: 'right-0 top-0 h-full w-full max-w-sm',
    enter: 'translate-x-0',
    exit: 'translate-x-full',
  },
  left: {
    panel: 'left-0 top-0 h-full w-full max-w-sm',
    enter: 'translate-x-0',
    exit: '-translate-x-full',
  },
  bottom: {
    panel: 'bottom-0 left-0 right-0 w-full max-h-[90vh] rounded-t-2xl',
    enter: 'translate-y-0',
    exit: 'translate-y-full',
  },
}

function Drawer({ open, onClose, title, side = 'right', className, children }: DrawerProps) {
  const { t } = useTranslation()
  const cfg = sideClasses[side]

  useEffect(() => {
    if (!open) return
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handleKey)
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', handleKey)
      document.body.style.overflow = ''
    }
  }, [open, onClose])

  if (!open) return null

  return createPortal(
    <div role="dialog" aria-modal="true" className="fixed inset-0 z-50">
      <div
        className="absolute inset-0 bg-black/40 backdrop-blur-sm"
        onClick={onClose}
        aria-hidden="true"
      />
      <div
        className={cn(
          'absolute bg-white shadow-xl transition-transform duration-300',
          cfg.panel,
          open ? cfg.enter : cfg.exit,
          className
        )}
      >
        <div className="flex items-center justify-between px-4 py-3 border-b border-gray-100">
          {title && <h2 className="text-base font-semibold text-gray-900">{title}</h2>}
          <button
            onClick={onClose}
            className="ml-auto rounded-lg p-1 text-gray-400 hover:text-gray-700 hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-brand-orange"
            aria-label={t('close')}
          >
            <X className="h-5 w-5" aria-hidden />
          </button>
        </div>
        <div className={cn('overflow-y-auto', side === 'bottom' ? 'max-h-[calc(90vh-56px)]' : 'h-[calc(100vh-56px)]')}>
          <div className="p-4">{children}</div>
        </div>
      </div>
    </div>,
    document.body
  )
}

export { Drawer }
