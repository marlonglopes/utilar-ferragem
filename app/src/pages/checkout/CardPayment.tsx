import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ExternalLink, CheckCircle2, Lock, Loader2 } from 'lucide-react'
import { Elements, PaymentElement, useElements, useStripe } from '@stripe/react-stripe-js'
import type { Appearance, StripeElementsOptions } from '@stripe/stripe-js'
import { isApiEnabled } from '@/lib/api'
import { getStripe, isStripeConfigured } from '@/lib/stripe'
import { formatCurrency } from '@/lib/format'
import { Input } from '@/components/ui'
import type { PaymentResult } from '@/hooks/usePayment'

interface Props {
  result: PaymentResult
  amount: number
  // Nome pra pré-preencher "Nome no cartão" (ex.: o da conta). SEMPRE editável —
  // o cliente pode pagar com o cartão de outra pessoa, então o titular não é
  // necessariamente o dono da conta.
  defaultCardholderName?: string
  onSimulateConfirm?: () => void
  onConfirmed: () => void
  onFailed: (msg: string) => void
}

export default function CardPayment({
  result,
  amount,
  defaultCardholderName,
  onSimulateConfirm,
  onConfirmed,
  onFailed,
}: Props) {
  const { t } = useTranslation('checkout')

  if (result.status === 'confirmed') {
    return (
      <div className="flex flex-col items-center gap-4 py-10 text-center">
        <CheckCircle2 className="h-16 w-16 text-green-500" />
        <p className="text-xl font-bold text-gray-900">{t('pix.confirmed')}</p>
      </div>
    )
  }

  // Appmax path (FUTURO): quando o contrato/sandbox entrar e
  // `isAppmaxCardEnabled` (VITE_APPMAX_PUBLIC_KEY) ligar, o cartão é tokenizado
  // no browser via o ponto único `tokenizeCard` em src/lib/appmaxCard.ts e o
  // `card_token` vai pro createPayment ANTES da cobrança — sem clientSecret nem
  // redirect. Este componente é compartilhado com o balcão; ao ganhar o ramo
  // Appmax aqui, o PDV herda. Hoje o seam é dormente/fail-closed.

  // Stripe path: usa Elements com clientSecret
  if (result.provider === 'stripe' && result.clientSecret && isStripeConfigured) {
    return (
      <StripeCardForm
        clientSecret={result.clientSecret}
        amount={amount}
        defaultCardholderName={defaultCardholderName}
        onConfirmed={onConfirmed}
        onFailed={onFailed}
      />
    )
  }

  // Mercado Pago path: redirect pra Checkout Pro
  return (
    <div className="flex flex-col gap-5">
      <h2 className="text-lg font-bold text-gray-900">{t('card.title')}</h2>

      <p className="text-sm text-gray-500">{t('card.hosted')}</p>

      {result.initPoint && result.initPoint !== '#' ? (
        <a
          href={result.initPoint}
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center justify-center gap-2 h-11 rounded-xl bg-brand-blue text-white font-semibold text-sm hover:opacity-90 transition-opacity"
        >
          <ExternalLink className="h-4 w-4" />
          {t('card.proceed')}
        </a>
      ) : (
        <button
          disabled
          className="flex items-center justify-center gap-2 h-11 rounded-xl bg-brand-blue text-white font-semibold text-sm opacity-60 cursor-not-allowed"
        >
          <ExternalLink className="h-4 w-4" />
          {t('card.proceed')}
        </button>
      )}

      {!isApiEnabled && onSimulateConfirm && (
        <button
          onClick={onSimulateConfirm}
          className="h-10 rounded-xl border border-dashed border-gray-300 text-sm text-gray-500 hover:text-gray-700 hover:border-gray-400 transition-colors"
        >
          {t('card.simulate')}
        </button>
      )}
    </div>
  )
}

// ─── Stripe Elements form ─────────────────────────────────────────────────────

const stripePromise = getStripe()

const appearance: Appearance = {
  theme: 'stripe',
  variables: {
    colorPrimary: '#F47920',
    colorBackground: '#ffffff',
    colorText: '#111827',
    colorDanger: '#dc2626',
    fontFamily: 'system-ui, -apple-system, sans-serif',
    borderRadius: '12px',
  },
}

function StripeCardForm({
  clientSecret,
  amount,
  defaultCardholderName,
  onConfirmed,
  onFailed,
}: {
  clientSecret: string
  amount: number
  defaultCardholderName?: string
  onConfirmed: () => void
  onFailed: (msg: string) => void
}) {
  const options: StripeElementsOptions = {
    clientSecret,
    appearance,
    locale: 'pt-BR',
  }

  return (
    <Elements stripe={stripePromise} options={options}>
      <CardFormInner
        amount={amount}
        defaultCardholderName={defaultCardholderName}
        onConfirmed={onConfirmed}
        onFailed={onFailed}
      />
    </Elements>
  )
}

function CardFormInner({
  amount,
  defaultCardholderName,
  onConfirmed,
  onFailed,
}: {
  amount: number
  defaultCardholderName?: string
  onConfirmed: () => void
  onFailed: (msg: string) => void
}) {
  const { t } = useTranslation('checkout')
  const stripe = useStripe()
  const elements = useElements()

  const [submitting, setSubmitting] = useState(false)
  const [elementReady, setElementReady] = useState(false)
  const [errorMsg, setErrorMsg] = useState('')
  // Nome do TITULAR do cartão. Pré-preenchido com o nome da conta por conveniência,
  // mas editável: o cliente pode pagar com o cartão de outra pessoa. Coletamos aqui
  // (name:'never' no Element) e passamos no confirm — o Element de cartão não mostra
  // esse campo por padrão (a rede não exige nome pra autorizar).
  const [cardName, setCardName] = useState((defaultCardholderName ?? '').trim())
  const nameOk = cardName.trim().length >= 2

  // Timeout pra detectar falha de carregamento do Element (publishable key inválida etc)
  useEffect(() => {
    const id = setTimeout(() => {
      if (!elementReady) setErrorMsg(t('card.loadError'))
    }, 8000)
    return () => clearTimeout(id)
  }, [elementReady, t])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!stripe || !elements) return

    setSubmitting(true)
    setErrorMsg('')

    const { error, paymentIntent } = await stripe.confirmPayment({
      elements,
      confirmParams: {
        return_url: window.location.href, // só usado se algum método precisar redirect
        // name está como 'never' no Element, então o titular vai aqui (a Stripe
        // rejeita se um campo desabilitado não for informado no confirm). O CEP
        // (address:'if_required') o Element coleta sozinho — não precisa passar.
        payment_method_data: {
          billing_details: { name: cardName.trim() },
        },
      },
      redirect: 'if_required',
    })

    if (error) {
      const msg = error.message ?? 'Erro ao processar pagamento'
      setErrorMsg(msg)
      onFailed(msg)
      setSubmitting(false)
      return
    }

    if (paymentIntent) {
      switch (paymentIntent.status) {
        case 'succeeded':
        case 'processing':
          // processing = capture eventual (alguns processadores); webhook promove pra confirmed
          onConfirmed()
          break
        case 'requires_payment_method': {
          const msg = 'Cartão recusado. Tente outro cartão.'
          setErrorMsg(msg)
          onFailed(msg)
          break
        }
        case 'requires_action':
        case 'requires_confirmation':
        case 'requires_capture':
        default: {
          // Status que não chegamos a tratar — informa user em vez de spinner sumir silencioso.
          const msg = `Pagamento em estado inesperado: ${paymentIntent.status}. Tente novamente.`
          setErrorMsg(msg)
          onFailed(msg)
          break
        }
      }
    } else {
      // Sem error e sem paymentIntent — caso edge da API. Fail-safe.
      const msg = 'Resposta inesperada do processador de pagamento.'
      setErrorMsg(msg)
      onFailed(msg)
    }

    setSubmitting(false)
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-5">
      <h2 className="text-lg font-bold text-gray-900">{t('card.title')}</h2>

      <p className="flex items-start gap-2 text-xs text-gray-500 bg-gray-50 border border-gray-200 rounded-lg px-3 py-2">
        <Lock className="h-3.5 w-3.5 flex-shrink-0 mt-0.5" />
        {t('card.secure')}
      </p>

      {errorMsg && (
        <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2">
          {errorMsg}
        </p>
      )}

      <Input
        label="Nome no cartão"
        value={cardName}
        onChange={(e) => setCardName(e.target.value)}
        placeholder="Como impresso no cartão"
        autoComplete="cc-name"
        required
        hint="Pode ser diferente do titular da conta."
      />

      <div className="min-h-[240px]">
        {/* name:'never' → coletamos "Nome no cartão" acima; address:'if_required' →
            a Stripe pede só o CEP de cobrança quando precisa (AVS/antifraude). */}
        <PaymentElement
          options={{
            layout: 'tabs',
            fields: { billingDetails: { name: 'never', address: 'if_required' } },
          }}
          onReady={() => setElementReady(true)}
        />
      </div>

      <button
        type="submit"
        disabled={!stripe || !elementReady || submitting || !nameOk}
        className="h-11 rounded-xl bg-brand-orange hover:bg-brand-orange-dark text-gray-900 font-semibold text-sm flex items-center justify-center gap-2 transition-colors disabled:opacity-60 disabled:cursor-not-allowed"
      >
        {submitting ? (
          <>
            <Loader2 className="h-4 w-4 animate-spin" />
            {t('card.processing')}
          </>
        ) : (
          t('card.pay', { amount: formatCurrency(amount) })
        )}
      </button>
    </form>
  )
}
