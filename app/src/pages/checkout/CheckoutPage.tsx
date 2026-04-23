import { useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { Check, ChevronRight, Loader2 } from 'lucide-react'
import { useCartStore } from '@/store/cartStore'
import { usePayment, type PaymentMethod } from '@/hooks/usePayment'
import { formatCurrency, formatCEP } from '@/lib/format'
import { cn } from '@/lib/cn'
import { isApiEnabled } from '@/lib/api'
import { Input } from '@/components/ui'
import PixPayment from './PixPayment'
import BoletoPayment from './BoletoPayment'
import CardPayment from './CardPayment'

type Step = 'address' | 'shipping' | 'payment'
const STEPS: Step[] = ['address', 'shipping', 'payment']

interface Address {
  cep: string
  street: string
  number: string
  complement: string
  neighborhood: string
  city: string
  state: string
}

interface ShippingOption {
  id: string
  label: string
  price: number
  days: number
}

const SHIPPING_OPTIONS: ShippingOption[] = [
  { id: 'free', label: 'Grátis', price: 0, days: 10 },
  { id: 'pac', label: 'PAC — Correios', price: 15.9, days: 7 },
  { id: 'sedex', label: 'SEDEX — Correios', price: 38.9, days: 2 },
]

// ─── Step indicator ──────────────────────────────────────────────────────────

function Stepper({ current }: { current: Step }) {
  const { t } = useTranslation('checkout')
  const currentIdx = STEPS.indexOf(current)
  return (
    <div className="flex items-center gap-0 mb-8">
      {STEPS.map((step, idx) => {
        const done = idx < currentIdx
        const active = idx === currentIdx
        return (
          <div key={step} className="flex items-center gap-0">
            <div className="flex flex-col items-center gap-1">
              <div
                className={cn(
                  'w-8 h-8 rounded-full flex items-center justify-center text-sm font-bold transition-colors',
                  done && 'bg-green-500 text-white',
                  active && 'bg-brand-orange text-white',
                  !done && !active && 'bg-gray-100 text-gray-400'
                )}
              >
                {done ? <Check className="h-4 w-4" /> : idx + 1}
              </div>
              <span className={cn('text-xs font-medium', active ? 'text-brand-orange' : 'text-gray-400')}>
                {t(`steps.${step}`)}
              </span>
            </div>
            {idx < STEPS.length - 1 && (
              <div className={cn('w-16 h-0.5 mb-4 mx-1', idx < currentIdx ? 'bg-green-400' : 'bg-gray-200')} />
            )}
          </div>
        )
      })}
    </div>
  )
}

// ─── Address step ─────────────────────────────────────────────────────────────

function AddressStep({
  onNext,
}: {
  onNext: (addr: Address) => void
}) {
  const { t } = useTranslation('checkout')
  const [form, setForm] = useState<Address>({
    cep: '', street: '', number: '', complement: '', neighborhood: '', city: '', state: '',
  })
  const [cepLoading, setCepLoading] = useState(false)

  function set(field: keyof Address, value: string) {
    setForm((prev) => ({ ...prev, [field]: value }))
  }

  async function handleCEPBlur() {
    const digits = form.cep.replace(/\D/g, '')
    if (digits.length !== 8) return
    setCepLoading(true)
    try {
      const res = await fetch(`https://viacep.com.br/ws/${digits}/json/`)
      const data = await res.json()
      if (!data.erro) {
        setForm((prev) => ({
          ...prev,
          street: data.logradouro ?? prev.street,
          neighborhood: data.bairro ?? prev.neighborhood,
          city: data.localidade ?? prev.city,
          state: data.uf ?? prev.state,
        }))
      }
    } catch { /* ignore */ }
    setCepLoading(false)
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    onNext(form)
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <h2 className="text-lg font-bold text-gray-900">{t('address.title')}</h2>

      <div className="grid grid-cols-2 gap-4">
        <div className="col-span-2 sm:col-span-1">
          <Input
            label={t('address.cep')}
            value={form.cep}
            onChange={(e) => set('cep', formatCEP(e.target.value))}
            onBlur={handleCEPBlur}
            placeholder="00000-000"
            maxLength={9}
            required
            hint={cepLoading ? 'Buscando...' : undefined}
          />
        </div>
      </div>

      <Input
        label={t('address.street')}
        value={form.street}
        onChange={(e) => set('street', e.target.value)}
        required
      />

      <div className="grid grid-cols-2 gap-4">
        <Input
          label={t('address.number')}
          value={form.number}
          onChange={(e) => set('number', e.target.value)}
          required
        />
        <Input
          label={t('address.complement')}
          value={form.complement}
          onChange={(e) => set('complement', e.target.value)}
        />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <Input
          label={t('address.neighborhood')}
          value={form.neighborhood}
          onChange={(e) => set('neighborhood', e.target.value)}
          required
        />
        <Input
          label={t('address.city')}
          value={form.city}
          onChange={(e) => set('city', e.target.value)}
          required
        />
      </div>

      <Input
        label={t('address.state')}
        value={form.state}
        onChange={(e) => set('state', e.target.value)}
        maxLength={2}
        required
      />

      <button
        type="submit"
        className="h-11 rounded-xl bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold text-sm flex items-center justify-center gap-2 transition-colors mt-2"
      >
        {t('address.continue')}
        <ChevronRight className="h-4 w-4" />
      </button>
    </form>
  )
}

// ─── Shipping step ────────────────────────────────────────────────────────────

function ShippingStep({
  onNext,
}: {
  onNext: (option: ShippingOption) => void
}) {
  const { t } = useTranslation('checkout')
  const [selected, setSelected] = useState<string>('free')

  const option = SHIPPING_OPTIONS.find((o) => o.id === selected)!

  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-lg font-bold text-gray-900">{t('shipping.title')}</h2>

      <p className="text-xs text-amber-600 bg-amber-50 border border-amber-200 rounded-lg px-3 py-2">
        {t('shipping.stub')}
      </p>

      <div className="flex flex-col gap-2">
        {SHIPPING_OPTIONS.map((opt) => (
          <label
            key={opt.id}
            className={cn(
              'flex items-center justify-between p-4 rounded-xl border cursor-pointer transition-colors',
              selected === opt.id
                ? 'border-brand-orange bg-orange-50'
                : 'border-gray-200 hover:border-gray-300'
            )}
          >
            <div className="flex items-center gap-3">
              <input
                type="radio"
                name="shipping"
                value={opt.id}
                checked={selected === opt.id}
                onChange={() => setSelected(opt.id)}
                className="accent-brand-orange"
              />
              <div>
                <p className="text-sm font-medium text-gray-900">{opt.label}</p>
                <p className="text-xs text-gray-400">
                  {t(opt.days === 1 ? 'shipping.days' : 'shipping.days_plural', { count: opt.days })}
                </p>
              </div>
            </div>
            <span className="text-sm font-semibold text-gray-900">
              {opt.price === 0 ? t('shipping.free') : formatCurrency(opt.price)}
            </span>
          </label>
        ))}
      </div>

      <button
        onClick={() => onNext(option)}
        className="h-11 rounded-xl bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold text-sm flex items-center justify-center gap-2 transition-colors mt-2"
      >
        {t('shipping.continue')}
        <ChevronRight className="h-4 w-4" />
      </button>
    </div>
  )
}

// ─── Payment step ─────────────────────────────────────────────────────────────

function PaymentStep({
  total,
  orderId,
  onPaymentCreated,
}: {
  total: number
  orderId: string
  onPaymentCreated: (paymentId: string, method: PaymentMethod) => void
}) {
  const { t } = useTranslation('checkout')
  const { result, error, createPayment, simulateConfirm } = usePayment()
  const [method, setMethod] = useState<PaymentMethod>('pix')

  const pixTotal = +(total * 0.95).toFixed(2)
  const displayTotal = method === 'pix' ? pixTotal : total

  async function handleConfirm() {
    const res = await createPayment(orderId, method, displayTotal)
    if (res) {
      onPaymentCreated(res.paymentId, method)
    }
  }

  async function handleRegenerate() {
    await createPayment(orderId, method, displayTotal)
  }

  if (result && result.status !== 'creating') {
    return (
      <div className="flex flex-col gap-5">
        {method === 'pix' && (
          <PixPayment
            result={result}
            onRegenerate={handleRegenerate}
            onSimulateConfirm={!isApiEnabled ? simulateConfirm : undefined}
          />
        )}
        {method === 'boleto' && <BoletoPayment result={result} />}
        {method === 'card' && (
          <CardPayment
            result={result}
            onSimulateConfirm={!isApiEnabled ? simulateConfirm : undefined}
          />
        )}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      <h2 className="text-lg font-bold text-gray-900">{t('payment.title')}</h2>

      {error && (
        <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2">{error}</p>
      )}

      {/* Method selector */}
      <div className="grid grid-cols-3 gap-3">
        {(['pix', 'boleto', 'card'] as PaymentMethod[]).map((m) => (
          <button
            key={m}
            onClick={() => setMethod(m)}
            className={cn(
              'h-14 rounded-xl border-2 text-sm font-semibold flex flex-col items-center justify-center gap-0.5 transition-colors',
              method === m
                ? 'border-brand-orange bg-orange-50 text-brand-orange'
                : 'border-gray-200 text-gray-500 hover:border-gray-300'
            )}
          >
            {m === 'pix' && '⚡'}
            {m === 'boleto' && '🧾'}
            {m === 'card' && '💳'}
            <span className="text-xs">
              {m === 'pix' && t('payment.pix')}
              {m === 'boleto' && t('payment.boleto')}
              {m === 'card' && t('payment.creditCard')}
            </span>
          </button>
        ))}
      </div>

      {method === 'pix' && (
        <p className="text-xs text-green-700 bg-green-50 border border-green-200 rounded-lg px-3 py-2">
          {t('payment.pixDiscount')} — {formatCurrency(pixTotal)}
        </p>
      )}

      <button
        onClick={handleConfirm}
        disabled={result?.status === 'creating'}
        className="h-11 rounded-xl bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold text-sm flex items-center justify-center gap-2 transition-colors disabled:opacity-60 disabled:cursor-wait"
      >
        {result?.status === 'creating' ? (
          <>
            <Loader2 className="h-4 w-4 animate-spin" />
            Processando...
          </>
        ) : (
          <>
            {t('payment.confirm')}
            <ChevronRight className="h-4 w-4" />
          </>
        )}
      </button>
    </div>
  )
}

// ─── Order summary sidebar ────────────────────────────────────────────────────

function OrderSummary({ shipping }: { shipping: ShippingOption | null }) {
  const { t } = useTranslation('checkout')
  const items = useCartStore((s) => s.items)

  const subtotal = items.reduce((sum, i) => sum + i.priceSnapshot * i.quantity, 0)
  const shippingCost = shipping?.price ?? 0
  const total = subtotal + shippingCost

  return (
    <div className="bg-white border border-gray-200 rounded-xl p-5 flex flex-col gap-4 lg:sticky lg:top-24">
      <h2 className="font-semibold text-gray-900 text-base">{t('order.summary')}</h2>

      <div className="flex flex-col gap-2 max-h-52 overflow-y-auto">
        {items.map((item) => (
          <div key={item.productId} className="flex items-center justify-between gap-2 text-sm">
            <span className="text-gray-500 truncate">
              {item.icon} {item.name} × {item.quantity}
            </span>
            <span className="font-medium text-gray-900 flex-shrink-0">
              {formatCurrency(item.priceSnapshot * item.quantity)}
            </span>
          </div>
        ))}
      </div>

      <div className="border-t border-gray-100 pt-3 flex flex-col gap-2 text-sm">
        <div className="flex items-center justify-between">
          <span className="text-gray-500">{t('checkout:cart.subtotal')}</span>
          <span className="font-medium text-gray-900">{formatCurrency(subtotal)}</span>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-gray-500">{t('checkout:cart.shipping')}</span>
          <span className="font-medium text-gray-900">
            {shippingCost === 0 ? (
              <span className="text-green-600">{t('shipping.free')}</span>
            ) : (
              formatCurrency(shippingCost)
            )}
          </span>
        </div>
      </div>

      <div className="border-t border-gray-100 pt-3 flex items-center justify-between">
        <span className="font-semibold text-gray-900">{t('checkout:cart.total')}</span>
        <span className="font-bold text-xl text-gray-900">{formatCurrency(total)}</span>
      </div>
    </div>
  )
}

// ─── CheckoutPage ──────────────────────────────────────────────────────────────

export default function CheckoutPage() {
  const navigate = useNavigate()
  const items = useCartStore((s) => s.items)
  const clearCart = useCartStore((s) => s.clearCart)

  const [step, setStep] = useState<Step>('address')
  const [_address, setAddress] = useState<Address | null>(null)
  const [shipping, setShipping] = useState<ShippingOption | null>(null)

  const subtotal = items.reduce((sum, i) => sum + i.priceSnapshot * i.quantity, 0)
  const shippingCost = shipping?.price ?? 0
  const total = subtotal + shippingCost

  const orderId = `order-${Date.now()}`

  const handleAddressDone = useCallback((addr: Address) => {
    setAddress(addr)
    setStep('shipping')
  }, [])

  const handleShippingDone = useCallback((option: ShippingOption) => {
    setShipping(option)
    setStep('payment')
  }, [])

  const handlePaymentCreated = useCallback((paymentId: string, method: PaymentMethod) => {
    clearCart()
    navigate(`/pedido/${paymentId}?method=${method}`)
  }, [clearCart, navigate])

  if (items.length === 0 && step === 'address') {
    navigate('/carrinho')
    return null
  }

  return (
    <div className="container py-6">
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 items-start">
        {/* Main wizard */}
        <div className="lg:col-span-2 bg-white border border-gray-200 rounded-xl p-6">
          <Stepper current={step} />

          {step === 'address' && (
            <AddressStep onNext={handleAddressDone} />
          )}
          {step === 'shipping' && (
            <ShippingStep onNext={handleShippingDone} />
          )}
          {step === 'payment' && (
            <PaymentStep
              total={total}
              orderId={orderId}
              onPaymentCreated={handlePaymentCreated}
            />
          )}
        </div>

        {/* Order summary */}
        <OrderSummary shipping={shipping} />
      </div>
    </div>
  )
}
