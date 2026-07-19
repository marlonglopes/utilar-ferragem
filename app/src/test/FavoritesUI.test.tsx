import { render, screen, fireEvent, within } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import { HelmetProvider } from 'react-helmet-async'
import i18n from '@/i18n'
import { FavoriteButton } from '@/components/catalog/FavoriteButton'
import FavoritesPage from '@/pages/favorites/FavoritesPage'
import { useFavoritesStore } from '@/store/favoritesStore'
import type { Product } from '@/types/product'

function product(over: Partial<Product> = {}): Product {
  return {
    id: 'p1',
    name: 'Cimento CP II 50kg',
    slug: 'cimento-cp-ii-50kg',
    category: 'construcao',
    price: 42.9,
    currency: 'BRL',
    icon: '◫',
    seller: 'Casa & Obra',
    stock: 100,
    rating: 4,
    reviewCount: 10,
    ...over,
  }
}

beforeEach(async () => {
  await i18n.changeLanguage('pt-BR')
  localStorage.clear()
  useFavoritesStore.setState({ items: [], syncedAt: null })
})

function renderButton(p: Product) {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <FavoriteButton product={p} />
      </MemoryRouter>
    </I18nextProvider>
  )
}

function renderPage() {
  return render(
    <HelmetProvider>
      <I18nextProvider i18n={i18n}>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
          <FavoritesPage />
        </MemoryRouter>
      </I18nextProvider>
    </HelmetProvider>
  )
}

describe('FavoriteButton — acessibilidade', () => {
  /**
   * REGRESSÃO: coração de favorito sem estado acessível.
   *
   * Um ícone que só muda de COR não comunica nada para leitor de tela. E um
   * `aria-label` que troca de texto faz o leitor anunciar dois botões
   * diferentes em vez do mesmo botão ligado/desligado. `aria-pressed` é o que
   * transforma o coração num toggle de verdade.
   */
  it('expõe aria-pressed refletindo o estado', () => {
    renderButton(product())
    const btn = screen.getByRole('button')

    expect(btn).toHaveAttribute('aria-pressed', 'false')
    fireEvent.click(btn)
    expect(btn).toHaveAttribute('aria-pressed', 'true')
    fireEvent.click(btn)
    expect(btn).toHaveAttribute('aria-pressed', 'false')
  })

  it('tem rótulo textual que descreve a ação', () => {
    renderButton(product())
    expect(screen.getByRole('button', { name: /salvar nos favoritos/i })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button'))
    expect(screen.getByRole('button', { name: /remover dos favoritos/i })).toBeInTheDocument()
  })

  it('tem alvo de toque confortável para o polegar', () => {
    renderButton(product())
    // A maioria compra no celular: alvo pequeno demais faz o cliente favoritar
    // sem querer, ou pior, abrir o produto tentando favoritar.
    expect(screen.getByRole('button').className).toMatch(/min-h-\[40px\]/)
  })
})

describe('FavoriteButton — comportamento', () => {
  it('grava o produto no store', () => {
    renderButton(product({ id: 'cimento-1' }))
    fireEvent.click(screen.getByRole('button'))

    expect(useFavoritesStore.getState().isFavorite('cimento-1')).toBe(true)
  })

  /**
   * REGRESSÃO: favoritar navegava para o produto.
   *
   * O card inteiro da vitrine é um <Link>. Sem preventDefault no coração, o
   * clique borbulha, o roteador navega e o cliente perde a vitrine que estava
   * percorrendo — exatamente o oposto do que "salvar pra depois" promete.
   */
  it('não deixa o clique borbulhar para o link do card', () => {
    let navegou = false
    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
          <div onClick={() => { navegou = true }}>
            <FavoriteButton product={product()} />
          </div>
        </MemoryRouter>
      </I18nextProvider>
    )

    fireEvent.click(screen.getByRole('button'))

    expect(navegou).toBe(false)
    expect(useFavoritesStore.getState().items).toHaveLength(1)
  })

  it('funciona com produto sem estoque — é justamente quando se quer marcar', () => {
    renderButton(product({ id: 'esgotado', stock: 0 }))
    const btn = screen.getByRole('button')

    expect(btn).not.toBeDisabled()
    fireEvent.click(btn)
    expect(useFavoritesStore.getState().isFavorite('esgotado')).toBe(true)
  })
})

describe('FavoritesPage', () => {
  it('mostra estado vazio com caminho de saída', () => {
    renderPage()

    expect(screen.getByText(/sua lista de favoritos está vazia/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /explorar catálogo/i })).toHaveAttribute('href', '/')
  })

  it('lista os favoritos salvos com link para o produto', () => {
    useFavoritesStore.getState().add(product({ id: 'a', name: 'Cimento CP II', slug: 'cimento' }))
    useFavoritesStore.getState().add(product({ id: 'b', name: 'Cabo 2,5mm', slug: 'cabo' }))

    renderPage()

    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByRole('link', { name: 'Cabo 2,5mm' })).toHaveAttribute('href', '/produto/cabo')
    expect(screen.getByText('2 produtos salvos')).toBeInTheDocument()
  })

  it('avisa que o preço mostrado é o de quando foi salvo', () => {
    useFavoritesStore.getState().add(product())
    renderPage()

    // Sem esse aviso o cliente monta o orçamento da obra sobre um preço velho.
    expect(screen.getByText(/preço pode ter mudado/i)).toBeInTheDocument()
    expect(screen.getByText(/preço quando você salvou/i)).toBeInTheDocument()
  })

  it('avisa que a lista é local enquanto não há backend', () => {
    useFavoritesStore.getState().add(product())
    renderPage()
    expect(screen.getByText(/salva neste dispositivo/i)).toBeInTheDocument()
  })

  it('remove um item da lista', () => {
    useFavoritesStore.getState().add(product({ id: 'a', name: 'Cimento CP II' }))
    useFavoritesStore.getState().add(product({ id: 'b', name: 'Cabo 2,5mm' }))
    renderPage()

    const cimento = screen
      .getAllByRole('listitem')
      .find((li) => within(li).queryByText('Cimento CP II'))!
    fireEvent.click(within(cimento).getByRole('button', { name: /remover dos favoritos/i }))

    expect(useFavoritesStore.getState().items.map((i) => i.productId)).toEqual(['b'])
  })

  it('limpar a lista pede confirmação antes de apagar', () => {
    useFavoritesStore.getState().add(product())
    renderPage()

    fireEvent.click(screen.getByRole('button', { name: /limpar lista/i }))
    // Ainda não apagou: a lista é trabalho de dias do cliente.
    expect(useFavoritesStore.getState().items).toHaveLength(1)

    const dialogo = screen.getByText(/remover todos os produtos/i).closest('div')!
    fireEvent.click(within(dialogo).getByRole('button', { name: /limpar lista/i }))

    expect(useFavoritesStore.getState().items).toHaveLength(0)
  })

  it('cancelar a limpeza preserva a lista', () => {
    useFavoritesStore.getState().add(product())
    renderPage()

    fireEvent.click(screen.getByRole('button', { name: /limpar lista/i }))
    fireEvent.click(screen.getByRole('button', { name: /^cancelar$/i }))

    expect(useFavoritesStore.getState().items).toHaveLength(1)
    expect(screen.queryByText(/remover todos os produtos/i)).not.toBeInTheDocument()
  })
})
