import { useState } from 'react'
import { AlertTriangle, Pencil, Plus, Trash2 } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import {
  Chip,
  EmptyState,
  ErrorState,
  LoadingRows,
  ScrollArea,
  Section,
  Table,
  Td,
  Th,
} from '@/components/admin/primitives'
import { cn } from '@/lib/cn'
import {
  useCreateRate,
  useDeleteRate,
  useShippingRates,
  useUpdateRate,
} from '@/hooks/useAdminShipping'
import {
  formatCep,
  isShippingAdminEnabled,
  looksLikeSaoPaulo,
  parseCep,
  type ShippingRate,
  type ShippingRateInput,
} from '@/lib/adminShippingApi'

const inputCls =
  'w-full rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue'

function fmtReais(v: number): string {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL' })
}

const EMPTY: ShippingRateInput = {
  zoneName: '',
  cepStart: 0,
  cepEnd: 0,
  serviceCode: 'standard',
  serviceName: 'Entrega padrão',
  baseCost: 0,
  costPerItem: 0,
  deliveryDays: 3,
  freeAbove: 0,
  active: true,
}

/**
 * Frete — CRUD da tabela de faixas por CEP. É o que o cliente PAGA, então só
 * admin. Avisa em vermelho se detectar faixa de SP (o problema do seed: a loja
 * é no RS).
 */
export default function ShippingPage() {
  const { data: rates = [], isLoading, isError, error, refetch } = useShippingRates()
  const createRate = useCreateRate()
  const updateRate = useUpdateRate()
  const deleteRate = useDeleteRate()

  const [editingId, setEditingId] = useState<string | null>(null)
  const [form, setForm] = useState<ShippingRateInput>(EMPTY)
  const [cepStartStr, setCepStartStr] = useState('')
  const [cepEndStr, setCepEndStr] = useState('')
  const [formError, setFormError] = useState('')

  const hasSaoPaulo = rates.some(looksLikeSaoPaulo)

  const startEdit = (r: ShippingRate) => {
    setEditingId(r.id)
    setForm({ ...r })
    setCepStartStr(formatCep(r.cepStart))
    setCepEndStr(formatCep(r.cepEnd))
    setFormError('')
  }
  const reset = () => {
    setEditingId(null)
    setForm(EMPTY)
    setCepStartStr('')
    setCepEndStr('')
    setFormError('')
  }

  const submit = () => {
    const cepStart = parseCep(cepStartStr)
    const cepEnd = parseCep(cepEndStr)
    if (cepStart === null || cepEnd === null) {
      setFormError('CEP inicial e final precisam ter 8 dígitos.')
      return
    }
    if (cepEnd < cepStart) {
      setFormError('O CEP final não pode ser menor que o inicial.')
      return
    }
    if (!form.zoneName.trim() || form.deliveryDays <= 0) {
      setFormError('Zona e prazo (maior que zero) são obrigatórios.')
      return
    }
    const input: ShippingRateInput = { ...form, cepStart, cepEnd }
    const onSuccess = () => reset()
    if (editingId) updateRate.mutate({ id: editingId, input }, { onSuccess })
    else createRate.mutate(input, { onSuccess })
  }

  const remove = (r: ShippingRate) => {
    if (!window.confirm(`Excluir a faixa "${r.zoneName}" (${r.serviceName})?`)) return
    deleteRate.mutate(r.id)
  }

  const saving = createRate.isPending || updateRate.isPending
  const set = <K extends keyof ShippingRateInput>(k: K, v: ShippingRateInput[K]) =>
    setForm((f) => ({ ...f, [k]: v }))

  return (
    <AdminShell
      title="Frete"
      description="Faixas de frete por CEP — é o que o cliente paga no checkout."
    >
      <div className="space-y-4">
        {!isShippingAdminEnabled && (
          <p className="rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-amber-50/60 p-3 text-xs leading-relaxed text-gray-700">
            <strong>Modo demonstração.</strong> O order-service não está configurado (
            <code className="font-mono">VITE_ORDER_URL</code> vazio): nada é gravado.
          </p>
        )}

        {hasSaoPaulo && (
          <div className="flex items-start gap-2 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800">
            <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0" aria-hidden="true" />
            <div>
              <strong>Atenção: faixas de CEP de São Paulo.</strong> Há faixa na área 01000–05999
              (SP), mas a loja é no <strong>Rio Grande do Sul</strong> (90000–99999). É o valor que
              o cliente paga — corrija as faixas para o CEP real da loja.
            </div>
          </div>
        )}

        <Section title={editingId ? 'Editar faixa' : 'Nova faixa'}>
          <div className="grid gap-3 p-3 sm:grid-cols-2 lg:grid-cols-4 sm:p-4">
            <Field label="Zona">
              <input
                value={form.zoneName}
                onChange={(e) => set('zoneName', e.target.value)}
                placeholder="ex.: Fronteira Oeste RS"
                className={inputCls}
              />
            </Field>
            <Field label="Serviço">
              <select
                value={form.serviceCode}
                onChange={(e) => {
                  const code = e.target.value
                  set('serviceCode', code)
                  set('serviceName', code === 'express' ? 'Entrega expressa' : 'Entrega padrão')
                }}
                className={inputCls}
              >
                <option value="standard">Padrão</option>
                <option value="express">Expressa</option>
              </select>
            </Field>
            <Field label="CEP inicial">
              <input
                value={cepStartStr}
                onChange={(e) => setCepStartStr(e.target.value)}
                placeholder="90000-000"
                className={inputCls}
              />
            </Field>
            <Field label="CEP final">
              <input
                value={cepEndStr}
                onChange={(e) => setCepEndStr(e.target.value)}
                placeholder="99999-999"
                className={inputCls}
              />
            </Field>
            <Field label="Custo base (R$)">
              <input
                type="number"
                step="0.01"
                value={form.baseCost}
                onChange={(e) => set('baseCost', Number(e.target.value))}
                className={inputCls}
              />
            </Field>
            <Field label="Por item (R$)">
              <input
                type="number"
                step="0.01"
                value={form.costPerItem}
                onChange={(e) => set('costPerItem', Number(e.target.value))}
                className={inputCls}
              />
            </Field>
            <Field label="Prazo (dias úteis)">
              <input
                type="number"
                value={form.deliveryDays}
                onChange={(e) => set('deliveryDays', Number(e.target.value))}
                className={inputCls}
              />
            </Field>
            <Field label="Grátis acima de (R$)">
              <input
                type="number"
                step="0.01"
                value={form.freeAbove}
                onChange={(e) => set('freeAbove', Number(e.target.value))}
                className={inputCls}
              />
            </Field>
            <label className="flex items-center gap-2 text-sm text-gray-700">
              <input
                type="checkbox"
                checked={form.active}
                onChange={(e) => set('active', e.target.checked)}
                className="h-4 w-4 rounded border-gray-300 text-brand-orange focus:ring-brand-orange"
              />
              Ativa
            </label>
            <div className="flex items-end gap-2 sm:col-span-2 lg:col-span-4">
              <button
                type="button"
                onClick={submit}
                disabled={saving}
                className="inline-flex items-center gap-1.5 rounded-md bg-brand-orange px-3 py-1.5 text-xs font-semibold text-gray-900 hover:bg-brand-orange-dark disabled:opacity-50"
              >
                <Plus className="h-3.5 w-3.5" aria-hidden="true" />
                {editingId ? 'Salvar alterações' : 'Adicionar faixa'}
              </button>
              {editingId && (
                <button
                  type="button"
                  onClick={reset}
                  className="rounded-md border border-gray-300 px-3 py-1.5 text-xs font-semibold text-gray-600 hover:bg-gray-50"
                >
                  Cancelar
                </button>
              )}
              {formError && <span className="text-xs text-red-700">{formError}</span>}
              {(createRate.isError || updateRate.isError) && (
                <span className="text-xs text-red-700">Falha ao salvar. Confira os campos.</span>
              )}
            </div>
          </div>
        </Section>

        <Section title="Faixas" description={`${rates.length} faixa(s)`}>
          {isError ? (
            <div className="p-4">
              <ErrorState
                message={error instanceof Error ? error.message : 'Falha ao carregar'}
                onRetry={() => void refetch()}
              />
            </div>
          ) : isLoading ? (
            <LoadingRows rows={5} />
          ) : rates.length === 0 ? (
            <div className="p-4">
              <EmptyState title="Nenhuma faixa" description="Adicione a primeira acima." />
            </div>
          ) : (
            <ScrollArea>
              <Table>
                <thead>
                  <tr>
                    <Th>Zona</Th>
                    <Th>Faixa de CEP</Th>
                    <Th>Serviço</Th>
                    <Th numeric>Base</Th>
                    <Th numeric>Por item</Th>
                    <Th numeric>Prazo</Th>
                    <Th>Grátis acima</Th>
                    <Th className="text-right">Ações</Th>
                  </tr>
                </thead>
                <tbody>
                  {rates.map((r) => (
                    <tr
                      key={r.id}
                      className={cn('hover:bg-gray-50', looksLikeSaoPaulo(r) && 'bg-red-50/40')}
                    >
                      <Td className="text-gray-800">
                        {r.zoneName}
                        {!r.active && (
                          <Chip className="ml-2 bg-gray-100 text-gray-500 ring-1 ring-inset ring-gray-400/20">
                            inativa
                          </Chip>
                        )}
                      </Td>
                      <Td className="font-mono text-xs text-gray-600">
                        {formatCep(r.cepStart)} – {formatCep(r.cepEnd)}
                      </Td>
                      <Td className="text-xs text-gray-700">{r.serviceName}</Td>
                      <Td numeric className="tabular-nums">
                        {fmtReais(r.baseCost)}
                      </Td>
                      <Td numeric className="tabular-nums text-gray-500">
                        {fmtReais(r.costPerItem)}
                      </Td>
                      <Td numeric className="tabular-nums text-gray-600">
                        {r.deliveryDays}d
                      </Td>
                      <Td className="text-xs text-gray-500">
                        {r.freeAbove > 0 ? fmtReais(r.freeAbove) : '—'}
                      </Td>
                      <Td className="text-right">
                        <div className="inline-flex gap-1.5">
                          <button
                            type="button"
                            onClick={() => startEdit(r)}
                            className="inline-flex items-center gap-1 rounded-md border border-gray-300 px-2 py-1 text-xs font-semibold text-gray-600 hover:bg-gray-50"
                          >
                            <Pencil className="h-3 w-3" aria-hidden="true" /> Editar
                          </button>
                          <button
                            type="button"
                            onClick={() => remove(r)}
                            disabled={deleteRate.isPending}
                            className="inline-flex items-center gap-1 rounded-md border border-red-200 px-2 py-1 text-xs font-semibold text-red-700 hover:bg-red-50 disabled:opacity-50"
                          >
                            <Trash2 className="h-3 w-3" aria-hidden="true" /> Excluir
                          </button>
                        </div>
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </ScrollArea>
          )}
        </Section>
      </div>
    </AdminShell>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-xs font-semibold text-gray-700">{label}</label>
      <div className="mt-1">{children}</div>
    </div>
  )
}
