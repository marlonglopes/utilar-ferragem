import { useState, useRef, useEffect, type FormEvent } from 'react'
import { Sparkles, Send } from 'lucide-react'
import { useAlice } from '@/hooks/useAlice'
import { AliceMessage, ModoBadge } from './AliceMessage'

/** Perguntas de balcão que o vendedor faz o dia inteiro — 1 toque, sem digitar. */
const ATALHOS = [
  'Quanto de cimento para 20 m² de contrapiso?',
  'O que vai junto com assentamento de piso?',
  'Tem em estoque?',
]

/**
 * Alice no PDV — variante EMBUTIDA, para ficar ao lado do carrinho do balcão.
 *
 * Diferenças em relação à AliceBubble, todas deliberadas:
 * - Painel fixo na coluna, não bolha flutuante. No balcão a Alice é ferramenta
 *   de trabalho: escondê-la atrás de um botão custa uma venda por atendimento.
 * - Layout mais denso (texto menor, menos respiro) — a tela do PDV já disputa
 *   espaço com a comanda.
 * - Atalhos de pergunta, porque o vendedor está com o cliente na frente e não
 *   vai digitar uma frase inteira.
 *
 * A LÓGICA é a mesma da bolha (mesmo hook, mesmas regras de aviso e cálculo):
 * o que muda é o layout, nunca o conteúdo nem as travas de segurança.
 */
export function AliceBalcao({ className = '' }: { className?: string }) {
  const { messages, loading, mode, send } = useAlice()
  const [input, setInput] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' })
  }, [messages, loading])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const text = input
    setInput('')
    void send(text)
  }

  return (
    <section
      aria-label="Assistente Alice do balcão"
      className={`flex h-full min-h-0 flex-col bg-white ${className}`}
    >
      <header className="flex items-center gap-2 border-b border-gray-200 bg-brand-blue px-3 py-2 text-white">
        <Sparkles className="h-4 w-4 text-brand-gold" aria-hidden="true" />
        <span className="font-display text-sm font-bold">Alice</span>
        <ModoBadge mode={mode} />
        <span className="ml-auto text-[10px] text-white/70">consulta técnica</span>
      </header>

      <div ref={scrollRef} className="flex-1 space-y-2 overflow-y-auto p-2.5">
        {messages.map((m, i) => (
          <AliceMessage key={i} message={m} maxProducts={6} />
        ))}
        {loading && (
          <div className="rounded-lg bg-gray-100 px-2.5 py-1.5 text-[13px] text-gray-400">
            Alice está consultando…
          </div>
        )}
      </div>

      <div className="flex flex-wrap gap-1 border-t border-gray-100 px-2 pt-2">
        {ATALHOS.map((a) => (
          <button
            key={a}
            type="button"
            onClick={() => void send(a)}
            disabled={loading}
            className="rounded-full border border-gray-200 px-2 py-1 text-[11px] text-gray-600 hover:border-brand-orange hover:text-brand-orange focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-orange disabled:opacity-50"
          >
            {a}
          </button>
        ))}
      </div>

      <form onSubmit={handleSubmit} className="flex items-center gap-2 p-2">
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Perguntar à Alice…"
          aria-label="Mensagem para a Alice no balcão"
          className="flex-1 rounded-lg bg-gray-100 px-3 py-1.5 text-[13px] focus:outline-none focus:ring-2 focus:ring-brand-orange"
        />
        <button
          type="submit"
          disabled={loading || !input.trim()}
          aria-label="Enviar pergunta"
          className="flex h-8 w-8 items-center justify-center rounded-lg bg-brand-orange text-white disabled:opacity-50"
        >
          <Send className="h-4 w-4" aria-hidden="true" />
        </button>
      </form>
    </section>
  )
}
