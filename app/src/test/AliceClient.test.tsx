import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { sendToLara } from '@/lib/alice'
import { AliceBalcao } from '@/components/assistant/AliceBalcao'
import { useCartStore } from '@/store/cartStore'

beforeEach(() => {
  useCartStore.setState({ items: [] })
})

describe('sendToLara (modo mock — sem VITE_ASSISTANT_URL)', () => {
  it('devolve materiais estruturados para uma pergunta de cálculo', async () => {
    const res = await sendToLara('quanto de cimento para 20 m² de contrapiso?', [])
    expect(res.mode).toBe('cliente')
    expect(res.fundamentado).toBe(true)
    expect(res.materiais).toHaveLength(2)
    expect(res.materiais?.[0].embalagens).toBe(4)
    expect(res.complementos?.[0].motivo).toBeTruthy()
  })

  it('anexa aviso de segurança quando a pergunta toca elemento estrutural', async () => {
    const res = await sendToLara('qual a bitola de ferro da viga?', [])
    expect(res.avisos?.length).toBeGreaterThan(0)
    expect(res.avisos?.[0]).toMatch(/ESTRUTURAL/)
  })

  it('saudação não traz materiais nem avisos', async () => {
    const res = await sendToLara('oi', [])
    expect(res.materiais).toBeUndefined()
    expect(res.avisos).toEqual([])
    expect(res.reply).toMatch(/sou a Alice/i)
  })
})

// Com backend configurado: é o Bearer token que habilita o modo vendedor.
// O módulo lê a env no load, então precisa ser reimportado com a env trocada.
describe('sendToLara (com backend) — autenticação', () => {
  async function importarComURL() {
    vi.resetModules()
    vi.stubEnv('VITE_ASSISTANT_URL', 'https://alice.test')
    return import('@/lib/alice')
  }

  function responder(body: Record<string, unknown>) {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => body,
    })
    vi.stubGlobal('fetch', fetchMock)
    return fetchMock
  }

  afterEach(() => {
    vi.unstubAllEnvs()
    vi.unstubAllGlobals()
    vi.resetModules()
    localStorage.clear()
  })

  it('envia Authorization quando há token do authStore no localStorage', async () => {
    localStorage.setItem(
      'utilar-auth',
      JSON.stringify({ state: { user: { id: 'u1', token: 'jwt-abc' } }, version: 0 })
    )
    const fetchMock = responder({ reply: 'ok', products: [], model: 'x', mode: 'vendedor' })

    const { sendToLara: send } = await importarComURL()
    const res = await send('tem em estoque?', [])

    const headers = fetchMock.mock.calls[0][1].headers as Record<string, string>
    expect(headers.Authorization).toBe('Bearer jwt-abc')
    expect(res.mode).toBe('vendedor')
  })

  it('sem token, não manda header algum e o modo cai para cliente', async () => {
    const fetchMock = responder({ reply: 'ok', products: [], model: 'x' })

    const { sendToLara: send } = await importarComURL()
    const res = await send('oi', [])

    const headers = fetchMock.mock.calls[0][1].headers as Record<string, string>
    expect(headers.Authorization).toBeUndefined()
    // `mode` ausente na resposta nunca vira vendedor por acidente.
    expect(res.mode).toBe('cliente')
    expect(res.fundamentado).toBe(false)
  })

  it('localStorage corrompido não derruba a chamada', async () => {
    localStorage.setItem('utilar-auth', '{isso não é json')
    const fetchMock = responder({ reply: 'ok', products: [], model: 'x' })

    const { sendToLara: send } = await importarComURL()
    await expect(send('oi', [])).resolves.toBeTruthy()
    const headers = fetchMock.mock.calls[0][1].headers as Record<string, string>
    expect(headers.Authorization).toBeUndefined()
  })
})

describe('AliceBalcao', () => {
  function renderBalcao() {
    return render(
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <AliceBalcao />
      </MemoryRouter>
    )
  }

  it('é um painel embutido — sem botão flutuante de abrir/fechar', () => {
    renderBalcao()
    expect(screen.getByRole('region', { name: /assistente alice do balcão/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /abrir assistente/i })).not.toBeInTheDocument()
    // já começa com a conversa visível, sem clique
    expect(screen.getByText(/sou a Alice/i)).toBeInTheDocument()
  })

  it('responde e renderiza a lista de materiais com total', async () => {
    renderBalcao()
    const input = screen.getByLabelText(/mensagem para a alice no balcão/i)
    fireEvent.change(input, { target: { value: 'quanto de cimento para o contrapiso?' } })
    fireEvent.click(screen.getByRole('button', { name: /enviar pergunta/i }))

    await waitFor(() => {
      expect(screen.getByRole('region', { name: /lista de materiais/i })).toBeInTheDocument()
    })
    // 4 sacos × R$ 42,90 = R$ 171,60 (areia não tem produto casado no mock)
    expect(screen.getByTestId('material-total')).toHaveTextContent('R$ 171,60')
  })

  it('atalho de pergunta dispara a conversa em 1 toque', async () => {
    renderBalcao()
    fireEvent.click(screen.getByRole('button', { name: /quanto de cimento para 20 m²/i }))
    await waitFor(() => {
      expect(screen.getByRole('region', { name: /lista de materiais/i })).toBeInTheDocument()
    })
  })
})
