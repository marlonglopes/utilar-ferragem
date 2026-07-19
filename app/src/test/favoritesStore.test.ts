import { describe, it, expect, beforeEach } from 'vitest'
import { useFavoritesStore, type FavoriteItem } from '@/store/favoritesStore'
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

const STORAGE_KEY = 'utilar-favorites'

beforeEach(() => {
  localStorage.clear()
  useFavoritesStore.setState({ items: [], syncedAt: null })
})

describe('favoritesStore — básico', () => {
  it('adiciona e reconhece o favorito', () => {
    useFavoritesStore.getState().add(product())
    expect(useFavoritesStore.getState().isFavorite('p1')).toBe(true)
    expect(useFavoritesStore.getState().items).toHaveLength(1)
  })

  it('não duplica o mesmo produto', () => {
    const p = product()
    useFavoritesStore.getState().add(p)
    useFavoritesStore.getState().add(p)
    expect(useFavoritesStore.getState().items).toHaveLength(1)
  })

  it('toggle devolve o estado resultante para a UI escolher o aviso', () => {
    const p = product()
    expect(useFavoritesStore.getState().toggle(p)).toBe(true)
    expect(useFavoritesStore.getState().isFavorite('p1')).toBe(true)
    expect(useFavoritesStore.getState().toggle(p)).toBe(false)
    expect(useFavoritesStore.getState().isFavorite('p1')).toBe(false)
  })

  it('guarda um snapshot do produto, não só o id', () => {
    useFavoritesStore.getState().add(
      product({ images: [{ url: 'https://x/cimento.jpg', alt: 'Cimento' }] })
    )
    const fav = useFavoritesStore.getState().items[0]

    // A página /favoritos precisa renderizar sem consultar o catálogo item a
    // item — no 4G da obra isso seria N requisições antes de mostrar qualquer
    // coisa, e catálogo fora do ar mostraria a lista vazia.
    expect(fav).toMatchObject({
      productId: 'p1',
      slug: 'cimento-cp-ii-50kg',
      name: 'Cimento CP II 50kg',
      priceSnapshot: 42.9,
      seller: 'Casa & Obra',
      imageUrl: 'https://x/cimento.jpg',
    })
    expect(fav.addedAt).toBeTruthy()
  })

  it('mais recente aparece primeiro', () => {
    useFavoritesStore.getState().add(product({ id: 'velho' }))
    useFavoritesStore.getState().add(product({ id: 'novo' }))
    expect(useFavoritesStore.getState().items.map((i) => i.productId)).toEqual(['novo', 'velho'])
  })

  it('remove e limpa', () => {
    useFavoritesStore.getState().add(product({ id: 'a' }))
    useFavoritesStore.getState().add(product({ id: 'b' }))
    useFavoritesStore.getState().remove('a')
    expect(useFavoritesStore.getState().items.map((i) => i.productId)).toEqual(['b'])
    useFavoritesStore.getState().clear()
    expect(useFavoritesStore.getState().items).toHaveLength(0)
  })

  it('remover produto que não está na lista não quebra nada', () => {
    useFavoritesStore.getState().add(product({ id: 'a' }))
    useFavoritesStore.getState().remove('inexistente')
    expect(useFavoritesStore.getState().items).toHaveLength(1)
  })
})

describe('favoritesStore — persistência', () => {
  /**
   * REGRESSÃO: lista de favoritos perdida ao recarregar a página.
   *
   * O ponto inteiro da feature é que obra se decide em DIAS: o cliente monta a
   * lista, fecha o navegador e volta na quinta. Uma lista que só vive em
   * memória não serve pra nada. Sem backend, o localStorage é o contrato.
   */
  it('sobrevive ao recarregamento da página', () => {
    useFavoritesStore.getState().add(product({ id: 'cimento' }))
    useFavoritesStore.getState().add(product({ id: 'cabo', name: 'Cabo 2,5mm' }))

    const raw = localStorage.getItem(STORAGE_KEY)
    expect(raw).toBeTruthy()

    // Simula o boot: zustand/persist reidrata do localStorage.
    const persisted = JSON.parse(raw!) as { state: { items: FavoriteItem[] } }
    expect(persisted.state.items).toHaveLength(2)
    expect(persisted.state.items.map((i) => i.productId).sort()).toEqual(['cabo', 'cimento'])
    // O snapshot precisa estar completo no disco, senão a página volta sem preço.
    expect(persisted.state.items[0].name).toBeTruthy()
    expect(persisted.state.items[0].priceSnapshot).toBeGreaterThan(0)
  })

  it('remoção também persiste', () => {
    useFavoritesStore.getState().add(product({ id: 'a' }))
    useFavoritesStore.getState().add(product({ id: 'b' }))
    useFavoritesStore.getState().remove('a')

    const persisted = JSON.parse(localStorage.getItem(STORAGE_KEY)!) as {
      state: { items: FavoriteItem[] }
    }
    expect(persisted.state.items.map((i) => i.productId)).toEqual(['b'])
  })

  it('usa uma chave própria, sem colidir com carrinho ou sessão', () => {
    useFavoritesStore.getState().add(product())
    expect(localStorage.getItem(STORAGE_KEY)).toBeTruthy()
    expect(localStorage.getItem('utilar-cart')).toBeNull()
    expect(localStorage.getItem('utilar-auth')).toBeNull()
  })
})

describe('favoritesStore — merge com o servidor (caminho pronto pro backend)', () => {
  /**
   * REGRESSÃO: perder a lista do visitante ao fazer login.
   *
   * O caminho ingênuo quando o endpoint chegar é `set(itensDoServidor)`. Isso
   * apaga tudo que o visitante montou ANTES de criar a conta — que é
   * exatamente o momento em que ele mais tem itens salvos, já que veio
   * pesquisando material.
   */
  it('une as duas listas em vez de sobrescrever a local', () => {
    useFavoritesStore.getState().add(product({ id: 'local-so-daqui' }))

    useFavoritesStore.getState().mergeFromServer([
      {
        productId: 'do-servidor',
        slug: 'cabo',
        name: 'Cabo 2,5mm',
        icon: '⚡',
        priceSnapshot: 249,
        seller: 'Elétrica Costa',
        addedAt: '2026-01-01T00:00:00.000Z',
      },
    ])

    const ids = useFavoritesStore.getState().items.map((i) => i.productId).sort()
    expect(ids).toEqual(['do-servidor', 'local-so-daqui'])
  })

  it('produto nos dois lados vira um só, com a data mais antiga', () => {
    useFavoritesStore.setState({
      items: [
        {
          productId: 'cimento',
          slug: 'cimento',
          name: 'Cimento (desktop, hoje)',
          icon: '◫',
          priceSnapshot: 42.9,
          seller: 'Casa & Obra',
          addedAt: '2026-07-01T00:00:00.000Z',
        },
      ],
      syncedAt: null,
    })

    useFavoritesStore.getState().mergeFromServer([
      {
        productId: 'cimento',
        slug: 'cimento',
        name: 'Cimento (celular, semana passada)',
        icon: '◫',
        priceSnapshot: 40,
        seller: 'Casa & Obra',
        addedAt: '2026-06-24T00:00:00.000Z',
      },
    ])

    const items = useFavoritesStore.getState().items
    expect(items).toHaveLength(1)
    expect(items[0].addedAt).toBe('2026-06-24T00:00:00.000Z')
  })

  it('registra o carimbo de sincronização', () => {
    expect(useFavoritesStore.getState().syncedAt).toBeNull()
    useFavoritesStore.getState().mergeFromServer([], '2026-07-19T10:00:00.000Z')
    expect(useFavoritesStore.getState().syncedAt).toBe('2026-07-19T10:00:00.000Z')
  })

  it('merge mantém a ordem mais-recente-primeiro', () => {
    useFavoritesStore.getState().mergeFromServer([
      {
        productId: 'antigo',
        slug: 'a',
        name: 'A',
        icon: '◫',
        priceSnapshot: 1,
        seller: 's',
        addedAt: '2026-01-01T00:00:00.000Z',
      },
      {
        productId: 'recente',
        slug: 'b',
        name: 'B',
        icon: '◫',
        priceSnapshot: 1,
        seller: 's',
        addedAt: '2026-06-01T00:00:00.000Z',
      },
    ])

    expect(useFavoritesStore.getState().items.map((i) => i.productId)).toEqual([
      'recente',
      'antigo',
    ])
  })
})
