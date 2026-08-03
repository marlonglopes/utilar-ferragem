import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { CartIssues } from '@/components/cart/CartIssues'
import { useCartStore, type CartItem } from '@/store/cartStore'
import { checkCartAvailability } from '@/lib/api'

function seed(items: Partial<CartItem>[]) {
  useCartStore.setState({
    items: items.map((i) => ({
      productId: i.productId ?? 'p',
      slug: 'x',
      name: i.name ?? 'Produto',
      icon: '',
      sellerId: 's',
      sellerName: 'Loja',
      priceSnapshot: 10,
      quantity: i.quantity ?? 1,
      stock: i.stock ?? 10,
      addedAt: '2026-01-01T00:00:00Z',
    })) as CartItem[],
  })
}

beforeEach(() => useCartStore.setState({ items: [] }))

describe('CartIssues — aviso gracioso de item indisponível', () => {
  it('não renderiza nada sem problemas', () => {
    const { container } = render(<CartIssues issues={[]} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('produto indisponível: mostra a mensagem e remove do carrinho', () => {
    seed([{ productId: 'p1', name: 'Furadeira' }])
    render(<CartIssues issues={[{ productId: 'p1', reason: 'indisponivel', available: 0 }]} />)
    expect(screen.getByText(/não está mais disponível/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /remover/i }))
    expect(useCartStore.getState().items).toHaveLength(0)
  })

  it('sem estoque: mostra o saldo e ajusta a quantidade', () => {
    seed([{ productId: 'p2', name: 'Cimento', quantity: 10 }])
    render(<CartIssues issues={[{ productId: 'p2', reason: 'sem_estoque', available: 3 }]} />)
    expect(screen.getByText(/só 3 em estoque/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /ajustar para 3/i }))
    expect(useCartStore.getState().items[0].quantity).toBe(3)
  })
})

describe('checkCartAvailability — em mock (sem catálogo) não bloqueia', () => {
  it('retorna [] quando o catálogo não está configurado', async () => {
    // Em teste VITE_CATALOG_URL é vazio → não deve inventar bloqueio.
    const issues = await checkCartAvailability([{ productId: 'p1', quantity: 5 }])
    expect(issues).toEqual([])
  })
})
