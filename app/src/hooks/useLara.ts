import { useCallback, useState } from 'react'
import { sendToLara, type LaraProduct, type LaraTurn } from '@/lib/lara'

export interface LaraMessage {
  role: 'user' | 'assistant'
  text: string
  products?: LaraProduct[]
}

const WELCOME: LaraMessage = {
  role: 'assistant',
  text: 'Oi! Sou a Lara ✨. O que você está procurando hoje?',
}

export function useLara() {
  const [messages, setMessages] = useState<LaraMessage[]>([WELCOME])
  const [loading, setLoading] = useState(false)

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
        setMessages((prev) => [
          ...prev,
          { role: 'assistant', text: res.reply, products: res.products },
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

  return { messages, loading, send }
}
