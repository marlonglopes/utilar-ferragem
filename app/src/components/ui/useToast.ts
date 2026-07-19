import { useContext } from 'react'
import { ToastContext, type ToastContextValue } from './toastContext'

export function useToast() {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used within ToastProvider')
  return ctx
}

// Toast que nunca derruba a árvore quando não há provider.
//
// PORQUÊ: `useToast` lança de propósito — em telas de checkout, um toast que
// some silenciosamente esconde erro de pagamento. Mas componentes de catálogo
// (o coração de favorito no ProductCard) são renderizados isolados em teste e
// embutidos em contextos que não montam o provider. Ali o toast é confirmação
// de conveniência: a fonte de verdade do estado é o próprio botão, que já muda
// de cor e de aria-pressed. Perder o toast não perde informação.
//
// Use este SÓ quando a mensagem for redundante com algo já visível na tela.
const NOOP_TOAST: ToastContextValue = { toast: () => {} }

export function useOptionalToast(): ToastContextValue {
  return useContext(ToastContext) ?? NOOP_TOAST
}
