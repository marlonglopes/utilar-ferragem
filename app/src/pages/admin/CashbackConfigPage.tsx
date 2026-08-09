import { useEffect, useState } from 'react'
import { Loader2, Coins } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import { ErrorState, LoadingRows, Section } from '@/components/admin/primitives'
import { useCashbackConfig, useUpdateCashbackConfig } from '@/hooks/useCashback'

/**
 * Configuração do programa de cashback — admin only (é dinheiro/passivo da loja).
 * Liga/desliga e ajusta taxa de acúmulo, teto de resgate e validade. As regras
 * são aplicadas no servidor (order-service); aqui só se define a política.
 */
export default function CashbackConfigPage() {
  const { data, isLoading, isError, error, refetch } = useCashbackConfig()
  const save = useUpdateCashbackConfig()

  const [active, setActive] = useState(false)
  const [earn, setEarn] = useState('5')
  const [redeem, setRedeem] = useState('50')
  const [validity, setValidity] = useState('90')
  const [formError, setFormError] = useState('')
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (data) {
      setActive(data.active)
      setEarn(String(data.earnRatePct))
      setRedeem(String(data.redeemMaxPct))
      setValidity(String(data.validityDays))
    }
  }, [data])

  function submit(e: React.FormEvent) {
    e.preventDefault()
    setFormError('')
    setSaved(false)
    const earnPct = Number(earn.replace(',', '.'))
    const redeemPct = Number(redeem.replace(',', '.'))
    const days = Number(validity)
    if (!Number.isFinite(earnPct) || earnPct < 0 || earnPct > 100) {
      return setFormError('Taxa de acúmulo deve ficar entre 0 e 100%.')
    }
    if (!Number.isFinite(redeemPct) || redeemPct < 0 || redeemPct > 100) {
      return setFormError('Teto de resgate deve ficar entre 0 e 100%.')
    }
    if (!Number.isInteger(days) || days < 1) {
      return setFormError('Validade deve ser de pelo menos 1 dia.')
    }
    save.mutate(
      { active, earnRatePct: earnPct, redeemMaxPct: redeemPct, validityDays: days },
      {
        onSuccess: () => setSaved(true),
        onError: (err) => setFormError(err instanceof Error ? err.message : 'Falha ao salvar.'),
      }
    )
  }

  return (
    <AdminShell
      title="Cashback"
      description="Quanto o cliente ganha de volta, quanto pode usar e por quanto tempo vale."
    >
      {isLoading ? (
        <LoadingRows rows={3} />
      ) : isError ? (
        <ErrorState
          message={error instanceof Error ? error.message : 'Falha ao carregar.'}
          onRetry={() => void refetch()}
        />
      ) : (
        <Section title="Programa de cashback">
          <form onSubmit={submit} className="flex flex-col gap-4 p-3 sm:p-4">
            <label className="flex items-center gap-2 text-sm font-medium text-gray-800">
              <input
                type="checkbox"
                checked={active}
                onChange={(e) => setActive(e.target.checked)}
                className="h-4 w-4"
              />
              Programa ativo (clientes acumulam e podem resgatar)
            </label>

            <div className="flex flex-wrap gap-4">
              <label className="flex flex-col text-xs font-semibold text-gray-600">
                Acúmulo (% do valor pago)
                <input
                  value={earn}
                  onChange={(e) => setEarn(e.target.value.replace(/[^\d.,]/g, ''))}
                  inputMode="decimal"
                  className="mt-1 h-10 w-32 rounded-md border border-gray-300 px-2 text-sm"
                />
              </label>
              <label className="flex flex-col text-xs font-semibold text-gray-600">
                Teto de resgate (% do pedido)
                <input
                  value={redeem}
                  onChange={(e) => setRedeem(e.target.value.replace(/[^\d.,]/g, ''))}
                  inputMode="decimal"
                  className="mt-1 h-10 w-32 rounded-md border border-gray-300 px-2 text-sm"
                />
              </label>
              <label className="flex flex-col text-xs font-semibold text-gray-600">
                Validade (dias)
                <input
                  value={validity}
                  onChange={(e) => setValidity(e.target.value.replace(/\D/g, ''))}
                  inputMode="numeric"
                  className="mt-1 h-10 w-32 rounded-md border border-gray-300 px-2 text-sm"
                />
              </label>
            </div>

            <p className="flex items-center gap-1.5 rounded-md bg-amber-50 p-2 text-xs text-amber-800">
              <Coins className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
              Cashback é dívida da loja com o cliente. Mudanças valem para acúmulos futuros; o saldo
              já concedido continua com a validade original.
            </p>

            <div className="flex items-center gap-3">
              <button
                type="submit"
                disabled={save.isPending}
                className="inline-flex h-10 items-center gap-1.5 rounded-md bg-brand-orange px-4 text-sm font-semibold text-gray-900 hover:bg-brand-orange-dark disabled:opacity-50"
              >
                {save.isPending && <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />}
                Salvar
              </button>
              {saved && !save.isPending && (
                <span className="text-sm font-medium text-green-600" role="status">
                  Salvo.
                </span>
              )}
              {formError && (
                <span className="text-sm font-semibold text-red-600" role="alert">
                  {formError}
                </span>
              )}
            </div>
          </form>
        </Section>
      )}
    </AdminShell>
  )
}
