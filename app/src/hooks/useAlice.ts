import { useCallback, useState } from 'react'
import {
  sendToLara,
  type LaraComplemento,
  type LaraMode,
  type LaraProduct,
  type LaraTurn,
  type MaterialItem,
} from '@/lib/alice'

export interface LaraMessage {
  role: 'user' | 'assistant'
  text: string
  products?: LaraProduct[]
  /** Avisos de segurança do servidor — renderizados em destaque, no topo. */
  avisos?: string[]
  /** Lista de materiais, quando o backend a devolve estruturada. */
  materiais?: MaterialItem[]
  complementos?: LaraComplemento[]
  mode?: LaraMode
  fundamentado?: boolean
}

const WELCOME: LaraMessage = {
  role: 'assistant',
  text: 'Oi! Sou a Alice ✨. O que você está procurando hoje?',
}

export function useAlice() {
  const [messages, setMessages] = useState<LaraMessage[]>([WELCOME])
  const [loading, setLoading] = useState(false)
  const [mode, setMode] = useState<LaraMode>('cliente')

  const send = useCallback(
    async (text: string) => {
      const trimmed = text.trim()
      if (!trimmed || loading) return

      const history: LaraTurn[] = messages
        .filter((m) => m.text)
        .map((m) => ({ role: m.role, text: m.text }))

      setMessages((prev) => [...prev, { role: 'user', text: trimmed }])
      setLoading(true)
      try {
        const res = await sendToLara(trimmed, history)
        setMode(res.mode)
        setMessages((prev) => [
          ...prev,
          {
            role: 'assistant',
            text: res.reply,
            products: res.products,
            avisos: res.avisos,
            materiais: res.materiais,
            complementos: res.complementos,
            mode: res.mode,
            fundamentado: res.fundamentado,
          },
        ])
      } catch {
        setMessages((prev) => [
          ...prev,
          { role: 'assistant', text: 'Ops, tive um problema pra responder agora. Tenta de novo?' },
        ])
      } finally {
        setLoading(false)
      }
    },
    [messages, loading]
  )

  return { messages, loading, mode, send }
}
