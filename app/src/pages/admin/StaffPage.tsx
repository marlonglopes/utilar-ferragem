import { useState } from 'react'
import { Search, UserPlus } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import {
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
  useCreateOperator,
  useOperators,
  useStores,
  useUpdateOperator,
  useUsersSearch,
} from '@/hooks/useAdminStaff'
import {
  isStaffAdminEnabled,
  LEVEL_LABEL,
  type Operator,
  type StoreLevel,
} from '@/lib/adminStaffApi'

const LEVELS: StoreLevel[] = ['operator', 'supervisor', 'manager']
const inputCls =
  'w-full rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue'

/**
 * Gestão de staff: promover um usuário a operador de loja e editar cargo/teto.
 *
 * PORQUÊ existe: o backend (/admin/operators, /admin/stores) já existia, mas
 * sem tela ninguém era promovido a não ser por curl. É a ponte pras personas —
 * é aqui que o contador/vendas/almoxarifado ganham acesso e o teto de desconto.
 */
export default function StaffPage() {
  const { data: operators = [], isLoading, isError, error, refetch } = useOperators()
  const { data: stores = [] } = useStores()
  const createOp = useCreateOperator()
  const updateOp = useUpdateOperator()

  // ---- Promover ----
  const [q, setQ] = useState('')
  const { data: users = [], isFetching: searching } = useUsersSearch(q)
  const [pick, setPick] = useState<{ id: string; name: string } | null>(null)
  const [storeId, setStoreId] = useState('')
  const [level, setLevel] = useState<StoreLevel>('operator')
  const [ceiling, setCeiling] = useState('')

  const promote = () => {
    if (!pick || !storeId) return
    createOp.mutate(
      {
        userId: pick.id,
        storeId,
        level,
        discountCeilingPct: ceiling.trim() ? Number(ceiling) : undefined,
      },
      {
        onSuccess: () => {
          setPick(null)
          setQ('')
          setCeiling('')
        },
      }
    )
  }

  // ---- Editar ----
  const [editing, setEditing] = useState<string | null>(null)
  const [draft, setDraft] = useState<{ level: StoreLevel; ceiling: string; active: boolean }>({
    level: 'operator',
    ceiling: '',
    active: true,
  })
  const startEdit = (op: Operator) => {
    setEditing(op.userId)
    setDraft({ level: op.level, ceiling: String(op.discountCeilingPct), active: op.active })
  }
  const saveEdit = (userId: string) => {
    updateOp.mutate(
      {
        userId,
        input: {
          level: draft.level,
          discountCeilingPct: draft.ceiling.trim() ? Number(draft.ceiling) : undefined,
          active: draft.active,
        },
      },
      { onSuccess: () => setEditing(null) }
    )
  }

  return (
    <AdminShell
      title="Operadores"
      description="Quem opera a loja: promover um usuário, definir cargo e teto de desconto."
    >
      <div className="space-y-4">
        {!isStaffAdminEnabled && (
          <p className="rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-amber-50/60 p-3 text-xs leading-relaxed text-gray-700">
            <strong>Modo demonstração.</strong> O auth-service não está configurado (
            <code className="font-mono">VITE_AUTH_URL</code> vazio): os dados são inventados e nada
            é gravado.
          </p>
        )}

        {/* ------------------------------------------------ Promover */}
        <Section
          title="Promover a operador"
          description="Busque um usuário já cadastrado e dê o acesso de operação."
        >
          <div className="space-y-3 p-3 sm:p-4">
            <div className="relative">
              <Search
                className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
                aria-hidden="true"
              />
              <input
                type="search"
                value={q}
                onChange={(e) => {
                  setQ(e.target.value)
                  setPick(null)
                }}
                placeholder="Buscar por nome ou e-mail (2+ letras)"
                className={cn(inputCls, 'pl-8')}
              />
            </div>

            {q.trim().length >= 2 && !pick && (
              <div className="max-h-44 overflow-auto rounded-md border border-gray-200">
                {searching ? (
                  <p className="p-2 text-xs text-gray-500">Buscando…</p>
                ) : users.length === 0 ? (
                  <p className="p-2 text-xs text-gray-500">Nenhum usuário encontrado.</p>
                ) : (
                  users.map((u) => (
                    <button
                      key={u.id}
                      type="button"
                      onClick={() => setPick({ id: u.id, name: u.name })}
                      className="flex w-full items-center justify-between gap-2 border-b border-gray-100 px-3 py-2 text-left text-sm last:border-0 hover:bg-gray-50"
                    >
                      <span className="truncate text-gray-800">{u.name}</span>
                      <span className="truncate text-xs text-gray-500">{u.email}</span>
                    </button>
                  ))
                )}
              </div>
            )}

            {pick && (
              <div className="grid gap-3 rounded-md border border-brand-blue/30 bg-blue-50/40 p-3 sm:grid-cols-4">
                <div className="sm:col-span-4">
                  <span className="text-xs text-gray-500">Promovendo</span>{' '}
                  <strong className="text-sm text-gray-900">{pick.name}</strong>
                </div>
                <div>
                  <label className="block text-xs font-semibold text-gray-700">Loja</label>
                  <select
                    value={storeId}
                    onChange={(e) => setStoreId(e.target.value)}
                    className={cn(inputCls, 'mt-1')}
                  >
                    <option value="">Selecione…</option>
                    {stores.map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.code} — {s.name}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-semibold text-gray-700">Cargo</label>
                  <select
                    value={level}
                    onChange={(e) => setLevel(e.target.value as StoreLevel)}
                    className={cn(inputCls, 'mt-1')}
                  >
                    {LEVELS.map((l) => (
                      <option key={l} value={l}>
                        {LEVEL_LABEL[l]}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-semibold text-gray-700">
                    Teto desc. (%)
                  </label>
                  <input
                    type="number"
                    min={0}
                    max={100}
                    value={ceiling}
                    onChange={(e) => setCeiling(e.target.value)}
                    placeholder="cargo"
                    className={cn(inputCls, 'mt-1')}
                  />
                </div>
                <div className="flex items-end gap-2">
                  <button
                    type="button"
                    onClick={promote}
                    disabled={!storeId || createOp.isPending}
                    className="inline-flex items-center gap-1.5 rounded-md bg-brand-orange px-3 py-1.5 text-xs font-semibold text-gray-900 hover:bg-brand-orange/90 disabled:opacity-50"
                  >
                    <UserPlus className="h-3.5 w-3.5" aria-hidden="true" />
                    Promover
                  </button>
                  <button
                    type="button"
                    onClick={() => setPick(null)}
                    className="rounded-md border border-gray-300 px-2.5 py-1.5 text-xs font-semibold text-gray-600 hover:bg-gray-50"
                  >
                    Cancelar
                  </button>
                </div>
                {createOp.isError && (
                  <p className="text-xs text-red-700 sm:col-span-4">
                    {createOp.error instanceof Error ? createOp.error.message : 'Falha ao promover'}
                  </p>
                )}
              </div>
            )}
          </div>
        </Section>

        {/* ------------------------------------------------ Lista */}
        <Section title="Operadores" description={`${operators.length} operador(es)`}>
          {isError ? (
            <div className="p-4">
              <ErrorState
                message={error instanceof Error ? error.message : 'Falha ao carregar'}
                onRetry={() => void refetch()}
              />
            </div>
          ) : isLoading ? (
            <LoadingRows rows={5} />
          ) : operators.length === 0 ? (
            <div className="p-4">
              <EmptyState title="Nenhum operador" description="Promova um usuário acima." />
            </div>
          ) : (
            <ScrollArea>
              <Table>
                <thead>
                  <tr>
                    <Th>Operador</Th>
                    <Th>Loja</Th>
                    <Th>Cargo</Th>
                    <Th numeric>Teto %</Th>
                    <Th>Ativo</Th>
                    <Th className="text-right">Ações</Th>
                  </tr>
                </thead>
                <tbody>
                  {operators.map((op) => {
                    const isEditing = editing === op.userId
                    return (
                      <tr key={op.userId} className="hover:bg-gray-50">
                        <Td>
                          <div className="text-gray-800">{op.name}</div>
                          <div className="text-xs text-gray-500">{op.email}</div>
                        </Td>
                        <Td className="text-xs text-gray-600">{op.storeCode}</Td>
                        <Td>
                          {isEditing ? (
                            <select
                              value={draft.level}
                              onChange={(e) =>
                                setDraft((d) => ({ ...d, level: e.target.value as StoreLevel }))
                              }
                              className={cn(inputCls, 'py-1')}
                            >
                              {LEVELS.map((l) => (
                                <option key={l} value={l}>
                                  {LEVEL_LABEL[l]}
                                </option>
                              ))}
                            </select>
                          ) : (
                            <span className="text-gray-700">{LEVEL_LABEL[op.level]}</span>
                          )}
                        </Td>
                        <Td numeric>
                          {isEditing ? (
                            <input
                              type="number"
                              min={0}
                              max={100}
                              value={draft.ceiling}
                              onChange={(e) => setDraft((d) => ({ ...d, ceiling: e.target.value }))}
                              className={cn(inputCls, 'w-20 py-1 text-right')}
                            />
                          ) : (
                            <span className="tabular-nums text-gray-700">
                              {op.discountCeilingPct}
                            </span>
                          )}
                        </Td>
                        <Td>
                          {isEditing ? (
                            <input
                              type="checkbox"
                              checked={draft.active}
                              onChange={(e) =>
                                setDraft((d) => ({ ...d, active: e.target.checked }))
                              }
                            />
                          ) : (
                            <span
                              className={cn(
                                'inline-flex rounded-full px-2 py-0.5 text-xs font-semibold',
                                op.active
                                  ? 'bg-green-100 text-green-800'
                                  : 'bg-gray-200 text-gray-600'
                              )}
                            >
                              {op.active ? 'Ativo' : 'Inativo'}
                            </span>
                          )}
                        </Td>
                        <Td className="text-right">
                          {isEditing ? (
                            <div className="inline-flex gap-1.5">
                              <button
                                type="button"
                                onClick={() => saveEdit(op.userId)}
                                disabled={updateOp.isPending}
                                className="rounded-md bg-brand-blue px-2 py-1 text-xs font-semibold text-white hover:bg-brand-blue/90 disabled:opacity-50"
                              >
                                Salvar
                              </button>
                              <button
                                type="button"
                                onClick={() => setEditing(null)}
                                className="rounded-md border border-gray-300 px-2 py-1 text-xs font-semibold text-gray-600 hover:bg-gray-50"
                              >
                                Cancelar
                              </button>
                            </div>
                          ) : (
                            <button
                              type="button"
                              onClick={() => startEdit(op)}
                              className="rounded-md border border-gray-300 px-2 py-1 text-xs font-semibold text-gray-600 hover:bg-gray-50"
                            >
                              Editar
                            </button>
                          )}
                        </Td>
                      </tr>
                    )
                  })}
                </tbody>
              </Table>
            </ScrollArea>
          )}
        </Section>
      </div>
    </AdminShell>
  )
}
