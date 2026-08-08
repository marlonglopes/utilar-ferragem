import { useState } from 'react'
import { Plus, Trash2, Check, Ban, Loader2, Ticket } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import { ErrorState, LoadingRows, Section } from '@/components/admin/primitives'
import { formatCurrency } from '@/lib/format'
import {
  useCoupons,
  useCreateCoupon,
  useUpdateCoupon,
  useDeleteCoupon,
} from '@/hooks/useAdminCoupons'
import type { CouponType } from '@/lib/adminCouponsApi'

/**
 * Gestão de cupons — admin only (dinheiro/marketing). Criar, listar, ativar/
 * desativar e apagar. O valor é sempre resolvido no servidor; aqui só se define
 * a regra. Espelha a tela de frete.
 */
export default function AdminCouponsPage() {
  const { data: coupons = [], isLoading, isError, error, refetch } = useCoupons()
  const create = useCreateCoupon()
  const update = useUpdateCoupon()
  const del = useDeleteCoupon()

  const [code, setCode] = useState('')
  const [type, setType] = useState<CouponType>('percent')
  const [value, setValue] = useState('')
  const [minSubtotal, setMinSubtotal] = useState('')
  const [maxUses, setMaxUses] = useState('')
  const [formError, setFormError] = useState('')

  function submit(e: React.FormEvent) {
    e.preventDefault()
    setFormError('')
    const v = Number(value.replace(',', '.'))
    if (!code.trim()) return setFormError('Informe o código.')
    if (!Number.isFinite(v) || v < 0) return setFormError('Valor inválido.')
    if (type === 'percent' && v > 100) return setFormError('Percentual não pode passar de 100.')
    create.mutate(
      {
        code: code.trim().toUpperCase(),
        type,
        value: v,
        minSubtotal: minSubtotal ? Number(minSubtotal.replace(',', '.')) : 0,
        maxUses: maxUses ? Number(maxUses) : null,
      },
      {
        onSuccess: () => {
          setCode('')
          setValue('')
          setMinSubtotal('')
          setMaxUses('')
        },
        onError: (err) =>
          setFormError(err instanceof Error ? err.message : 'Falha ao criar o cupom.'),
      }
    )
  }

  return (
    <AdminShell title="Cupons" description="Descontos do site (percentual ou valor fixo).">
      <div className="space-y-4">
        <Section title="Novo cupom">
          <form onSubmit={submit} className="flex flex-wrap items-end gap-3 p-3 sm:p-4">
            <label className="flex flex-col text-xs font-semibold text-gray-600">
              Código
              <input
                value={code}
                onChange={(e) => setCode(e.target.value.toUpperCase())}
                placeholder="OBRA10"
                className="mt-1 h-10 w-40 rounded-md border border-gray-300 px-2 font-mono text-sm"
              />
            </label>
            <label className="flex flex-col text-xs font-semibold text-gray-600">
              Tipo
              <select
                value={type}
                onChange={(e) => setType(e.target.value as CouponType)}
                className="mt-1 h-10 rounded-md border border-gray-300 px-2 text-sm"
              >
                <option value="percent">Percentual (%)</option>
                <option value="fixed">Valor fixo (R$)</option>
              </select>
            </label>
            <label className="flex flex-col text-xs font-semibold text-gray-600">
              {type === 'percent' ? 'Percentual' : 'Valor (R$)'}
              <input
                value={value}
                onChange={(e) => setValue(e.target.value.replace(/[^\d.,]/g, ''))}
                inputMode="decimal"
                placeholder={type === 'percent' ? '10' : '20,00'}
                className="mt-1 h-10 w-28 rounded-md border border-gray-300 px-2 text-sm"
              />
            </label>
            <label className="flex flex-col text-xs font-semibold text-gray-600">
              Pedido mínimo (R$)
              <input
                value={minSubtotal}
                onChange={(e) => setMinSubtotal(e.target.value.replace(/[^\d.,]/g, ''))}
                inputMode="decimal"
                placeholder="0,00"
                className="mt-1 h-10 w-28 rounded-md border border-gray-300 px-2 text-sm"
              />
            </label>
            <label className="flex flex-col text-xs font-semibold text-gray-600">
              Limite de usos
              <input
                value={maxUses}
                onChange={(e) => setMaxUses(e.target.value.replace(/\D/g, ''))}
                inputMode="numeric"
                placeholder="ilimitado"
                className="mt-1 h-10 w-28 rounded-md border border-gray-300 px-2 text-sm"
              />
            </label>
            <button
              type="submit"
              disabled={create.isPending}
              className="inline-flex h-10 items-center gap-1.5 rounded-md bg-brand-orange px-4 text-sm font-semibold text-gray-900 hover:bg-brand-orange-dark disabled:opacity-50"
            >
              {create.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
              ) : (
                <Plus className="h-4 w-4" aria-hidden="true" />
              )}
              Criar
            </button>
            {formError && (
              <p className="w-full text-sm font-semibold text-red-600" role="alert">
                {formError}
              </p>
            )}
          </form>
        </Section>

        <Section title="Cupons">
          {isLoading ? (
            <LoadingRows rows={4} />
          ) : isError ? (
            <ErrorState
              message={error instanceof Error ? error.message : 'Falha ao carregar.'}
              onRetry={() => void refetch()}
            />
          ) : coupons.length === 0 ? (
            <p className="flex items-center gap-2 p-4 text-sm text-gray-500">
              <Ticket className="h-4 w-4" aria-hidden="true" /> Nenhum cupom ainda.
            </p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-200 text-left text-xs text-gray-500">
                    <th className="p-2">Código</th>
                    <th className="p-2">Desconto</th>
                    <th className="p-2">Mín.</th>
                    <th className="p-2">Usos</th>
                    <th className="p-2">Status</th>
                    <th className="p-2"></th>
                  </tr>
                </thead>
                <tbody>
                  {coupons.map((c) => (
                    <tr key={c.id} className="border-b border-gray-100">
                      <td className="p-2 font-mono font-semibold">{c.code}</td>
                      <td className="p-2">
                        {c.type === 'percent' ? `${c.value}%` : formatCurrency(c.value)}
                      </td>
                      <td className="p-2 text-gray-500">
                        {c.minSubtotal > 0 ? formatCurrency(c.minSubtotal) : '—'}
                      </td>
                      <td className="p-2 text-gray-500">
                        {c.uses}
                        {c.maxUses != null ? `/${c.maxUses}` : ''}
                      </td>
                      <td className="p-2">
                        <span
                          className={
                            c.active
                              ? 'rounded bg-green-50 px-2 py-0.5 text-xs font-semibold text-green-700'
                              : 'rounded bg-gray-100 px-2 py-0.5 text-xs font-semibold text-gray-500'
                          }
                        >
                          {c.active ? 'ativo' : 'inativo'}
                        </span>
                      </td>
                      <td className="p-2">
                        <div className="flex justify-end gap-1">
                          <button
                            type="button"
                            onClick={() =>
                              update.mutate({ id: c.id, input: { active: !c.active } })
                            }
                            aria-label={c.active ? `Desativar ${c.code}` : `Ativar ${c.code}`}
                            className="rounded p-1.5 text-gray-500 hover:bg-gray-100"
                          >
                            {c.active ? (
                              <Ban className="h-4 w-4" aria-hidden="true" />
                            ) : (
                              <Check className="h-4 w-4" aria-hidden="true" />
                            )}
                          </button>
                          <button
                            type="button"
                            onClick={() => del.mutate(c.id)}
                            aria-label={`Apagar ${c.code}`}
                            className="rounded p-1.5 text-gray-400 hover:bg-gray-100 hover:text-red-600"
                          >
                            <Trash2 className="h-4 w-4" aria-hidden="true" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Section>
      </div>
    </AdminShell>
  )
}
