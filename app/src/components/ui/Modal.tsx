import { type ReactNode, useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/cn'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export type ModalSize = 'sm' | 'md' | 'lg' | 'xl' | 'full'

export interface ModalProps {
  open: boolean
  onClose: () => void
  title?: string
  size?: ModalSize
  className?: string
  children: ReactNode
}

const sizes: Record<ModalSize, string> = {
  sm: 'max-w-sm',
  md: 'max-w-md',
  lg: 'max-w-lg',
  xl: 'max-w-xl',
  full: 'max-w-[95vw]',
}

function Modal({ open, onClose, title, size = 'md', className, children }: ModalProps) {
  const { t } = useTranslation()
  const dialogRef = useRef<HTMLDivElement>(null)

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
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby={title ? 'modal-title' : undefined}
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
    >
      <div
        className="absolute inset-0 bg-black/40 backdrop-blur-sm"
        onClick={onClose}
        aria-hidden="true"
      />
      <div
        ref={dialogRef}
        className={cn(
          'relative z-10 w-full rounded-2xl bg-white shadow-xl',
          sizes[size],
          className
        )}
      >
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-100">
          {title && (
            <h2 id="modal-title" className="text-base font-semibold text-gray-900">
              {title}
            </h2>
          )}
          <button
            onClick={onClose}
            className="ml-auto rounded-lg p-1 text-gray-400 hover:text-gray-700 hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-brand-orange"
            aria-label={t('close')}
          >
            <X className="h-5 w-5" aria-hidden />
          </button>
        </div>
        <div className="px-6 py-4">{children}</div>
      </div>
    </div>,
    document.body
  )
}

export { Modal }
