import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ShoppingCart, ChevronUp } from 'lucide-react'
import { Drawer } from '@/components/ui'
import { cn } from '@/lib/cn'
import { formatCurrency } from '@/lib/format'
import { BalcaoTopBar } from '@/components/balcao/BalcaoTopBar'
import { ComandaTabs } from '@/components/balcao/ComandaTabs'
import { OrderPanel } from '@/components/balcao/OrderPanel'
import { ProductSearchPanel } from '@/components/balcao/ProductSearchPanel'
import { ChargeModal } from '@/components/balcao/ChargeModal'
import { toBalcaoItem } from '@/hooks/useBalcaoProducts'
import { useBalcaoCheckout, type BalcaoPaymentMethod } from '@/hooks/useBalcaoCheckout'
import { useBalcaoOperator } from '@/hooks/useBalcaoOperator'
import { useBalcaoStore, selectActiveComanda, computeBalcaoPricing } from '@/store/balcaoStore'
import type { Product } from '@/types/product'

/**
 * PDV do balcão — tela principal (tablet landscape, duas colunas).
 *
 * Esquerda: busca + grade de produtos. Direita: painel do pedido fixo.
 * Em tela estreita (< lg) o painel vira gaveta inferior com uma barra de
 * resumo sempre visível — ele nunca some, senão o vendedor perde o total de
 * vista no meio da venda.
 */
export default function BalcaoPage() {
  const navigate = useNavigate()
  const searchRef = useRef<HTMLInputElement>(null)
  const [query, setQuery] = useState('')
  const [chargeOpen, setChargeOpen] = useState(false)
  const [panelOpen, setPanelOpen] = useState(false)

  const comandas = useBalcaoStore((s) => s.comandas)
  const activeId = useBalcaoStore((s) => s.activeId)
  const comanda = useBalcaoStore(selectActiveComanda)
  // O teto de desconto vem de GET /api/v1/store/me — nunca do front. Enquanto a
  // resposta não chega, o teto é 0 (fail-closed) e todo desconto aparece como
  // pendente, que é o erro seguro.
  const { operator } = useBalcaoOperator()
  // useMemo, não selector: um selector que devolve objeto novo a cada chamada
  // quebra o `getSnapshot` do useSyncExternalStore (loop de render).
  const pricing = useMemo(
    () =>
      computeBalcaoPricing({
        items: comanda.items,
        discountPct: comanda.discountPct,
        ceilingPct: operator.discountCeilingPct,
      }),
    [comanda.items, comanda.discountPct, operator.discountCeilingPct]
  )

  const addItem = useBalcaoStore((s) => s.addItem)
  const removeItem = useBalcaoStore((s) => s.removeItem)
  const incrementItem = useBalcaoStore((s) => s.incrementItem)
  const decrementItem = useBalcaoStore((s) => s.decrementItem)
  const setDiscountPct = useBalcaoStore((s) => s.setDiscountPct)
  const setCustomer = useBalcaoStore((s) => s.setCustomer)
  const clearComanda = useBalcaoStore((s) => s.clearComanda)
  const openComanda = useBalcaoStore((s) => s.openComanda)
  const closeComanda = useBalcaoStore((s) => s.closeComanda)
  const setActiveComanda = useBalcaoStore((s) => s.setActiveComanda)

  const checkout = useBalcaoCheckout()

  const handleAdd = useCallback(
    (product: Product) => {
      addItem(toBalcaoItem(product))
    },
    [addItem]
  )

  const canCharge = comanda.items.length > 0 && !pricing.blocked && comanda.customer !== null

  // MOBILE: abre a gaveta do pedido sozinha quando o 1º item entra numa comanda
  // vazia. No celular a comanda mora na gaveta inferior, e sem isso o vendedor
  // adicionava um produto e "não via nada acontecer" — o pedido nascia escondido.
  // Só dispara na transição 0→1 (o ref evita reabrir a cada item), então não
  // atrapalha quem adiciona vários itens seguidos. Restrito a tela estreita
  // (< lg): no desktop o painel já fica fixo ao lado, abrir a gaveta seria ruído.
  const prevItemCount = useRef(comanda.items.length)
  useEffect(() => {
    const count = comanda.items.length
    const isNarrow =
      typeof window !== 'undefined' &&
      typeof window.matchMedia === 'function' &&
      window.matchMedia('(max-width: 1023px)').matches
    if (isNarrow && prevItemCount.current === 0 && count > 0) setPanelOpen(true)
    prevItemCount.current = count
  }, [comanda.items.length])

  const openCharge = useCallback(() => {
    if (!canCharge) return
    checkout.reset()
    setChargeOpen(true)
  }, [canCharge, checkout])

  // Atalhos de teclado — o balcão tem teclado físico acoplado ao tablet.
  // F2 busca · F4 desconto · F8 cobrar.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'F2') {
        e.preventDefault()
        searchRef.current?.focus()
        searchRef.current?.select()
      } else if (e.key === 'F4') {
        e.preventDefault()
        setPanelOpen(true)
        const slider = document.getElementById('balcao-desconto')
        if (slider instanceof HTMLInputElement) slider.focus()
      } else if (e.key === 'F8') {
        e.preventDefault()
        openCharge()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [openCharge])

  const handleConfirm = useCallback(
    async (method: BalcaoPaymentMethod, nsu?: string) => {
      if (!comanda.customer) return
      await checkout.charge({
        items: comanda.items,
        pricing,
        customer: comanda.customer,
        method,
        nsu,
      })
    },
    [checkout, comanda.customer, comanda.items, pricing]
  )

  const handleDone = useCallback(() => {
    const outcome = checkout.outcome
    setChargeOpen(false)
    setPanelOpen(false)
    clearComanda()
    setQuery('')
    navigate('/balcao/venda-concluida', {
      state: outcome
        ? {
            orderId: outcome.orderId,
            orderNumber: outcome.orderNumber,
            method: outcome.method,
            total: pricing.total,
            // Direto do servidor: ele reavaliou o desconto contra o teto atual.
            requiresApproval: outcome.requiresApproval,
            approvalStatus: outcome.approvalStatus,
            discountPct: outcome.discountPct,
            discountAmount: outcome.discountAmount,
            customerName: comanda.customer?.name,
            nsu: outcome.external?.nsu,
          }
        : undefined,
    })
  }, [checkout.outcome, clearComanda, navigate, pricing.total, comanda.customer])

  // Mesmas props nos dois lugares; muda só o layout: painel lateral (tablet) tem
  // altura fixa com rolagem interna; a gaveta (celular) usa `flow` — cresce com o
  // conteúdo e quem rola é a própria gaveta, senão a lista de itens colapsava.
  const orderPanelProps = {
    comanda,
    pricing,
    onIncrement: incrementItem,
    onDecrement: decrementItem,
    onRemove: removeItem,
    onDiscountChange: setDiscountPct,
    onCustomerChange: setCustomer,
    onCharge: openCharge,
    ceilingFromBackend: operator.fromBackend,
  }

  // h-[100svh] (não h-screen/100vh): no celular o 100vh inclui a área atrás das
  // barras do navegador, e a barra inferior "Ver pedido/Cobrar" escorregava pra
  // fora da tela (sumia em landscape). svh = MENOR viewport visível (barras à
  // mostra), então a barra fica garantida em TODA orientação. Classe única de
  // propósito: empilhar h-screen + h-[100svh] deixaria o vencedor à mercê da
  // ordem do CSS gerado pelo Tailwind, não do className.
  return (
    <div className="flex h-[100svh] flex-col bg-gray-50">
      <BalcaoTopBar />

      <ComandaTabs
        comandas={comandas}
        activeId={activeId}
        onSelect={setActiveComanda}
        onOpen={openComanda}
        onClose={closeComanda}
      />

      <div className="flex min-h-0 flex-1">
        <ProductSearchPanel
          ref={searchRef}
          query={query}
          onQueryChange={setQuery}
          onAdd={handleAdd}
        />

        {/* Painel fixo — tablet landscape / desktop */}
        <aside className="hidden w-[380px] shrink-0 border-l border-gray-200 lg:block xl:w-[420px]">
          <OrderPanel {...orderPanelProps} />
        </aside>
      </div>

      {/* Tela estreita: barra de resumo (abre a gaveta) — sempre visível.
          Alça + seta ↑ deixam claro que é um "bottom sheet" tocável; quando há
          itens o texto vira ação ("Ver pedido e cobrar") e, quando dá pra cobrar,
          ganha um anel laranja pra puxar o olho pro próximo passo. */}
      <button
        type="button"
        onClick={() => setPanelOpen(true)}
        aria-label="Abrir o pedido do balcão"
        className={cn(
          'relative flex h-[70px] shrink-0 flex-col justify-center border-t border-gray-200 bg-brand-blue px-4 text-white shadow-[0_-4px_16px_rgba(0,0,0,0.18)] lg:hidden',
          canCharge && 'ring-2 ring-inset ring-brand-orange'
        )}
      >
        {/* alça — afordância universal de gaveta inferior */}
        <span
          className="absolute left-1/2 top-1.5 h-1 w-10 -translate-x-1/2 rounded-full bg-white/40"
          aria-hidden="true"
        />
        <div className="mt-1 flex items-center justify-between gap-3">
          <span className="flex items-center gap-2 font-semibold">
            <ShoppingCart className="h-5 w-5" aria-hidden="true" />
            {pricing.itemCount > 0 ? 'Ver pedido e cobrar' : 'Pedido'}
            {pricing.itemCount > 0 && (
              <span className="rounded-full bg-white/20 px-2 py-0.5 text-xs font-bold">
                {pricing.itemCount}
              </span>
            )}
          </span>
          <span className="flex items-center gap-2">
            <span className="font-display text-lg font-bold">{formatCurrency(pricing.total)}</span>
            <ChevronUp className="h-5 w-5" aria-hidden="true" />
          </span>
        </div>
      </button>

      <Drawer
        open={panelOpen}
        onClose={() => setPanelOpen(false)}
        side="bottom"
        title="Pedido do balcão"
        className="lg:hidden"
      >
        {/* flow: o painel cresce com o conteúdo e a PRÓPRIA gaveta rola (a Drawer
            já é overflow-y-auto). Sem isso, a lista de itens colapsava pra altura
            0 no celular e os itens não apareciam. */}
        <OrderPanel {...orderPanelProps} flow />
      </Drawer>

      <ChargeModal
        open={chargeOpen}
        onClose={() => setChargeOpen(false)}
        pricing={pricing}
        submitting={checkout.submitting}
        error={checkout.error}
        paymentResult={checkout.paymentResult}
        onConfirm={handleConfirm}
        onDone={handleDone}
        onCardConfirmed={checkout.markConfirmed}
        onSimulateConfirm={checkout.simulateConfirm}
      />
    </div>
  )
}
