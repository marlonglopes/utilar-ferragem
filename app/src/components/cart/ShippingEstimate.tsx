import { useState } from 'react'
import { Truck, Loader2 } from 'lucide-react'
import { formatCurrency } from '@/lib/format'
import {
  useShippingQuote,
  formatCep,
  isValidCep,
  type ShippingOption,
} from '@/hooks/useShippingQuote'

interface ShippingEstimateProps {
  subtotal: number
  itemCount: number
  /** Avisa o carrinho da opção escolhida, para somar no total. */
  onSelect: (opcao: ShippingOption | null) => void
}

/**
 * Cálculo de frete no carrinho, por CEP.
 *
 * PORQUÊ: o carrinho dizia "Calculado no checkout". No Brasil o cliente decide
 * a compra olhando o total COM entrega — descobrir o frete só no fim do funil é
 * das maiores causas de abandono, e obriga a refazer o caminho todo se o valor
 * assustar.
 */
export function ShippingEstimate({ subtotal, itemCount, onSelect }: ShippingEstimateProps) {
  const [cep, setCep] = useState('')
  const [escolhida, setEscolhida] = useState<string | null>(null)
  const { options, loading, error, quote, reset } = useShippingQuote()

  const podeCalcular = isValidCep(cep) && !loading

  async function calcular() {
    const res = await quote(cep, subtotal, itemCount)
    // Pré-seleciona a mais barata (a API já devolve ordenado) — é o que o
    // cliente escolheria na maioria das vezes, e deixa o total completo na tela
    // sem exigir mais um clique.
    if (res && res.length > 0) {
      setEscolhida(res[0].serviceCode)
      onSelect(res[0])
    } else {
      setEscolhida(null)
      onSelect(null)
    }
  }

  function trocarCep(valor: string) {
    setCep(formatCep(valor))
    if (options) {
      reset()
      setEscolhida(null)
      onSelect(null)
    }
  }

  function escolher(opcao: ShippingOption) {
    setEscolhida(opcao.serviceCode)
    onSelect(opcao)
  }

  return (
    <div className="flex flex-col gap-2">
      <label htmlFor="cep-frete" className="text-gray-500 text-sm flex items-center gap-1.5">
        <Truck className="h-4 w-4" aria-hidden="true" />
        Calcular frete
      </label>

      <div className="flex gap-2">
        <input
          id="cep-frete"
          type="text"
          inputMode="numeric"
          autoComplete="postal-code"
          placeholder="00000-000"
          value={cep}
          onChange={(e) => trocarCep(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && podeCalcular) calcular()
          }}
          aria-invalid={!!error}
          aria-describedby={error ? 'cep-frete-erro' : undefined}
          className="flex-1 min-w-0 h-11 px-3 rounded-lg border border-gray-300 text-sm
                     focus:outline-none focus:ring-2 focus:ring-brand-orange/30 focus:border-brand-orange"
        />
        <button
          type="button"
          onClick={calcular}
          disabled={!podeCalcular}
          className="h-11 px-4 rounded-lg bg-gray-900 text-white text-sm font-semibold
                     disabled:bg-gray-200 disabled:text-gray-400 disabled:cursor-not-allowed
                     hover:bg-gray-800 transition-colors flex items-center gap-2"
        >
          {loading ? <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" /> : null}
          {loading ? 'Calculando' : 'OK'}
        </button>
      </div>

      {error && (
        <p id="cep-frete-erro" role="alert" className="text-xs text-red-600">
          {error}
        </p>
      )}

      {options && options.length > 0 && (
        <ul className="flex flex-col gap-1.5 mt-1">
          {options.map((o) => (
            <li key={o.serviceCode}>
              <button
                type="button"
                onClick={() => escolher(o)}
                aria-pressed={escolhida === o.serviceCode}
                className={[
                  'w-full flex items-center justify-between gap-3 px-3 py-2.5 rounded-lg border text-left text-sm transition-colors',
                  escolhida === o.serviceCode
                    ? 'border-brand-orange bg-brand-orange/5'
                    : 'border-gray-200 hover:border-gray-300',
                ].join(' ')}
              >
                <span className="min-w-0">
                  <span className="block font-medium text-gray-900">{o.serviceName}</span>
                  <span className="block text-xs text-gray-500">
                    {o.deliveryDays} {o.deliveryDays === 1 ? 'dia útil' : 'dias úteis'}
                  </span>
                </span>
                {/* "Frete grátis" e não "R$ 0,00": o cliente lê como benefício,
                    não como valor. */}
                <span
                  className={
                    o.free
                      ? 'text-green-700 font-semibold whitespace-nowrap'
                      : 'text-gray-900 font-semibold tabular-nums whitespace-nowrap'
                  }
                >
                  {o.free ? 'Frete grátis' : formatCurrency(o.cost)}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
