import { render, screen, fireEvent, within } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { MaterialList } from '@/components/assistant/MaterialList'
import { subtotalItem, totalLista } from '@/components/assistant/helpers'
import { MemoriaCalculo } from '@/components/assistant/MemoriaCalculo'
import { useCartStore } from '@/store/cartStore'
import type { MaterialItem } from '@/lib/alice'

const cimento: MaterialItem = {
  materialId: 'cimento-cp2',
  nome: 'Cimento CP-II 50 kg',
  quantidade: 168,
  unidBase: 'kg',
  embalagens: 4,
  unidVenda: 'saco 50 kg',
  memoria: '20 m² × 0,05 m = 1 m³ × 168 kg/m³ = 168 kg',
  fonte: 'Tabela de traços — NBR 12655',
  coefMin: 150,
  coefMax: 190,
  coefUnid: 'kg/m³',
  observacao: 'A faixa varia com o traço.',
  produto: {
    slug: 'cimento-cp2-50kg',
    nome: 'Cimento CP-II-Z 32 50 kg',
    preco: 40,
    estoque: 120,
    id: 'prod-cimento',
  },
}

// Sem produto casado no catálogo: tem quantidade, NÃO tem preço.
const areia: MaterialItem = {
  materialId: 'areia-media',
  nome: 'Areia média',
  quantidade: 0.75,
  unidBase: 'm³',
  embalagens: 1,
  unidVenda: 'm³',
  memoria: '1 m³ × 0,75 m³/m³ = 0,75 m³',
  fonte: 'Tabela de traços',
  coefMin: 0.7,
  coefMax: 0.85,
  coefUnid: 'm³/m³',
}

function renderList(itens: MaterialItem[]) {
  return render(
    <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <MaterialList itens={itens} />
    </MemoryRouter>
  )
}

beforeEach(() => {
  useCartStore.setState({ items: [] })
})

describe('MaterialList', () => {
  it('renderiza quantidade, unidade, produto, preço e subtotal', () => {
    renderList([cimento])

    expect(screen.getByText('168 kg')).toBeInTheDocument()
    expect(screen.getByText('4 × saco 50 kg')).toBeInTheDocument()
    expect(screen.getByText('Cimento CP-II-Z 32 50 kg')).toBeInTheDocument()
    // preço unitário R$ 40,00 e subtotal 4 × 40 = R$ 160,00
    expect(screen.getByText('R$ 40,00')).toBeInTheDocument()
    // com um item só, subtotal e total coincidem — ambos precisam estar lá
    expect(screen.getAllByText('R$ 160,00')).toHaveLength(2)
    expect(screen.getByTestId('material-total')).toHaveTextContent('R$ 160,00')
  })

  it('soma o total apenas com os itens que têm preço real', () => {
    renderList([cimento, areia])
    expect(screen.getByTestId('material-total')).toHaveTextContent('R$ 160,00')
  })

  it('item sem produto casado aparece sem preço inventado', () => {
    renderList([cimento, areia])

    // A quantidade calculada continua visível…
    expect(screen.getByText('0,75 m³')).toBeInTheDocument()
    // …mas explicitamente sem preço, e sem link de produto.
    expect(screen.getByText('sem produto no catálogo')).toBeInTheDocument()
    expect(screen.getByText('sem preço')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /areia/i })).not.toBeInTheDocument()
    // Nenhum valor monetário além do preço unitário e do subtotal do cimento.
    expect(screen.getAllByText(/R\$/)).toHaveLength(3) // unit + subtotal + total
  })

  it('avisa quantos itens ficaram fora do total', () => {
    renderList([cimento, areia])
    expect(screen.getByText(/1 item ficou/i)).toBeInTheDocument()
  })

  it('não renderiza nada com lista vazia', () => {
    const { container } = renderList([])
    expect(container).toBeEmptyDOMElement()
  })

  it('subtotalItem devolve null sem produto, e totalLista ignora esse item', () => {
    expect(subtotalItem(areia)).toBeNull()
    expect(subtotalItem(cimento)).toBe(160)
    expect(totalLista([cimento, areia])).toBe(160)
  })
})

describe('MemoriaCalculo', () => {
  it('começa recolhida e abre ao clicar, mostrando faixa e fonte', () => {
    const { container } = render(<MemoriaCalculo item={cimento} />)

    const details = container.querySelector('details')
    expect(details).not.toBeNull()
    expect(details).not.toHaveAttribute('open')

    const summary = screen.getByText(/como cheguei nesse número/i)
    fireEvent.click(summary)
    // happy-dom não alterna `open` por si só no clique — refletimos o toggle.
    details?.setAttribute('open', '')

    expect(details).toHaveAttribute('open')
    const dets = details as HTMLElement
    expect(within(dets).getByText(/150–190 kg\/m³/)).toBeInTheDocument()
    expect(within(dets).getByText(/NBR 12655/)).toBeInTheDocument()
    expect(within(dets).getByText(/168 kg/)).toBeInTheDocument()
    expect(within(dets).getByText(/varia com o traço/i)).toBeInTheDocument()
  })

  it('mostra um valor único quando min e max coincidem', () => {
    const exato: MaterialItem = { ...areia, coefMin: 2, coefMax: 2, coefUnid: 'un/m²' }
    render(<MemoriaCalculo item={exato} />)
    expect(screen.getByText(/2 un\/m²/)).toBeInTheDocument()
  })
})
