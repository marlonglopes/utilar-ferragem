import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/cn'
import { X, CheckCircle, AlertCircle, Info, AlertTriangle } from 'lucide-react'

export type ToastVariant = 'success' | 'error' | 'info' | 'warning'

export interface ToastItem {
  id: string
  message: string
  variant?: ToastVariant
  duration?: number
}

interface ToastContextValue {
  toast: (message: string, variant?: ToastVariant, duration?: number) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

const icons: Record<ToastVariant, typeof CheckCircle> = {
  success: CheckCircle,
  error: AlertCircle,
  info: Info,
  warning: AlertTriangle,
}

const variantClasses: Record<ToastVariant, string> = {
  success: 'bg-green-50 border-green-200 text-green-800',
  error: 'bg-red-50 border-red-200 text-red-800',
  info: 'bg-blue-50 border-blue-200 text-blue-800',
  warning: 'bg-yellow-50 border-yellow-200 text-yellow-800',
}

const iconClasses: Record<ToastVariant, string> = {
  success: 'text-green-500',
  error: 'text-red-500',
  info: 'text-blue-500',
  warning: 'text-yellow-500',
}

function ToastMessage({ item, onDismiss }: { item: ToastItem; onDismiss: (id: string) => void }) {
  const variant = item.variant ?? 'info'
  const Icon = icons[variant]
  const timer = useRef<ReturnType<typeof setTimeout>>()

  useEffect(() => {
    timer.current = setTimeout(() => onDismiss(item.id), item.duration ?? 4000)
    return () => clearTimeout(timer.current)
  }, [item.id, item.duration, onDismiss])

  return (
    <div
      role="alert"
      className={cn(
        'flex items-start gap-3 rounded-xl border px-4 py-3 shadow-lg min-w-[280px] max-w-sm',
        variantClasses[variant]
      )}
    >
      <Icon className={cn('h-5 w-5 flex-shrink-0 mt-0.5', iconClasses[variant])} aria-hidden />
      <p className="flex-1 text-sm font-medium">{item.message}</p>
      <button
        onClick={() => onDismiss(item.id)}
        className="flex-shrink-0 rounded p-0.5 opacity-60 hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-current"
        aria-label="Fechar"
      >
        <X className="h-4 w-4" aria-hidden />
      </button>
    </div>
  )
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([])

  const dismiss = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const toast = useCallback(
    (message: string, variant: ToastVariant = 'info', duration = 4000) => {
      const id = Math.random().toString(36).slice(2)
      setToasts((prev) => [...prev, { id, message, variant, duration }])
    },
    []
  )

  return (
    <ToastContext.Provider value={{ toast }}>
      {children}
      {createPortal(
        <div
          aria-live="polite"
          aria-atomic="false"
          className="fixed bottom-4 right-4 z-[60] flex flex-col gap-2 items-end"
        >
          {toasts.map((item) => (
            <ToastMessage key={item.id} item={item} onDismiss={dismiss} />
          ))}
        </div>,
        document.body
      )}
    </ToastContext.Provider>
  )
}

export function useToast() {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used within ToastProvider')
  return ctx
}
