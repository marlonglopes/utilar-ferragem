// Cliente da assistente Lara ✨. Base URL independente (VITE_ASSISTANT_URL);
// vazio = mock leve no cliente (o assistant-service tem o modo mock próprio, mas
// aqui garantimos que a bolha funcione mesmo sem backend).
const ASSISTANT_URL = import.meta.env.VITE_ASSISTANT_URL ?? ''
export const isLaraEnabled = ASSISTANT_URL !== ''

export interface LaraProduct {
  id: string
  slug: string
  name: string
  price: number
  stock: number
  brand?: string | null
  category: string
}

export interface LaraResult {
  reply: string
  products: LaraProduct[]
  model: string
}

export interface LaraTurn {
  role: 'user' | 'assistant'
  text: string
}

export async function sendToLara(message: string, history: LaraTurn[]): Promise<LaraResult> {
  if (!isLaraEnabled) {
    return mockReply(message)
  }
  const res = await fetch(`${ASSISTANT_URL}/api/v1/assistant/chat`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message, history }),
  })
  if (!res.ok) throw new Error(`lara ${res.status}`)
  const data = (await res.json()) as LaraResult
  return { reply: data.reply, products: data.products ?? [], model: data.model }
}

// Mock cliente (sem backend): saudação + orientação. Determinístico p/ testes.
function mockReply(message: string): LaraResult {
  const q = message.toLowerCase()
  const greeting = /\b(oi|olá|ola|bom dia|boa tarde|boa noite|ajuda)\b/.test(q)
  const reply = greeting
    ? 'Oi! Eu sou a Lara ✨, sua ajudante aqui da UtiLar Ferragem. Posso achar ferramentas e materiais, comparar preços e estoque, e montar a lista pra sua obra. O que você procura?'
    : 'Posso te ajudar a encontrar ferramentas e materiais. Me diga o que você precisa — por exemplo "furadeira", "cimento" ou uma categoria como elétrica.'
  return { reply, products: [], model: 'mock-client' }
}
