import { useState, useRef, useEffect, type FormEvent } from 'react'
import { Sparkles, Send, X } from 'lucide-react'
import { useAlice } from '@/hooks/useAlice'
import { AliceMessage, ModoBadge } from './AliceMessage'

/**
 * Alice ✨ — copiloto embarcado da UtiLar (equivalente à "Gi" do Gifthy).
 * Bolha flutuante no canto → painel de chat. Sugere produtos reais do catálogo,
 * calcula lista de material e destaca os avisos de segurança do servidor.
 */
export function AliceBubble() {
  const [open, setOpen] = useState(false)
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
    <>
      {/* Botão flutuante */}
      <button
        onClick={() => setOpen((v) => !v)}
        aria-label={open ? 'Fechar assistente Alice' : 'Abrir assistente Alice'}
        className="fixed bottom-5 right-5 z-50 flex h-14 w-14 items-center justify-center rounded-full bg-brand-orange text-white shadow-lg transition-transform hover:scale-105 hover:bg-brand-orange-dark focus:outline-none focus:ring-2 focus:ring-brand-orange focus:ring-offset-2"
      >
        {open ? <X className="h-6 w-6" /> : <Sparkles className="h-6 w-6" />}
      </button>

      {/* Painel de chat */}
      {open && (
        <div className="fixed bottom-24 right-5 z-50 flex h-[70vh] max-h-[560px] w-[92vw] max-w-[420px] flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl">
          <header className="flex items-center gap-2 bg-brand-blue px-4 py-3 text-white">
            <Sparkles className="h-5 w-5 text-brand-gold" />
            <div className="leading-tight">
              <div className="flex items-center gap-2 font-display font-bold">
                Alice
                <ModoBadge mode={mode} />
              </div>
              <div className="text-[11px] text-white/70">sua ajudante na UtiLar</div>
            </div>
          </header>

          <div ref={scrollRef} className="flex-1 space-y-3 overflow-y-auto p-3">
            {messages.map((m, i) => (
              <AliceMessage key={i} message={m} onNavigate={() => setOpen(false)} />
            ))}
            {loading && (
              <div className="mr-auto max-w-[85%] rounded-2xl rounded-bl-sm bg-gray-100 px-3 py-2 text-sm text-gray-400">
                Alice está digitando…
              </div>
            )}
          </div>

          <form
            onSubmit={handleSubmit}
            className="flex items-center gap-2 border-t border-gray-100 p-2"
          >
            <input
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="Pergunte à Alice…"
              aria-label="Mensagem para a Alice"
              className="flex-1 rounded-full bg-gray-100 px-4 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-brand-orange"
            />
            <button
              type="submit"
              disabled={loading || !input.trim()}
              aria-label="Enviar"
              className="flex h-9 w-9 items-center justify-center rounded-full bg-brand-orange text-white disabled:opacity-50"
            >
              <Send className="h-4 w-4" />
            </button>
          </form>
        </div>
      )}
    </>
  )
}
