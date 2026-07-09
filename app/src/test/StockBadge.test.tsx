import { render, screen } from '@testing-library/react'
import { describe, it, expect, beforeAll } from 'vitest'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import { StockBadge } from '@/components/catalog/StockBadge'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

function renderBadge(stock: number | null | undefined) {
  return render(
    <I18nextProvider i18n={i18n}>
      <StockBadge stock={stock} />
    </I18nextProvider>
  )
}

describe('StockBadge', () => {
  it('stock null → "Sob consulta"', () => {
    renderBadge(null)
    expect(screen.getByText('Sob consulta')).toBeInTheDocument()
  })

  it('stock 0 → "Sem estoque"', () => {
    renderBadge(0)
    expect(screen.getByText('Sem estoque')).toBeInTheDocument()
  })

  it('stock < 10 → "Últimas unidades" com a quantidade', () => {
    renderBadge(5)
    expect(screen.getByText(/Últimas unidades/)).toBeInTheDocument()
    expect(screen.getByText(/\(5\)/)).toBeInTheDocument()
  })

  it('stock alto → "Em estoque"', () => {
    renderBadge(50)
    expect(screen.getByText('Em estoque')).toBeInTheDocument()
  })
})
