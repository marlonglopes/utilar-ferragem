import { useEffect, useState } from 'react'
import { Loader2, Coins } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import { ErrorState, LoadingRows, Section } from '@/components/admin/primitives'
import {
  useCashbackConfig,
  useUpdateCashbackConfig,
  useCashbackCategoryRates,
  useSetCategoryRate,
  useDeleteCategoryRate,
} from '@/hooks/useCashback'
import { useAdminCategories } from '@/hooks/useAdminCategories'

// ISO (do servidor) → valor do <input type="datetime-local"> ("YYYY-MM-DDTHH:mm"
// no fuso local). Vazio quando não há data.
function toLocalInput(iso: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// valor do datetime-local → ISO (RFC3339) pro servidor; null quando vazio.
function fromLocalInput(v: string): string | null {
  if (!v) return null
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? null : d.toISOString()
}

/**
 * Configuração do programa de cashback — admin only (é dinheiro/passivo da loja).
 * Liga/desliga e ajusta taxa de acúmulo, teto de resgate, validade, pedidos
 * mínimos e campanha por período. As regras são aplicadas no servidor
 * (order-service); aqui só se define a política.
 */
export default function CashbackConfigPage() {
  const { data, isLoading, isError, error, refetch } = useCashbackConfig()
  const save = useUpdateCashbackConfig()

  const [active, setActive] = useState(false)
  const [earn, setEarn] = useState('5')
  const [redeem, setRedeem] = useState('50')
  const [validity, setValidity] = useState('90')
  const [minEarn, setMinEarn] = useState('0')
  const [minRedeem, setMinRedeem] = useState('0')
  const [campRate, setCampRate] = useState('0')
  const [campStart, setCampStart] = useState('')
  const [campEnd, setCampEnd] = useState('')
  const [formError, setFormError] = useState('')
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (data) {
      setActive(data.active)
      setEarn(String(data.earnRatePct))
      setRedeem(String(data.redeemMaxPct))
      setValidity(String(data.validityDays))
      setMinEarn(String(data.minEarnSubtotal))
      setMinRedeem(String(data.minRedeemSubtotal))
      setCampRate(String(data.campaignRatePct))
      setCampStart(toLocalInput(data.campaignStartsAt))
      setCampEnd(toLocalInput(data.campaignEndsAt))
    }
  }, [data])

  function submit(e: React.FormEvent) {
    e.preventDefault()
    setFormError('')
    setSaved(false)
    const earnPct = Number(earn.replace(',', '.'))
    const redeemPct = Number(redeem.replace(',', '.'))
    const days = Number(validity)
    const minEarnV = Number(minEarn.replace(',', '.'))
    const minRedeemV = Number(minRedeem.replace(',', '.'))
    const campPct = Number(campRate.replace(',', '.'))
    if (!Number.isFinite(earnPct) || earnPct < 0 || earnPct > 100) {
      return setFormError('Taxa de acúmulo deve ficar entre 0 e 100%.')
    }
    if (!Number.isFinite(redeemPct) || redeemPct < 0 || redeemPct > 100) {
      return setFormError('Teto de resgate deve ficar entre 0 e 100%.')
    }
    if (!Number.isInteger(days) || days < 1) {
      return setFormError('Validade deve ser de pelo menos 1 dia.')
    }
    if (!Number.isFinite(campPct) || campPct < 0 || campPct > 100) {
      return setFormError('Taxa de campanha deve ficar entre 0 e 100%.')
    }
    const start = fromLocalInput(campStart)
    const end = fromLocalInput(campEnd)
    if (start && end && new Date(start) > new Date(end)) {
      return setFormError('O início da campanha não pode ser depois do fim.')
    }
    save.mutate(
      {
        active,
        earnRatePct: earnPct,
        redeemMaxPct: redeemPct,
        validityDays: days,
        minEarnSubtotal: Number.isFinite(minEarnV) ? minEarnV : 0,
        minRedeemSubtotal: Number.isFinite(minRedeemV) ? minRedeemV : 0,
        campaignRatePct: campPct,
        campaignStartsAt: start,
        campaignEndsAt: end,
      },
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
        <div className="space-y-4">
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

              {/* Pedidos mínimos */}
              <div className="flex flex-wrap gap-4 border-t border-gray-100 pt-4">
                <label className="flex flex-col text-xs font-semibold text-gray-600">
                  Pedido mín. pra acumular (R$)
                  <input
                    value={minEarn}
                    onChange={(e) => setMinEarn(e.target.value.replace(/[^\d.,]/g, ''))}
                    inputMode="decimal"
                    placeholder="0 = sem mínimo"
                    className="mt-1 h-10 w-40 rounded-md border border-gray-300 px-2 text-sm"
                  />
                </label>
                <label className="flex flex-col text-xs font-semibold text-gray-600">
                  Pedido mín. pra resgatar (R$)
                  <input
                    value={minRedeem}
                    onChange={(e) => setMinRedeem(e.target.value.replace(/[^\d.,]/g, ''))}
                    inputMode="decimal"
                    placeholder="0 = sem mínimo"
                    className="mt-1 h-10 w-40 rounded-md border border-gray-300 px-2 text-sm"
                  />
                </label>
              </div>

              {/* Campanha por período: taxa turbinada entre datas. Taxa 0 = sem
                campanha (usa a taxa de acúmulo normal). */}
              <fieldset className="flex flex-col gap-2 border-t border-gray-100 pt-4">
                <legend className="text-xs font-semibold text-gray-600">
                  Campanha (taxa turbinada por período — deixe a taxa em 0 pra desligar)
                </legend>
                <div className="flex flex-wrap gap-4">
                  <label className="flex flex-col text-xs font-semibold text-gray-600">
                    Taxa da campanha (%)
                    <input
                      value={campRate}
                      onChange={(e) => setCampRate(e.target.value.replace(/[^\d.,]/g, ''))}
                      inputMode="decimal"
                      className="mt-1 h-10 w-32 rounded-md border border-gray-300 px-2 text-sm"
                    />
                  </label>
                  <label className="flex flex-col text-xs font-semibold text-gray-600">
                    Início
                    <input
                      type="datetime-local"
                      value={campStart}
                      onChange={(e) => setCampStart(e.target.value)}
                      className="mt-1 h-10 rounded-md border border-gray-300 px-2 text-sm"
                    />
                  </label>
                  <label className="flex flex-col text-xs font-semibold text-gray-600">
                    Fim
                    <input
                      type="datetime-local"
                      value={campEnd}
                      onChange={(e) => setCampEnd(e.target.value)}
                      className="mt-1 h-10 rounded-md border border-gray-300 px-2 text-sm"
                    />
                  </label>
                </div>
              </fieldset>

              <p className="flex items-center gap-1.5 rounded-md bg-amber-50 p-2 text-xs text-amber-800">
                <Coins className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
                Cashback é dívida da loja com o cliente. Mudanças valem para acúmulos futuros; o
                saldo já concedido continua com a validade original.
              </p>

              <div className="flex items-center gap-3">
                <button
                  type="submit"
                  disabled={save.isPending}
                  className="inline-flex h-10 items-center gap-1.5 rounded-md bg-brand-orange px-4 text-sm font-semibold text-gray-900 hover:bg-brand-orange-dark disabled:opacity-50"
                >
                  {save.isPending && (
                    <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
                  )}
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

          <CategoryRatesSection baseRate={data?.earnRatePct ?? 0} />
        </div>
      )}
    </AdminShell>
  )
}

// Taxa de cashback POR CATEGORIA (override da taxa base). Categoria em branco usa
// a taxa base. Salva tudo de uma vez, aplicando só as linhas que mudaram.
function CategoryRatesSection({ baseRate }: { baseRate: number }) {
  const { data: categories = [], isLoading } = useAdminCategories()
  const { data: rates = {} } = useCashbackCategoryRates()
  const setRate = useSetCategoryRate()
  const delRate = useDeleteCategoryRate()

  const [edits, setEdits] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  // Hidrata os inputs a partir dos overrides atuais. `rates` é estável entre
  // renders (react-query só troca a referência quando o dado muda).
  useEffect(() => {
    const init: Record<string, string> = {}
    for (const [cat, r] of Object.entries(rates)) init[cat] = String(r)
    setEdits(init)
  }, [rates])

  async function saveAll() {
    setSaving(true)
    setSaved(false)
    try {
      for (const cat of categories) {
        const raw = (edits[cat.id] ?? '').trim().replace(',', '.')
        const had = Object.prototype.hasOwnProperty.call(rates, cat.id)
        if (raw === '') {
          if (had) await delRate.mutateAsync(cat.id) // limpou → volta pra base
          continue
        }
        const v = Number(raw)
        if (!Number.isFinite(v) || v < 0 || v > 100) continue
        if (!had || rates[cat.id] !== v) await setRate.mutateAsync({ id: cat.id, ratePct: v })
      }
      setSaved(true)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Section title="Taxa por categoria (opcional)">
      <div className="p-3 sm:p-4">
        <p className="mb-3 text-xs text-gray-500">
          Deixe em branco para a categoria usar a taxa base ({baseRate}%). Preencha para dar uma
          taxa diferente (ex.: ferramentas 3%, tintas 8%).
        </p>
        {isLoading ? (
          <LoadingRows rows={4} />
        ) : categories.length === 0 ? (
          <p className="text-sm text-gray-400">Nenhuma categoria cadastrada.</p>
        ) : (
          <div className="flex flex-col gap-2">
            {categories.map((cat) => (
              <label key={cat.id} className="flex items-center justify-between gap-3 text-sm">
                <span className="text-gray-700">{cat.name}</span>
                <span className="flex items-center gap-1">
                  <input
                    value={edits[cat.id] ?? ''}
                    onChange={(e) =>
                      setEdits((p) => ({ ...p, [cat.id]: e.target.value.replace(/[^\d.,]/g, '') }))
                    }
                    inputMode="decimal"
                    placeholder={`${baseRate}`}
                    className="h-9 w-20 rounded-md border border-gray-300 px-2 text-right text-sm"
                  />
                  <span className="text-gray-400">%</span>
                </span>
              </label>
            ))}
            <div className="mt-2 flex items-center gap-3">
              <button
                type="button"
                onClick={saveAll}
                disabled={saving}
                className="inline-flex h-10 items-center gap-1.5 rounded-md border border-gray-300 px-4 text-sm font-semibold text-gray-700 hover:bg-gray-50 disabled:opacity-50"
              >
                {saving && <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />}
                Salvar taxas por categoria
              </button>
              {saved && !saving && (
                <span className="text-sm font-medium text-green-600" role="status">
                  Salvo.
                </span>
              )}
            </div>
          </div>
        )}
      </div>
    </Section>
  )
}
