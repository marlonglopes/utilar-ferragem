import { useEffect, type RefObject } from 'react'

const FOCUSABLE = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

function focusableWithin(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
    (el) => el.offsetParent !== null || el === document.activeElement
  )
}

/**
 * Prende o Tab dentro do container enquanto `active` for true e devolve o foco
 * ao elemento que abriu o diálogo quando ele fecha.
 *
 * Sem isso o teclado continua navegando pela página atrás do overlay — o
 * usuário de leitor de tela "sai" do modal sem perceber que ele ainda está
 * aberto (WCAG 2.4.3 e 2.1.2).
 */
export function useFocusTrap(ref: RefObject<HTMLElement>, active: boolean) {
  useEffect(() => {
    if (!active) return
    const container = ref.current
    if (!container) return

    // Guarda quem tinha o foco para restaurar no fechamento.
    const previouslyFocused = document.activeElement as HTMLElement | null

    // Move o foco para dentro do diálogo. Se não houver nada focável, foca o
    // próprio container (que recebe tabIndex={-1} nos componentes).
    const initial = focusableWithin(container)[0] ?? container
    initial.focus()

    function handleKeyDown(e: KeyboardEvent) {
      if (e.key !== 'Tab' || !container) return

      const items = focusableWithin(container)
      if (items.length === 0) {
        e.preventDefault()
        return
      }

      const first = items[0]
      const last = items[items.length - 1]
      const activeEl = document.activeElement

      if (e.shiftKey && (activeEl === first || activeEl === container)) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && activeEl === last) {
        e.preventDefault()
        first.focus()
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('keydown', handleKeyDown)
      // `isConnected` evita erro quando o elemento anterior já saiu do DOM.
      if (previouslyFocused?.isConnected) previouslyFocused.focus()
    }
  }, [ref, active])
}
