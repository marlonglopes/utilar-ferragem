import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { AddAllToCart } from '@/components/assistant/AddAllToCart'
import { itensComProduto } from '@/components/assistant/helpers'
import {
  ComplementSuggestion,
  ComplementSuggestions,
} from '@/components/assistant/ComplementSuggestion'
import { useCartStore } from '@/store/cartStore'
import type { LaraComplemento, MaterialItem } from '@/lib/alice'

const cimento: MaterialItem = {
  materialId: 'cimento-cp2',
  nome: 'Cimento CP-II 50 kg',
  quantidade: 168,
  unidBase: 'kg',
  embalagens: 4,
  unidVenda: 'saco 50 kg',
  memoria: 'x',
  fonte: 'y',
  coefMin: 150,
  coefMax: 190,
  coefUnid: 'kg/m³',
  produto: { slug: 'cimento-cp2-50kg', nome: 'Cimento CP-II', preco: 40, estoque: 120 },
}

const argamassa: MaterialItem = {
  ...cimento,
  materialId: 'argamassa-ac3',
  nome: 'Argamassa AC-III',
  embalagens: 7,
  produto: { slug: 'argamassa-ac3-20kg', nome: 'Argamassa AC-III 20 kg', preco: 30, estoque: 50 },
}

const areia: MaterialItem = {
  ...cimento,
  materialId: 'areia-media',
  nome: 'Areia média',
  embalagens: 1,
  produto: undefined,
}

beforeEach(() => {
  useCartStore.setState({ items: [] })
})

describe('AddAllToCart', () => {
  it('adiciona a quantidade `embalagens` de cada item com produto e pula os sem produto', () => {
    render(<AddAllToCart itens={[cimento, argamassa, areia]} />)
    fireEvent.click(screen.getByRole('button', { name: /adicionar tudo ao carrinho/i }))

    const items = useCartStore.getState().items
    expect(items).toHaveLength(2)

    const c = items.find((i) => i.productId === 'cimento-cp2-50kg')
    expect(c?.quantity).toBe(4)
    expect(c?.priceSnapshot).toBe(40)
    expect(c?.stock).toBe(120)

    expect(items.find((i) => i.productId === 'argamassa-ac3-20kg')?.quantity).toBe(7)
    expect(items.some((i) => i.name.includes('Areia'))).toBe(false)
  })

  it('reporta o que foi adicionado e o que foi pulado', () => {
    render(<AddAllToCart itens={[cimento, areia]} />)
    fireEvent.click(screen.getByRole('button', { name: /adicionar tudo ao carrinho/i }))

    const status = screen.getByRole('status')
    expect(status).toHaveTextContent(/1 item adicionado/i)
    expect(status).toHaveTextContent(/Cimento CP-II 50 kg/)
    expect(status).toHaveTextContent(/Não adicionei Areia média/i)
    expect(status).toHaveTextContent(/sem produto correspondente/i)
  })

  it('chama onAdicionado com o resumo', () => {
    let resumo: { adicionados: string[]; pulados: string[] } | null = null
    render(<AddAllToCart itens={[cimento, areia]} onAdicionado={(r) => (resumo = r)} />)
    fireEvent.click(screen.getByRole('button', { name: /adicionar tudo ao carrinho/i }))

    expect(resumo).toEqual({
      adicionados: ['Cimento CP-II 50 kg'],
      pulados: ['Areia média'],
    })
  })

  it('sem nenhum item casado, não oferece o botão', () => {
    render(<AddAllToCart itens={[areia]} />)
    expect(screen.queryByRole('button', { name: /adicionar tudo/i })).not.toBeInTheDocument()
    expect(screen.getByText(/nenhum item desta lista tem produto casado/i)).toBeInTheDocument()
  })

  it('itensComProduto ignora item sem produto e item com 0 embalagens', () => {
    expect(itensComProduto([cimento, areia])).toEqual([cimento])
    expect(itensComProduto([{ ...cimento, embalagens: 0 }])).toEqual([])
  })
})

const comMotivo: LaraComplemento = {
  produto: {
    id: 'prod-desemp',
    slug: 'desempenadeira-aco',
    name: 'Desempenadeira de aço',
    price: 34.5,
    stock: 18,
    category: 'ferramentas',
  },
  motivo: 'Contrapiso exige desempenadeira para o acabamento.',
  origem: 'tecnica',
}

describe('ComplementSuggestion', () => {
  it('mostra produto, preço e o MOTIVO da sugestão', () => {
    render(<ComplementSuggestion complemento={comMotivo} />)
    expect(screen.getByText('Desempenadeira de aço')).toBeInTheDocument()
    expect(screen.getByText('R$ 34,50')).toBeInTheDocument()
    expect(screen.getByText(/exige desempenadeira/i)).toBeInTheDocument()
    expect(screen.getByText(/^Por que:/)).toBeInTheDocument()
  })

  it('rotula co-compra como tendência, não como exigência', () => {
    render(
      <ComplementSuggestion
        complemento={{ ...comMotivo, origem: 'co-compra', motivo: 'apareceu em 40 pedidos' }}
      />
    )
    expect(screen.getByText(/costuma sair junto/i)).toBeInTheDocument()
  })

  it('sugestão SEM motivo não renderiza', () => {
    const { container } = render(
      <ComplementSuggestion complemento={{ ...comMotivo, motivo: '' }} />
    )
    expect(container).toBeEmptyDOMElement()

    const branco = render(<ComplementSuggestion complemento={{ ...comMotivo, motivo: '   ' }} />)
    expect(branco.container).toBeEmptyDOMElement()
  })

  it('adiciona ao carrinho com 1 toque', () => {
    render(<ComplementSuggestion complemento={comMotivo} />)
    fireEvent.click(screen.getByRole('button', { name: /adicionar desempenadeira de aço/i }))

    const items = useCartStore.getState().items
    expect(items).toHaveLength(1)
    expect(items[0]).toMatchObject({ productId: 'prod-desemp', quantity: 1, priceSnapshot: 34.5 })
    expect(screen.getByRole('button', { name: /adicionar desempenadeira/i })).toBeDisabled()
  })

  it('a lista filtra os sem motivo e some inteira quando nenhum sobra', () => {
    const { unmount } = render(
      <ComplementSuggestions complementos={[comMotivo, { ...comMotivo, motivo: '' }]} />
    )
    expect(screen.getAllByRole('button', { name: /adicionar/i })).toHaveLength(1)
    unmount()

    const vazio = render(<ComplementSuggestions complementos={[{ ...comMotivo, motivo: '' }]} />)
    expect(vazio.container).toBeEmptyDOMElement()
  })
})
