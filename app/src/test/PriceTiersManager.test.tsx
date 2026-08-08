import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'

vi.mock('@/lib/adminProductsApi', () => ({ setPriceTiers: vi.fn() }))

import { setPriceTiers } from '@/lib/adminProductsApi'
import { PriceTiersManager } from '@/components/admin/products/PriceTiersManager'

const mockedSet = vi.mocked(setPriceTiers)

beforeEach(() => mockedSet.mockReset())

describe('PriceTiersManager', () => {
  it('adiciona faixas e salva com o payload correto', async () => {
    mockedSet.mockResolvedValueOnce()
    render(<PriceTiersManager productId="p1" />)

    fireEvent.click(screen.getByRole('button', { name: /adicionar faixa/i }))
    fireEvent.change(screen.getByLabelText(/quantidade mínima da faixa 1/i), {
      target: { value: '10' },
    })
    fireEvent.change(screen.getByLabelText(/preço unitário da faixa 1/i), {
      target: { value: '8,50' },
    })
    fireEvent.click(screen.getByRole('button', { name: /salvar faixas/i }))

    await waitFor(() => expect(mockedSet).toHaveBeenCalledOnce())
    expect(mockedSet).toHaveBeenCalledWith('p1', [{ minQty: 10, price: 8.5 }])
    expect(await screen.findByText(/faixas salvas/i)).toBeInTheDocument()
  })

  it('barra faixa maior mais cara que a menor (não salva)', async () => {
    render(
      <PriceTiersManager
        productId="p1"
        initialTiers={[
          { minQty: 1, price: 10 },
          { minQty: 10, price: 8 },
        ]}
      />
    )
    // Torna a faixa de 10+ MAIS cara (12) que a de 1+ (10) → inválido.
    fireEvent.change(screen.getByLabelText(/preço unitário da faixa 2/i), {
      target: { value: '12' },
    })
    expect(screen.getByText(/não pode ser mais cara/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /salvar faixas/i })).toBeDisabled()
    expect(mockedSet).not.toHaveBeenCalled()
  })

  it('barra quantidade duplicada', () => {
    render(
      <PriceTiersManager
        productId="p1"
        initialTiers={[
          { minQty: 10, price: 9 },
          { minQty: 10, price: 8 },
        ]}
      />
    )
    expect(screen.getByText(/duas faixas com a mesma quantidade/i)).toBeInTheDocument()
  })

  it('faixa pela metade (só quantidade) é erro', () => {
    render(<PriceTiersManager productId="p1" />)
    fireEvent.click(screen.getByRole('button', { name: /adicionar faixa/i }))
    fireEvent.change(screen.getByLabelText(/quantidade mínima da faixa 1/i), {
      target: { value: '5' },
    })
    expect(screen.getByText(/preencha quantidade e preço/i)).toBeInTheDocument()
  })
})
