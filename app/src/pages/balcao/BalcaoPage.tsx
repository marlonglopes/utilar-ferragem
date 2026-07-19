import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ShoppingCart } from 'lucide-react'
import { Drawer } from '@/components/ui'
import { formatCurrency } from '@/lib/format'
import { BalcaoTopBar } from '@/components/balcao/BalcaoTopBar'
import { ComandaTabs } from '@/components/balcao/ComandaTabs'
import { OrderPanel } from '@/components/balcao/OrderPanel'
import { ProductSearchPanel } from '@/components/balcao/ProductSearchPanel'
import { ChargeModal } from '@/components/balcao/ChargeModal'
import { toBalcaoItem } from '@/hooks/useBalcaoProducts'
import { useBalcaoCheckout, type BalcaoPaymentMethod } from '@/hooks/useBalcaoCheckout'
import {
  useBalcaoStore,
  selectActiveComanda,
  computeBalcaoPricing,
} from '@/store/balcaoStore'
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
  const role = useBalcaoStore((s) => s.role)
  // useMemo, não selector: um selector que devolve objeto novo a cada chamada
  // quebra o `getSnapshot` do useSyncExternalStore (loop de render).
  const pricing = useMemo(
    () => computeBalcaoPricing({ items: comanda.items, discountPct: comanda.discountPct, role }),
    [comanda.items, comanda.discountPct, role]
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
            requiresApproval: outcome.requiresApproval,
            customerName: comanda.customer?.name,
            nsu: outcome.external?.nsu,
          }
        : undefined,
    })
  }, [checkout.outcome, clearComanda, navigate, pricing.total, comanda.customer])

  const orderPanel = (
    <OrderPanel
      comanda={comanda}
      pricing={pricing}
      onIncrement={incrementItem}
      onDecrement={decrementItem}
      onRemove={removeItem}
      onDiscountChange={setDiscountPct}
      onCustomerChange={setCustomer}
      onCharge={openCharge}
    />
  )

  return (
    <div className="flex h-screen flex-col bg-gray-50">
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
          {orderPanel}
        </aside>
      </div>

      {/* Tela estreita: barra de resumo + gaveta */}
      <button
        type="button"
        onClick={() => setPanelOpen(true)}
        className="flex h-16 shrink-0 items-center justify-between gap-3 border-t border-gray-200 bg-brand-blue px-4 text-white lg:hidden"
      >
        <span className="flex items-center gap-2 font-semibold">
          <ShoppingCart className="h-5 w-5" aria-hidden="true" />
          Pedido
          <span className="rounded-full bg-white/20 px-2 py-0.5 text-xs font-bold">
            {pricing.itemCount}
          </span>
        </span>
        <span className="font-display text-lg font-bold">{formatCurrency(pricing.total)}</span>
      </button>

      <Drawer
        open={panelOpen}
        onClose={() => setPanelOpen(false)}
        side="bottom"
        title="Pedido do balcão"
        className="lg:hidden"
      >
        <div className="h-[75vh]">{orderPanel}</div>
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
      />
    </div>
  )
}
