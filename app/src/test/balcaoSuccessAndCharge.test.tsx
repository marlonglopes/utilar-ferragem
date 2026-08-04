import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, it, expect, vi } from 'vitest'
import BalcaoSuccessPage from '@/pages/balcao/BalcaoSuccessPage'
import { ChargeModal } from '@/components/balcao/ChargeModal'
import { computeBalcaoPricing, type BalcaoItem } from '@/store/balcaoStore'

function renderSuccess(state: Record<string, unknown>) {
  return render(
    <MemoryRouter initialEntries={[{ pathname: '/balcao/venda-concluida', state }]}>
      <BalcaoSuccessPage />
    </MemoryRouter>
  )
}

describe('BalcaoSuccessPage — não anuncia "concluída" o que está pendente', () => {
  it('venda pendente de aprovação: header honesto, não "Venda concluída"', () => {
    renderSuccess({ total: 100, requiresApproval: true, approvalStatus: 'pending', method: 'external' })
    expect(screen.getByText('Pendente de aprovação')).toBeInTheDocument()
    expect(screen.queryByText('Venda concluída')).not.toBeInTheDocument()
  })

  it('venda finalizada: "Venda concluída"', () => {
    renderSuccess({ total: 100, approvalStatus: 'not_required', method: 'external', nsu: '004512' })
    expect(screen.getByText('Venda concluída')).toBeInTheDocument()
    expect(screen.queryByText('Pendente de aprovação')).not.toBeInTheDocument()
  })
})

describe('ChargeModal — não oferece método quebrado no balcão', () => {
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
  it('mostra Maquininha e Pix, NÃO mostra "Cartão online" (quebrado com Appmax)', () => {
    render(
      <ChargeModal
        open
        onClose={() => {}}
        pricing={pricing}
        submitting={false}
        error=""
        paymentResult={null}
        onConfirm={vi.fn()}
        onDone={vi.fn()}
      />
    )
    expect(screen.getByText('Maquininha')).toBeInTheDocument()
    expect(screen.getByText('Pix')).toBeInTheDocument()
    expect(screen.queryByText('Cartão online')).not.toBeInTheDocument()
  })
})
