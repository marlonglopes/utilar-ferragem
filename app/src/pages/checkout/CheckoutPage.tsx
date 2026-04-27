import { useState, useCallback, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { Check, ChevronRight, Loader2, Plus } from 'lucide-react'
import { useCartStore } from '@/store/cartStore'
import { useAuthStore } from '@/store/authStore'
import { useAddressStore, type Address as StoredAddress } from '@/store/addressStore'
import { usePayment, type PaymentMethod } from '@/hooks/usePayment'
import { formatCurrency, formatCEP } from '@/lib/format'
import { cn } from '@/lib/cn'
import { isApiEnabled, isOrderEnabled, orderPostWithJWT } from '@/lib/api'
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

function toShippingAddress(a: StoredAddress): Address {
  return {
    cep: a.cep,
    street: a.street,
    number: a.number,
    complement: a.complement,
    neighborhood: a.neighborhood,
    city: a.city,
    state: a.state,
  }
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
  const isLoggedIn = useAuthStore((s) => s.isLoggedIn())
  const savedAddresses = useAddressStore((s) => s.addresses)
  const addAddress = useAddressStore((s) => s.addAddress)

  const defaultAddress = useMemo(
    () => savedAddresses.find((a) => a.isDefault) ?? savedAddresses[0] ?? null,
    [savedAddresses]
  )

  const hasSaved = isLoggedIn && savedAddresses.length > 0
  const [selectedId, setSelectedId] = useState<string | null>(defaultAddress?.id ?? null)
  const [addingNew, setAddingNew] = useState(!hasSaved)
  const [saveForLater, setSaveForLater] = useState(isLoggedIn)

  const [form, setForm] = useState<Address>({
    cep: '', street: '', number: '', complement: '', neighborhood: '', city: '', state: '',
  })
  const [label, setLabel] = useState('')
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

  function pickSavedAddress(addr: StoredAddress) {
    onNext(toShippingAddress(addr))
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!addingNew && selectedId) {
      const picked = savedAddresses.find((a) => a.id === selectedId)
      if (picked) {
        onNext(toShippingAddress(picked))
        return
      }
    }
    if (saveForLater && isLoggedIn) {
      addAddress({ ...form, label: label || form.street }, savedAddresses.length === 0)
    }
    onNext(form)
  }

  if (hasSaved && !addingNew) {
    return (
      <form onSubmit={submit} className="flex flex-col gap-4">
        <h2 className="text-lg font-bold text-gray-900">{t('address.title')}</h2>

        <div className="flex flex-col gap-2">
          {savedAddresses.map((addr) => (
            <label
              key={addr.id}
              className={cn(
                'flex items-start gap-3 p-4 rounded-xl border cursor-pointer transition-colors',
                selectedId === addr.id
                  ? 'border-brand-orange bg-orange-50'
                  : 'border-gray-200 hover:border-gray-300'
              )}
            >
              <input
                type="radio"
                name="savedAddress"
                value={addr.id}
                checked={selectedId === addr.id}
                onChange={() => setSelectedId(addr.id)}
                className="accent-brand-orange mt-0.5"
              />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-semibold text-gray-900">
                  {addr.label || addr.street}
                  {addr.isDefault && (
                    <span className="ml-2 text-[10px] font-bold uppercase tracking-wide bg-brand-orange text-white px-1.5 py-0.5 rounded-full">
                      {t('account.defaultAddress', { ns: 'common' })}
                    </span>
                  )}
                </p>
                <p className="text-xs text-gray-600 mt-0.5">
                  {addr.street}{addr.number ? `, ${addr.number}` : ''}{addr.complement ? ` - ${addr.complement}` : ''}
                </p>
                <p className="text-xs text-gray-500">
                  {addr.neighborhood}, {addr.city} – {addr.state} · {addr.cep}
                </p>
              </div>
              {selectedId === addr.id && (
                <button
                  type="button"
                  onClick={(e) => { e.preventDefault(); pickSavedAddress(addr) }}
                  className="text-xs font-semibold text-brand-orange hover:text-brand-orange-dark whitespace-nowrap"
                >
                  {t('address.useThis')}
                </button>
              )}
            </label>
          ))}
        </div>

        <button
          type="button"
          onClick={() => setAddingNew(true)}
          className="flex items-center gap-2 h-10 px-4 rounded-xl border border-dashed border-gray-300 text-sm text-gray-500 hover:border-brand-orange hover:text-brand-orange transition-colors self-start"
        >
          <Plus className="h-4 w-4" />
          {t('address.addNew')}
        </button>

        <button
          type="submit"
          disabled={!selectedId}
          className="h-11 rounded-xl bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold text-sm flex items-center justify-center gap-2 transition-colors mt-2 disabled:opacity-60"
        >
          {t('address.continue')}
          <ChevronRight className="h-4 w-4" />
        </button>
      </form>
    )
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

      {isLoggedIn && (
        <>
          <Input
            label={t('address.label', { defaultValue: 'Nome do endereço (ex: Casa, Trabalho)' })}
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            placeholder="Casa"
          />
          <label className="flex items-center gap-2 text-sm text-gray-600 cursor-pointer">
            <input
              type="checkbox"
              checked={saveForLater}
              onChange={(e) => setSaveForLater(e.target.checked)}
              className="accent-brand-orange"
            />
            {t('address.saveForLater', { defaultValue: 'Salvar este endereço para próximas compras' })}
          </label>
        </>
      )}

      <div className="flex gap-2 mt-2">
        {hasSaved && (
          <button
            type="button"
            onClick={() => setAddingNew(false)}
            className="h-11 px-4 rounded-xl border border-gray-300 text-sm text-gray-600 hover:bg-gray-50 transition-colors"
          >
            {t('cancel', { ns: 'common' })}
          </button>
        )}
        <button
          type="submit"
          className="flex-1 h-11 rounded-xl bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold text-sm flex items-center justify-center gap-2 transition-colors"
        >
          {t('address.continue')}
          <ChevronRight className="h-4 w-4" />
        </button>
      </div>
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
  ensureOrderId,
  onPaymentCreated,
}: {
  total: number
  ensureOrderId: (method: PaymentMethod) => Promise<string>
  onPaymentCreated: (paymentId: string, method: PaymentMethod) => void
}) {
  const { t } = useTranslation('checkout')
  const { result, error, createPayment, simulateConfirm, markConfirmed, markFailed } = usePayment()
  const user = useAuthStore((s) => s.user)
  const [method, setMethod] = useState<PaymentMethod>('pix')
  // U1: pre-fill com CPF/Nome do user logado quando disponível.
  // Backend (M6) também busca de /auth/me, mas pre-fill é melhor UX.
  // Bug B: re-sincroniza se user mudar durante o checkout (login/logout).
  const [payerCPF, setPayerCPF] = useState(() => user?.cpf ?? '')
  const [payerName, setPayerName] = useState(() => user?.name ?? '')
  useEffect(() => {
    if (user?.cpf) setPayerCPF(user.cpf)
    if (user?.name) setPayerName(user.name)
  }, [user?.cpf, user?.name])

  // Persiste a orderId após o primeiro POST /orders bem-sucedido pra evitar
  // criar pedidos duplicados em retries (regenerate boleto/pix, falha no PSP).
  const [committedOrderId, setCommittedOrderId] = useState<string | null>(null)
  const [orderError, setOrderError] = useState<string | null>(null)

  // U2: invalida o pedido cacheado quando:
  // (a) ensureOrderId muda — items/endereço/frete diferentes, OU
  // (b) method muda — Bug A: order tem paymentMethod gravado no DB que precisa
  //     bater com o que o user vai pagar. Order criada como `card` não pode ser
  //     reusada pra `pix`/`boleto`. Cria nova order com método novo.
  useEffect(() => {
    setCommittedOrderId(null)
  }, [ensureOrderId, method])

  const pixTotal = +(total * 0.95).toFixed(2)
  const displayTotal = method === 'pix' ? pixTotal : total

  // Boleto requer CPF + nome (Stripe rejeita sem isso). Validação simples client-side.
  const boletoReady = method !== 'boleto' || (
    payerCPF.replace(/\D/g, '').length === 11 && payerName.trim().length >= 3
  )

  // Garante que existe um order id real do backend antes de criar payment.
  // Usa cache (committedOrderId) — só cria UMA order por sessão de checkout.
  async function getOrCreateOrderId(): Promise<string | null> {
    if (committedOrderId) return committedOrderId
    setOrderError(null)
    try {
      const id = await ensureOrderId(method)
      setCommittedOrderId(id)
      return id
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Erro ao criar pedido'
      setOrderError(msg)
      return null
    }
  }

  async function handleConfirm() {
    const orderId = await getOrCreateOrderId()
    if (!orderId) return
    const extras = method === 'boleto'
      ? { payer_cpf: payerCPF.replace(/\D/g, ''), payer_name: payerName.trim() }
      : undefined
    const res = await createPayment(orderId, method, displayTotal, extras)
    if (!res) return
    // Pix/Boleto: navega imediatamente pra página de status (poll continua lá ou aqui).
    // Card: NÃO navega — aguarda Stripe Elements confirmar via onConfirmed.
    if (method === 'pix' || method === 'boleto') {
      onPaymentCreated(res.paymentId, method)
    }
  }

  async function handleRegenerate() {
    const orderId = committedOrderId ?? await getOrCreateOrderId()
    if (!orderId) return
    await createPayment(orderId, method, displayTotal)
  }

  function handleStripeConfirmed() {
    markConfirmed()
    if (result) onPaymentCreated(result.paymentId, method)
  }

  // B1: pra cartão, criar o PaymentIntent automaticamente quando o user
  // seleciona o método. Stripe Elements precisa de clientSecret pra montar;
  // sem isso, ficamos com tela de método selecionado e nenhum input de cartão
  // aparece até clicar "Pagar" — UX confusa.
  // Pix/Boleto continuam sob clique do "Pagar" porque o resultado é um QR
  // ou boleto que o user precisa decidir gerar conscientemente.
  useEffect(() => {
    if (method !== 'card') return
    // Já temos resultado pra este método? Não recria.
    if (result && result.method === 'card' && result.status !== 'creating') return
    // Já está em criação? Não dispara duplicado.
    if (result?.status === 'creating') return
    void (async () => {
      const orderId = await getOrCreateOrderId()
      if (!orderId) return
      await createPayment(orderId, 'card', displayTotal)
    })()
    // Disparamos quando o método muda OU o total muda (ex: shipping diferente).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [method, displayTotal])

  // Result que efetivamente é desta sessão pra esse método (renderiza componente específico).
  const activeResult = result && result.method === method && result.status !== 'creating'
    ? result
    : null
  // Só considera "creating" se for do método atual — evita travar botão quando
  // user troca de método durante criação anterior em flight.
  const isCreating = result?.status === 'creating' && result?.method === method

  return (
    <div className="flex flex-col gap-5">
      <h2 className="text-lg font-bold text-gray-900">{t('payment.title')}</h2>

      {error && (
        <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2">{error}</p>
      )}
      {orderError && (
        <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2">{orderError}</p>
      )}

      {/* Method selector — sempre visível pra permitir trocar de método */}
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

      {method === 'pix' && !activeResult && (
        <p className="text-xs text-green-700 bg-green-50 border border-green-200 rounded-lg px-3 py-2">
          {t('payment.pixDiscount')} — {formatCurrency(pixTotal)}
        </p>
      )}

      {method === 'boleto' && !activeResult && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <Input
            label="CPF do pagador"
            value={payerCPF}
            onChange={(e) => setPayerCPF(e.target.value)}
            placeholder="000.000.000-00"
            maxLength={14}
            required
          />
          <Input
            label="Nome completo"
            value={payerName}
            onChange={(e) => setPayerName(e.target.value)}
            placeholder="Como no documento"
            required
          />
        </div>
      )}

      {/* Resultado/UI específico por método */}
      {activeResult && method === 'pix' && (
        <PixPayment
          result={activeResult}
          onRegenerate={handleRegenerate}
          onSimulateConfirm={!isApiEnabled ? simulateConfirm : undefined}
        />
      )}
      {activeResult && method === 'boleto' && <BoletoPayment result={activeResult} />}
      {activeResult && method === 'card' && (
        <CardPayment
          result={activeResult}
          amount={displayTotal}
          onSimulateConfirm={!isApiEnabled ? simulateConfirm : undefined}
          onConfirmed={handleStripeConfirmed}
          onFailed={(msg) => markFailed(msg)}
        />
      )}

      {/* Botão "Pagar"/"Continuar" sempre visível quando ainda não há resultado
          ativo do método selecionado. Pra card, o useEffect de B1 dispara o
          create automaticamente, mas o botão fica como retry quando falha
          (ex: STRIPE_SECRET_KEY dummy → 502).
          Quando o card cria com sucesso e Stripe Elements monta, este botão
          some e o submit passa a ser o do form do Elements. */}
      {!activeResult && (
        <button
          onClick={handleConfirm}
          disabled={isCreating || !boletoReady}
          className="h-11 rounded-xl bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold text-sm flex items-center justify-center gap-2 transition-colors disabled:opacity-60 disabled:cursor-not-allowed"
        >
          {isCreating ? (
            <>
              <Loader2 className="h-4 w-4 animate-spin" />
              {method === 'card'
                ? t('card.preparing', { defaultValue: 'Preparando formulário do cartão...' })
                : 'Processando...'}
            </>
          ) : (
            <>
              {method === 'card'
                ? t('card.continue', { defaultValue: 'Continuar com cartão' })
                : t('payment.confirm')}
              <ChevronRight className="h-4 w-4" />
            </>
          )}
        </button>
      )}
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
  const accessToken = useAuthStore((s) => s.user?.token ?? null)

  const [step, setStep] = useState<Step>('address')
  const [address, setAddress] = useState<Address | null>(null)
  const [shipping, setShipping] = useState<ShippingOption | null>(null)

  const subtotal = items.reduce((sum, i) => sum + i.priceSnapshot * i.quantity, 0)
  const shippingCost = shipping?.price ?? 0
  const total = subtotal + shippingCost

  const handleAddressDone = useCallback((addr: Address) => {
    setAddress(addr)
    setStep('shipping')
  }, [])

  const handleShippingDone = useCallback((option: ShippingOption) => {
    setShipping(option)
    setStep('payment')
  }, [])

  // U5: cacheia o orderNumber retornado pelo order-service pra mostrar
  // (humano "2026-XYZ") na tela de confirmação em vez do UUID do payment.
  const [committedOrderNumber, setCommittedOrderNumber] = useState<string | null>(null)

  // Cria o pedido no order-service quando o usuário confirma o pagamento.
  // Mantém um único pedido por sessão de checkout (PaymentStep cacheia o id).
  const ensureOrderId = useCallback(
    async (method: PaymentMethod): Promise<string> => {
      if (!isOrderEnabled || !accessToken || !address) {
        // Fallback dev/mock: gera UUID local. Backend real recusará via 404 se for chamado.
        return crypto.randomUUID()
      }
      const payload = {
        paymentMethod: method,
        shippingCost,
        items: items.map((i) => ({
          productId: i.productId,
          name: i.name,
          icon: i.icon,
          sellerId: i.sellerId,
          sellerName: i.sellerName,
          quantity: i.quantity,
          unitPrice: i.priceSnapshot,
        })),
        address: {
          street: address.street,
          number: address.number,
          complement: address.complement || undefined,
          neighborhood: address.neighborhood,
          city: address.city,
          state: address.state,
          cep: address.cep.replace(/\D/g, '').replace(/^(\d{5})(\d{3})$/, '$1-$2'),
        },
      }
      const order = await orderPostWithJWT<{ id: string; number: string }>(
        '/api/v1/orders',
        accessToken,
        payload,
      )
      setCommittedOrderNumber(order.number)
      return order.id
    },
    [accessToken, address, items, shippingCost],
  )

  const handlePaymentCreated = useCallback(
    (paymentId: string, method: PaymentMethod) => {
      clearCart()
      navigate(`/pedido/${paymentId}?method=${method}`, {
        state: committedOrderNumber ? { orderNumber: committedOrderNumber } : undefined,
      })
    },
    [clearCart, navigate, committedOrderNumber],
  )

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
              ensureOrderId={ensureOrderId}
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
