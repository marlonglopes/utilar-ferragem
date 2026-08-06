import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import { describe, it, expect, vi } from 'vitest'
import i18n from '@/i18n'
import BalcaoSuccessPage from '@/pages/balcao/BalcaoSuccessPage'
import { ChargeModal } from '@/components/balcao/ChargeModal'
import { computeBalcaoPricing, type BalcaoItem } from '@/store/balcaoStore'
import type { PaymentResult } from '@/hooks/usePayment'

function renderSuccess(state: Record<string, unknown>) {
  return render(
    <MemoryRouter initialEntries={[{ pathname: '/balcao/venda-concluida', state }]}>
      <BalcaoSuccessPage />
    </MemoryRouter>
  )
}

describe('BalcaoSuccessPage — não anuncia "concluída" o que está pendente', () => {
  it('venda pendente de aprovação: header honesto, não "Venda concluída"', () => {
    renderSuccess({
      total: 100,
      requiresApproval: true,
      approvalStatus: 'pending',
      method: 'external',
    })
    expect(screen.getByText('Pendente de aprovação')).toBeInTheDocument()
    expect(screen.queryByText('Venda concluída')).not.toBeInTheDocument()
  })

  it('venda finalizada: "Venda concluída"', () => {
    renderSuccess({ total: 100, approvalStatus: 'not_required', method: 'external', nsu: '004512' })
    expect(screen.getByText('Venda concluída')).toBeInTheDocument()
    expect(screen.queryByText('Pendente de aprovação')).not.toBeInTheDocument()
  })
})

describe('ChargeModal — métodos de cobrança no balcão', () => {
  const item: BalcaoItem = {
    productId: 'p1',
    sku: 'FER-1',
    name: 'Furadeira',
    icon: '⚒',
    unit: 'un',
    unitPrice: 100,
    unitCost: 60,
    costIsEstimated: false,
    quantity: 1,
    stock: 10,
    addedAt: new Date().toISOString(),
  }
  const pricing = computeBalcaoPricing({ items: [item], discountPct: 0, ceilingPct: 12 })

  // Envolve com i18n porque o CardPayment (reutilizado da web) usa useTranslation.
  function renderModal(props: Partial<React.ComponentProps<typeof ChargeModal>>) {
    return render(
      <I18nextProvider i18n={i18n}>
        <ChargeModal
          open
          onClose={() => {}}
          pricing={pricing}
          submitting={false}
          error=""
          paymentResult={null}
          onConfirm={vi.fn()}
          onDone={vi.fn()}
          {...props}
        />
      </I18nextProvider>
    )
  }

  const cardResult: PaymentResult = {
    paymentId: 'm',
    provider: 'mock',
    method: 'card',
    status: 'pending',
    initPoint: '#',
  }

  it('oferece Pix, Cartão (digitado), Boleto e Maquininha', () => {
    renderModal({})
    expect(screen.getByText('Pix')).toBeInTheDocument()
    expect(screen.getByText('Cartão')).toBeInTheDocument()
    expect(screen.getByText('Boleto')).toBeInTheDocument()
    expect(screen.getByText('Maquininha')).toBeInTheDocument()
  })

  it('cartão digitado: com resultado de cartão, mostra o formulário e permite simular (mock)', () => {
    const onSimulateConfirm = vi.fn()
    renderModal({ paymentResult: cardResult, onSimulateConfirm })
    // CardPayment (branch mock) mostra o botão de simular aprovação.
    fireEvent.click(screen.getByText(/Simular aprovação/i))
    expect(onSimulateConfirm).toHaveBeenCalled()
    // Cartão em aberto NÃO oferece "concluir mesmo assim" — não fecha sem aprovar.
    expect(screen.queryByText('Concluir mesmo assim')).not.toBeInTheDocument()
  })

  it('cartão confirmado: mostra "Pagamento confirmado" e o botão de concluir', () => {
    renderModal({ paymentResult: { ...cardResult, status: 'confirmed' } })
    expect(screen.getByText('Pagamento confirmado')).toBeInTheDocument()
    expect(screen.getByText('Concluir venda')).toBeInTheDocument()
  })
})
