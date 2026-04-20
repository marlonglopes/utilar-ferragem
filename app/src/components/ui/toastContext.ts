import { createContext } from 'react'

export type ToastVariant = 'success' | 'error' | 'info' | 'warning'

export interface ToastContextValue {
  toast: (message: string, variant?: ToastVariant, duration?: number) => void
}

export const ToastContext = createContext<ToastContextValue | null>(null)
