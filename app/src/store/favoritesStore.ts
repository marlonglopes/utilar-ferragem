import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Product } from '@/types/product'

/**
 * Favorito guardado no dispositivo.
 *
 * PORQUÊ guardar um SNAPSHOT do produto e não só o `productId`: obra se decide
 * em dias. O cliente favorita 8 itens hoje e volta na quinta pra comparar. Se a
 * lista guardasse só ids, abrir /favoritos dispararia 8 requisições em série no
 * 4G da obra antes de mostrar qualquer coisa — e com o catálogo fora do ar a
 * página apareceria vazia, como se o cliente tivesse perdido a lista.
 *
 * Com snapshot, a lista renderiza instantaneamente e offline. O preço do
 * snapshot é explicitamente rotulado como "preço de quando você salvou" na UI,
 * porque preço de ferragem muda — não dá pra exibir como se fosse o de agora.
 */
export interface FavoriteItem {
  productId: string
  slug: string
  name: string
  icon: string
  /** Preço no momento em que foi favoritado. NÃO é o preço atual. */
  priceSnapshot: number
  imageUrl?: string
  seller: string
  addedAt: string
}

interface FavoritesState {
  items: FavoriteItem[]
  /**
   * Marca d'água da última sincronização com o servidor. `null` = a lista nunca
   * saiu deste dispositivo. Ver `mergeFromServer` para o contrato pendente.
   */
  syncedAt: string | null

  add: (product: Product) => void
  remove: (productId: string) => void
  /** Alterna e devolve o estado resultante — a UI usa pra escolher o toast. */
  toggle: (product: Product) => boolean
  isFavorite: (productId: string) => boolean
  clear: () => void
  /**
   * União com a lista do servidor, sem perder o que foi salvo offline.
   *
   * AINDA NÃO É CHAMADO — não existe endpoint de favoritos no auth-service
   * (ver `docs/favoritos-backend.md`). Está aqui porque a regra de merge é a
   * parte que dá errado quando o backend chega: o caminho ingênuo
   * (`set(serverItems)`) apaga o que o visitante montou antes de logar, que é
   * exatamente o momento em que ele mais tem itens na lista.
   *
   * União por `productId`, mantendo o `addedAt` MAIS ANTIGO: o cliente que
   * favoritou cimento no celular semana passada e no desktop hoje tem um
   * favorito só, datado de semana passada.
   */
  mergeFromServer: (serverItems: FavoriteItem[], syncedAt?: string) => void
}

function toFavorite(product: Product): FavoriteItem {
  return {
    productId: product.id,
    slug: product.slug,
    name: product.name,
    icon: product.icon,
    priceSnapshot: product.price,
    imageUrl: product.images?.[0]?.url ?? product.imageUrl,
    seller: product.seller,
    addedAt: new Date().toISOString(),
  }
}

export const useFavoritesStore = create<FavoritesState>()(
  persist(
    (set, get) => ({
      items: [],
      syncedAt: null,

      add: (product) =>
        set((state) => {
          if (state.items.some((i) => i.productId === product.id)) return state
          // Mais recente primeiro: a lista de favoritos é usada como
          // "coisas que ainda vou decidir", e o que acabou de entrar é o
          // que está na cabeça do cliente.
          return { items: [toFavorite(product), ...state.items] }
        }),

      remove: (productId) =>
        set((state) => ({ items: state.items.filter((i) => i.productId !== productId) })),

      toggle: (product) => {
        const has = get().items.some((i) => i.productId === product.id)
        if (has) {
          get().remove(product.id)
          return false
        }
        get().add(product)
        return true
      },

      isFavorite: (productId) => get().items.some((i) => i.productId === productId),

      clear: () => set({ items: [] }),

      mergeFromServer: (serverItems, syncedAt) =>
        set((state) => {
          const byId = new Map<string, FavoriteItem>()
          for (const item of [...state.items, ...serverItems]) {
            const existing = byId.get(item.productId)
            if (!existing) {
              byId.set(item.productId, item)
              continue
            }
            // Empate: fica o mais antigo, e o snapshot do servidor (mais
            // provável de estar atualizado) só entra se for o registro novo.
            byId.set(item.productId, item.addedAt < existing.addedAt ? item : existing)
          }
          const merged = [...byId.values()].sort((a, b) => (a.addedAt < b.addedAt ? 1 : -1))
          return { items: merged, syncedAt: syncedAt ?? new Date().toISOString() }
        }),
    }),
    { name: 'utilar-favorites' }
  )
)
