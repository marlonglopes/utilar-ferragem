import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import BalcaoApprovalsPage from '@/pages/balcao/BalcaoApprovalsPage'
import { blockedReasonFor, BLOCKED_MESSAGE } from '@/hooks/useBalcaoApprovals'
import { MOCK_OPERATOR, useBalcaoStore } from '@/store/balcaoStore'
import { useAuthStore } from '@/store/authStore'

/**
 * Fila de aprovação de desconto.
 *
 * A regra sob teste é a REGRA 3 do `internal/balcao/authz.go`: **ninguém aprova
 * o próprio desconto** — nem o gerente que fez a venda, nem o admin. O backend
 * responde 403, mas a UI não pode depender disso: um botão que existe e nunca
 * funciona é pior que um botão ausente com o motivo escrito.
 */

const VIEWER_ID = 'mock-operator'

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={client}>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        {children}
      </MemoryRouter>
    </QueryClientProvider>
  )
}

/** Gerente: cargo que homologa. */
function seedApprover() {
  useAuthStore.setState({
    user: { id: VIEWER_ID, email: 'g@loja.com', name: 'Gerente', role: 'admin', token: 't' },
  })
  useBalcaoStore.setState({
    operator: {
      ...MOCK_OPERATOR,
      userId: VIEWER_ID,
      level: 'manager',
      canApproveDiscount: true,
      fromBackend: true,
    },
  })
}

beforeEach(() => {
  seedApprover()
})

describe('blockedReasonFor — espelho de balcao.CanApproveOrder', () => {
  const approver = { userId: 'u-gerente', canApprove: true }

  it('libera o pedido vendido por outra pessoa', () => {
    expect(blockedReasonFor({ operatorId: 'u-vendedor' }, approver)).toBeNull()
  })

  it('BLOQUEIA o pedido vendido pelo próprio aprovador', () => {
    expect(blockedReasonFor({ operatorId: 'u-gerente' }, approver)).toBe('self_approval')
  })

  it('a auto-aprovação vence a checagem de cargo — a ordem importa', () => {
    // O caso perigoso é justamente o gerente que vendeu: ele TEM poder de
    // aprovar, então uma checagem de cargo feita primeiro o deixaria passar.
    // Mesma ordem do backend.
    expect(blockedReasonFor({ operatorId: 'u-gerente' }, { userId: 'u-gerente', canApprove: true }))
      .toBe('self_approval')
  })

  it('quem não tem cargo de aprovação é bloqueado por cargo', () => {
    expect(blockedReasonFor({ operatorId: 'u-outro' }, { userId: 'u-eu', canApprove: false })).toBe(
      'not_approver',
    )
  })

  it('pedido sem operador registrado não vira auto-aprovação por engano', () => {
    expect(blockedReasonFor({ operatorId: undefined }, approver)).toBeNull()
  })
})

describe('BalcaoApprovalsPage', () => {
  it('lista os pedidos pendentes da loja', async () => {
    render(<BalcaoApprovalsPage />, { wrapper })

    expect(
      await screen.findByRole('heading', { name: /aprovações pendentes/i }),
    ).toBeInTheDocument()
    expect(await screen.findByText('BAL-0001')).toBeInTheDocument()
    expect(screen.getByText('BAL-0002')).toBeInTheDocument()
  })

  it('mostra Aprovar/Recusar no pedido vendido por outra pessoa', async () => {
    render(<BalcaoApprovalsPage />, { wrapper })

    const card = (await screen.findByText('BAL-0001')).closest('li')!
    expect(within(card).getByRole('button', { name: /aprovar/i })).toBeEnabled()
    expect(within(card).getByRole('button', { name: /recusar/i })).toBeEnabled()
  })

  it('NÃO oferece aprovação no pedido que o próprio usuário vendeu, e diz por quê', async () => {
    render(<BalcaoApprovalsPage />, { wrapper })

    const card = (await screen.findByText('BAL-0002')).closest('li')!
    expect(within(card).queryByRole('button', { name: /aprovar/i })).not.toBeInTheDocument()
    expect(within(card).queryByRole('button', { name: /recusar/i })).not.toBeInTheDocument()
    expect(within(card).getByText(BLOCKED_MESSAGE.self_approval)).toBeInTheDocument()
  })

  it('aprovar tira o pedido da fila', async () => {
    const user = userEvent.setup()
    render(<BalcaoApprovalsPage />, { wrapper })

    const card = (await screen.findByText('BAL-0001')).closest('li')!
    await user.click(within(card).getByRole('button', { name: /aprovar/i }))

    await waitFor(() => {
      expect(screen.queryByText('BAL-0001')).not.toBeInTheDocument()
    })
    // O pedido bloqueado continua na fila: some por decisão, não por bloqueio.
    expect(screen.getByText('BAL-0002')).toBeInTheDocument()
  })

  it('recusa exige motivo — igual ao backend', async () => {
    const user = userEvent.setup()
    render(<BalcaoApprovalsPage />, { wrapper })

    const card = (await screen.findByText('BAL-0001')).closest('li')!
    await user.click(within(card).getByRole('button', { name: /recusar/i }))

    // Confirmar sem motivo não passa.
    await user.click(await screen.findByRole('button', { name: /confirmar recusa/i }))
    expect(await screen.findByText(/informe o motivo da recusa/i)).toBeInTheDocument()
    expect(screen.getByText('BAL-0001')).toBeInTheDocument()

    await user.type(
      screen.getByLabelText(/motivo da recusa/i),
      'Desconto acima da política para este cliente',
    )
    await user.click(screen.getByRole('button', { name: /confirmar recusa/i }))

    await waitFor(() => {
      expect(screen.queryByText('BAL-0001')).not.toBeInTheDocument()
    })
  })

  it('quem não homologa não vê a fila, vê a explicação', async () => {
    useBalcaoStore.setState({
      operator: { ...MOCK_OPERATOR, userId: VIEWER_ID, canApproveDiscount: false },
    })
    render(<BalcaoApprovalsPage />, { wrapper })

    expect(
      await screen.findByRole('heading', { name: /fila de aprovação restrita/i }),
    ).toBeInTheDocument()
    expect(screen.queryByText('BAL-0001')).not.toBeInTheDocument()
  })
})
