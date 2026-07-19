import { create } from 'zustand'
import { persist } from 'zustand/middleware'

/**
 * Store do PDV de balcão (venda presencial no tablet).
 *
 * Deliberadamente SEPARADO do `cartStore` do e-commerce: o vendedor opera o
 * balcão na mesma máquina o dia inteiro e não pode misturar itens com a sessão
 * de um cliente que ficou logada. Chave de persistência própria
 * (`utilar-balcao`), tipos próprios, nenhum import de `cartStore`.
 *
 * Este arquivo também concentra o MOTOR DE PRECIFICAÇÃO (funções puras,
 * exportadas e testadas em `src/test/balcaoPricing.test.ts`). A regra é: nenhum
 * cálculo de margem/desconto pode viver dentro de componente.
 */

// ---------------------------------------------------------------------------
// Tipos
// ---------------------------------------------------------------------------

/**
 * Cargo do operador dentro da loja — espelha `store_operator_levels.level` do
 * auth-service (`operator | supervisor | manager`).
 *
 * O cargo é rótulo; quem manda no quanto de desconto é
 * {@link BalcaoOperator.discountCeilingPct}, que vem RESOLVIDO do backend
 * (override individual, se houver, senão o teto do cargo). O front nunca deriva
 * teto a partir do cargo — foi exatamente esse hardcode que saiu daqui.
 */
export type BalcaoLevel = 'operator' | 'supervisor' | 'manager'

/**
 * Contexto de loja do operador logado — resposta de `GET /api/v1/store/me`
 * (auth-service, `model.StoreOperator`).
 */
export interface BalcaoOperator {
  userId: string
  name: string
  storeId: string
  storeCode: string
  storeName: string
  level: BalcaoLevel
  /** Teto de desconto (%) sem aprovação. Autoritativo, vem do banco. */
  discountCeilingPct: number
  /** O cargo homologa desconto de terceiros. */
  canApproveDiscount: boolean
  /**
   * `false` = este contexto NÃO veio do backend (modo mock, ou falha na
   * consulta). A UI precisa saber para não apresentar número de demonstração
   * como se fosse a regra da loja.
   */
  fromBackend: boolean
}

/**
 * Contexto de demonstração: usado só quando o auth-service não está configurado
 * (modo mock), para o PDV continuar demonstrável sem backend. Os números aqui
 * são plausíveis, não autoritativos — `fromBackend: false` marca isso.
 */
export const MOCK_OPERATOR: BalcaoOperator = {
  userId: 'mock-operator',
  name: 'Vendedor (demonstração)',
  storeId: 'mock-store',
  storeCode: 'CTR',
  storeName: 'Loja Centro',
  level: 'operator',
  discountCeilingPct: 12,
  canApproveDiscount: false,
  fromBackend: false,
}

/**
 * Contexto usado quando o backend ESTÁ configurado mas a consulta falhou (rede,
 * 403, vínculo revogado). Teto 0 = todo desconto cai na fila do gerente.
 *
 * É a mesma política fail-closed do order-service (`actorFromContext`): teto
 * otimista durante uma indisponibilidade transforma incidente de infra em rombo
 * de caixa.
 */
export const FAIL_CLOSED_OPERATOR: BalcaoOperator = {
  userId: '',
  name: '',
  storeId: '',
  storeCode: '',
  storeName: '',
  level: 'operator',
  discountCeilingPct: 0,
  canApproveDiscount: false,
  fromBackend: false,
}

/** Abaixo desta margem (%) a barra vira âmbar — venda "apertada". */
export const HEALTHY_MARGIN_PCT = 15

/**
 * Custo estimado como fração do preço, usado APENAS quando o custo real não
 * chega do catálogo — ver `deriveUnitCost()` em `src/hooks/useBalcaoProducts.ts`.
 * Toda linha estimada carrega `costIsEstimated: true` e a UI rotula a margem
 * como estimada.
 */
export const ASSUMED_COST_RATIO = 0.72

export interface BalcaoItem {
  productId: string
  sku: string
  barcode?: string
  name: string
  icon: string
  /** Unidade de venda: "un", "cx", "m", "kg"… */
  unit: string
  unitPrice: number
  unitCost: number
  /** `true` = `unitCost` é chute (preço × ratio), não custo real do catálogo. */
  costIsEstimated: boolean
  quantity: number
  stock: number
  addedAt: string
}

export type CustomerSegment = 'varejo' | 'atacado' | 'construtora'

export interface BalcaoCustomer {
  id?: string
  name: string
  /** CPF ou CNPJ, apenas dígitos. */
  document: string
  /** Obrigatório: a Appmax rejeita a cobrança sem celular do pagador. */
  phone: string
  segment: CustomerSegment
}

/** Uma "comanda" = um pedido de balcão em aberto. */
export interface Comanda {
  id: string
  label: string
  items: BalcaoItem[]
  discountPct: number
  customer: BalcaoCustomer | null
  createdAt: string
}

// ---------------------------------------------------------------------------
// Motor de precificação — funções PURAS (sem React, sem store)
// ---------------------------------------------------------------------------

export function round2(value: number): number {
  return Math.round((value + Number.EPSILON) * 100) / 100
}

function clamp(value: number, min: number, max: number): number {
  if (Number.isNaN(value)) return min
  return Math.min(max, Math.max(min, value))
}

export type MarginStatus = 'healthy' | 'warning' | 'negative'

export type PricedLine = Pick<BalcaoItem, 'unitPrice' | 'unitCost' | 'quantity'> &
  Partial<Pick<BalcaoItem, 'costIsEstimated'>>

export interface PricingInput {
  items: PricedLine[]
  discountPct: number
  /**
   * Teto de desconto (%) do operador — vem de `GET /api/v1/store/me`.
   * OBRIGATÓRIO de propósito: sem default, ninguém calcula margem contra um
   * teto inventado pelo front.
   */
  ceilingPct: number
}

export interface BalcaoPricing {
  /** Soma das quantidades (não o número de linhas). */
  itemCount: number
  lineCount: number
  /** Bruto, antes do desconto. */
  subtotal: number
  /** Custo total das mercadorias. */
  cost: number
  discountPct: number
  discountAmount: number
  /** Líquido a cobrar. */
  total: number
  /** total - cost. Negativo = venda no prejuízo. */
  grossProfit: number
  /** Margem sobre a venda (%), já com o desconto aplicado. */
  marginPct: number
  /** Margem antes de qualquer desconto (%), para referência na barra. */
  baseMarginPct: number
  /**
   * Alguma linha usa custo ESTIMADO — logo `cost`, `grossProfit` e `marginPct`
   * são aproximações. A UI é obrigada a rotular isso: margem apresentada como
   * fato quando é chute é pior que margem assumidamente estimada.
   */
  costEstimated: boolean
  status: MarginStatus
  /** Desconto levou o total abaixo do custo — alerta bloqueante. */
  belowCost: boolean
  /** Teto do cargo (%). */
  ceilingPct: number
  overCeiling: boolean
  /** Sai como pedido pendente de aprovação do gerente (não bloqueia). */
  requiresApproval: boolean
  /** Impede a cobrança. */
  blocked: boolean
}

/**
 * Calcula totais, desconto e saúde da margem de um pedido de balcão.
 *
 * Regras:
 * - `marginPct` é margem SOBRE A VENDA: (total - custo) / total.
 * - abaixo de {@link HEALTHY_MARGIN_PCT} → 'warning'; lucro negativo → 'negative'.
 * - desconto acima do teto do operador NÃO bloqueia: marca `requiresApproval`.
 *   (o servidor decide de fato — aqui é só a previsão que o vendedor vê).
 * - desconto que leva o total abaixo do custo bloqueia (`blocked`).
 */
export function computeBalcaoPricing(input: PricingInput): BalcaoPricing {
  const ceilingPct = Math.max(0, input.ceilingPct)
  const discountPct = round2(clamp(input.discountPct, 0, 100))

  let subtotal = 0
  let cost = 0
  let itemCount = 0
  let costEstimated = false
  for (const line of input.items) {
    const qty = Math.max(0, line.quantity)
    subtotal += line.unitPrice * qty
    cost += line.unitCost * qty
    itemCount += qty
    // Uma única linha estimada já contamina o total: a margem do pedido inteiro
    // passa a ser aproximada.
    if (line.costIsEstimated) costEstimated = true
  }
  subtotal = round2(subtotal)
  cost = round2(cost)

  const discountAmount = round2((subtotal * discountPct) / 100)
  const total = round2(subtotal - discountAmount)
  const grossProfit = round2(total - cost)

  const marginPct = total > 0 ? round2((grossProfit / total) * 100) : 0
  const baseMarginPct = subtotal > 0 ? round2(((subtotal - cost) / subtotal) * 100) : 0

  const empty = input.items.length === 0 || subtotal === 0
  const belowCost = !empty && total < cost

  let status: MarginStatus = 'healthy'
  if (!empty) {
    if (grossProfit < 0) status = 'negative'
    else if (marginPct < HEALTHY_MARGIN_PCT) status = 'warning'
  }

  const overCeiling = !empty && discountPct > ceilingPct

  return {
    itemCount,
    lineCount: input.items.length,
    subtotal,
    cost,
    discountPct,
    discountAmount,
    total,
    grossProfit,
    marginPct,
    baseMarginPct,
    costEstimated,
    status,
    belowCost,
    ceilingPct,
    overCeiling,
    requiresApproval: overCeiling,
    blocked: belowCost,
  }
}

/**
 * Maior desconto (%) que ainda mantém o total >= custo. Usado para mostrar ao
 * vendedor onde fica o "ponto de prejuízo" na régua do slider.
 */
export function maxDiscountPctBeforeCost(items: PricedLine[]): number {
  let subtotal = 0
  let cost = 0
  for (const line of items) {
    const qty = Math.max(0, line.quantity)
    subtotal += line.unitPrice * qty
    cost += line.unitCost * qty
  }
  if (subtotal <= 0) return 0
  if (cost >= subtotal) return 0
  return round2(((subtotal - cost) / subtotal) * 100)
}

/** Documento é CNPJ (14 dígitos) em vez de CPF (11)? */
export function isCNPJ(document: string): boolean {
  return document.replace(/\D/g, '').length > 11
}

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

let comandaSeq = 0
function nextComandaId(): string {
  comandaSeq += 1
  return `cmd-${Date.now().toString(36)}-${comandaSeq}`
}

export function createComanda(label?: string): Comanda {
  const id = nextComandaId()
  return {
    id,
    label: label ?? 'Comanda',
    items: [],
    discountPct: 0,
    customer: null,
    createdAt: new Date().toISOString(),
  }
}

export type NewBalcaoItem = Omit<BalcaoItem, 'addedAt'>

interface BalcaoState {
  comandas: Comanda[]
  activeId: string
  /**
   * Contexto de loja do operador logado. Preenchido por `useBalcaoOperator`
   * a partir de `GET /api/v1/store/me`; NUNCA persistido (ver `partialize`).
   */
  operator: BalcaoOperator

  // comandas
  openComanda: () => string
  closeComanda: (id: string) => void
  setActiveComanda: (id: string) => void

  // itens da comanda ativa
  addItem: (item: NewBalcaoItem) => void
  removeItem: (productId: string) => void
  setQuantity: (productId: string, quantity: number) => void
  incrementItem: (productId: string) => void
  decrementItem: (productId: string) => void

  // negociação / cliente
  setDiscountPct: (pct: number) => void
  setCustomer: (customer: BalcaoCustomer | null) => void
  /** `null` = falha ao resolver o contexto → fail-closed (teto 0). */
  setOperator: (operator: BalcaoOperator | null) => void

  clearComanda: () => void
}

const INITIAL = createComanda('Comanda 1')

function relabel(comandas: Comanda[]): Comanda[] {
  return comandas.map((c, i) => ({ ...c, label: `Comanda ${i + 1}` }))
}

export const useBalcaoStore = create<BalcaoState>()(
  persist(
    (set, get) => {
      /** Aplica `fn` somente na comanda ativa. */
      const patchActive = (fn: (c: Comanda) => Comanda) =>
        set((state) => ({
          comandas: state.comandas.map((c) => (c.id === state.activeId ? fn(c) : c)),
        }))

      return {
        comandas: [INITIAL],
        activeId: INITIAL.id,
        // Nasce fail-closed: até `useBalcaoOperator` responder, teto 0.
        operator: FAIL_CLOSED_OPERATOR,

        openComanda: () => {
          const created = createComanda()
          set((state) => ({
            comandas: relabel([...state.comandas, created]),
            activeId: created.id,
          }))
          return created.id
        },

        closeComanda: (id) =>
          set((state) => {
            const remaining = state.comandas.filter((c) => c.id !== id)
            // Nunca fica sem comanda: fechar a última abre uma vazia no lugar.
            if (remaining.length === 0) {
              const fresh = createComanda('Comanda 1')
              return { comandas: [fresh], activeId: fresh.id }
            }
            const relabeled = relabel(remaining)
            return {
              comandas: relabeled,
              activeId: state.activeId === id ? relabeled[0].id : state.activeId,
            }
          }),

        setActiveComanda: (id) =>
          set((state) => (state.comandas.some((c) => c.id === id) ? { activeId: id } : state)),

        addItem: (item) =>
          patchActive((c) => {
            const existing = c.items.find((i) => i.productId === item.productId)
            if (existing) {
              return {
                ...c,
                items: c.items.map((i) =>
                  i.productId === item.productId
                    ? { ...i, quantity: Math.min(i.stock, i.quantity + item.quantity) }
                    : i
                ),
              }
            }
            return {
              ...c,
              items: [
                ...c.items,
                {
                  ...item,
                  quantity: Math.min(item.stock, Math.max(1, item.quantity)),
                  addedAt: new Date().toISOString(),
                },
              ],
            }
          }),

        removeItem: (productId) =>
          patchActive((c) => ({ ...c, items: c.items.filter((i) => i.productId !== productId) })),

        setQuantity: (productId, quantity) =>
          patchActive((c) => {
            // Quantidade <= 0 remove a linha (comportamento esperado no ± do PDV).
            if (quantity <= 0) {
              return { ...c, items: c.items.filter((i) => i.productId !== productId) }
            }
            return {
              ...c,
              items: c.items.map((i) =>
                i.productId === productId
                  ? { ...i, quantity: Math.min(i.stock, quantity) }
                  : i
              ),
            }
          }),

        incrementItem: (productId) => {
          const item = activeComandaOf(get()).items.find((i) => i.productId === productId)
          if (item) get().setQuantity(productId, item.quantity + 1)
        },

        decrementItem: (productId) => {
          const item = activeComandaOf(get()).items.find((i) => i.productId === productId)
          if (item) get().setQuantity(productId, item.quantity - 1)
        },

        setDiscountPct: (pct) =>
          patchActive((c) => ({ ...c, discountPct: round2(clamp(pct, 0, 100)) })),

        setCustomer: (customer) => patchActive((c) => ({ ...c, customer })),

        setOperator: (operator) => set({ operator: operator ?? FAIL_CLOSED_OPERATOR }),

        clearComanda: () =>
          patchActive((c) => ({ ...c, items: [], discountPct: 0, customer: null })),
      }
    },
    {
      name: 'utilar-balcao',
      version: 2,
      /**
       * O CONTEXTO DO OPERADOR NÃO É PERSISTIDO — de propósito.
       *
       * Teto de desconto em localStorage é o hardcode de volta, só que pior:
       * sobreviveria a um rebaixamento de cargo e viajaria entre turnos no mesmo
       * tablet. Ele é rebuscado a cada carga do PDV; enquanto não chega, vale o
       * FAIL_CLOSED_OPERATOR.
       */
      partialize: (state) => ({ comandas: state.comandas, activeId: state.activeId }),
      // Reidratação defensiva: um localStorage antigo/corrompido não pode deixar
      // o PDV sem comanda ativa (tela branca no tablet).
      merge: (persisted, current) => {
        const merged = { ...current, ...(persisted as Partial<BalcaoState>) }
        if (!merged.comandas || merged.comandas.length === 0) {
          const fresh = createComanda('Comanda 1')
          merged.comandas = [fresh]
          merged.activeId = fresh.id
        } else if (!merged.comandas.some((c) => c.id === merged.activeId)) {
          merged.activeId = merged.comandas[0].id
        }
        return merged
      },
    }
  )
)

// ---------------------------------------------------------------------------
// Selectors
// ---------------------------------------------------------------------------

function activeComandaOf(state: Pick<BalcaoState, 'comandas' | 'activeId'>): Comanda {
  return state.comandas.find((c) => c.id === state.activeId) ?? state.comandas[0]
}

/** Comanda ativa (nunca undefined). */
export function selectActiveComanda(
  state: Pick<BalcaoState, 'comandas' | 'activeId'>
): Comanda {
  return activeComandaOf(state)
}

/**
 * Precificação da comanda ativa.
 *
 * NÃO use isto como selector de `useBalcaoStore` — devolve objeto novo a cada
 * chamada e quebraria o `getSnapshot` do useSyncExternalStore (loop de render).
 * Em componente, chame `computeBalcaoPricing` dentro de um `useMemo`.
 */
export function balcaoPricingOf(
  state: Pick<BalcaoState, 'comandas' | 'activeId' | 'operator'>
): BalcaoPricing {
  const comanda = activeComandaOf(state)
  return computeBalcaoPricing({
    items: comanda.items,
    discountPct: comanda.discountPct,
    ceilingPct: state.operator.discountCeilingPct,
  })
}
