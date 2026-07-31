import { useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
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
  useAdminCategories,
  useCreateCategory,
  useDeleteCategory,
  useUpdateCategory,
} from '@/hooks/useAdminCategories'
import {
  isCategoriesAdminEnabled,
  isValidCategoryId,
  type Category,
} from '@/lib/adminCategoriesApi'

const inputCls =
  'w-full rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue'

/**
 * Gestão de categorias. Antes só dava para criar por seed/migration — agora o
 * dono cria, renomeia e exclui pela tela. O `id` é o slug (imutável, FK dos
 * produtos); excluir é bloqueado se houver produto na categoria.
 */
export default function CategoriesPage() {
  const { data: cats = [], isLoading, isError, error, refetch } = useAdminCategories()
  const createCat = useCreateCategory()
  const updateCat = useUpdateCategory()
  const deleteCat = useDeleteCategory()

  // criar
  const [newId, setNewId] = useState('')
  const [newName, setNewName] = useState('')
  const [newIcon, setNewIcon] = useState('')
  const idOk = newId === '' || isValidCategoryId(newId.toLowerCase())

  const create = () => {
    if (!newName.trim() || !isValidCategoryId(newId.toLowerCase())) return
    createCat.mutate(
      { id: newId.toLowerCase(), name: newName.trim(), icon: newIcon.trim() || undefined },
      {
        onSuccess: () => {
          setNewId('')
          setNewName('')
          setNewIcon('')
        },
      }
    )
  }

  // editar
  const [editing, setEditing] = useState<string | null>(null)
  const [draft, setDraft] = useState<{ name: string; icon: string }>({ name: '', icon: '' })
  const startEdit = (c: Category) => {
    setEditing(c.id)
    setDraft({ name: c.name, icon: c.icon })
  }
  const saveEdit = (id: string) => {
    updateCat.mutate(
      { id, input: { name: draft.name.trim() || undefined, icon: draft.icon.trim() || undefined } },
      { onSuccess: () => setEditing(null) }
    )
  }

  const remove = (id: string) => {
    if (!window.confirm(`Excluir a categoria "${id}"? Só é possível se não houver produtos nela.`))
      return
    deleteCat.mutate(id)
  }

  return (
    <AdminShell
      title="Categorias"
      description="Organize o catálogo: criar, renomear e excluir categorias."
    >
      <div className="space-y-4">
        {!isCategoriesAdminEnabled && (
          <p className="rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-amber-50/60 p-3 text-xs leading-relaxed text-gray-700">
            <strong>Modo demonstração.</strong> O catálogo não está configurado (
            <code className="font-mono">VITE_CATALOG_URL</code> vazio): nada é gravado.
          </p>
        )}

        <Section
          title="Nova categoria"
          description="O identificador (slug) é fixo depois de criado — é a chave dos produtos."
        >
          <div className="grid gap-3 p-3 sm:grid-cols-4 sm:p-4">
            <div>
              <label htmlFor="nc-id" className="block text-xs font-semibold text-gray-700">
                Identificador (slug)
              </label>
              <input
                id="nc-id"
                value={newId}
                onChange={(e) => setNewId(e.target.value)}
                placeholder="ex.: fechaduras"
                className={cn(inputCls, 'mt-1', !idOk && 'border-red-400')}
              />
              {!idOk && <p className="mt-1 text-xs text-red-600">só minúsculas, números e hífen</p>}
            </div>
            <div>
              <label htmlFor="nc-name" className="block text-xs font-semibold text-gray-700">
                Nome
              </label>
              <input
                id="nc-name"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                placeholder="ex.: Fechaduras"
                className={cn(inputCls, 'mt-1')}
              />
            </div>
            <div>
              <label htmlFor="nc-icon" className="block text-xs font-semibold text-gray-700">
                Ícone (opcional)
              </label>
              <input
                id="nc-icon"
                value={newIcon}
                onChange={(e) => setNewIcon(e.target.value)}
                placeholder="▣"
                className={cn(inputCls, 'mt-1')}
              />
            </div>
            <div className="flex items-end">
              <button
                type="button"
                onClick={create}
                disabled={
                  !newName.trim() || !isValidCategoryId(newId.toLowerCase()) || createCat.isPending
                }
                className="inline-flex items-center gap-1.5 rounded-md bg-brand-orange px-3 py-1.5 text-xs font-semibold text-white hover:bg-brand-orange/90 disabled:opacity-50"
              >
                <Plus className="h-3.5 w-3.5" aria-hidden="true" />
                Criar
              </button>
            </div>
            {createCat.isError && (
              <p className="text-xs text-red-700 sm:col-span-4">
                {createCat.error instanceof Error ? createCat.error.message : 'Falha ao criar'}
              </p>
            )}
          </div>
        </Section>

        <Section title="Categorias" description={`${cats.length} categoria(s)`}>
          {isError ? (
            <div className="p-4">
              <ErrorState
                message={error instanceof Error ? error.message : 'Falha ao carregar'}
                onRetry={() => void refetch()}
              />
            </div>
          ) : isLoading ? (
            <LoadingRows rows={6} />
          ) : cats.length === 0 ? (
            <div className="p-4">
              <EmptyState title="Nenhuma categoria" description="Crie a primeira acima." />
            </div>
          ) : (
            <ScrollArea>
              <Table>
                <thead>
                  <tr>
                    <Th>Ícone</Th>
                    <Th>Identificador</Th>
                    <Th>Nome</Th>
                    <Th numeric>Ordem</Th>
                    <Th className="text-right">Ações</Th>
                  </tr>
                </thead>
                <tbody>
                  {cats.map((c) => {
                    const isEditing = editing === c.id
                    return (
                      <tr key={c.id} className="hover:bg-gray-50">
                        <Td className="text-lg">
                          {isEditing ? (
                            <input
                              value={draft.icon}
                              onChange={(e) => setDraft((d) => ({ ...d, icon: e.target.value }))}
                              className={cn(inputCls, 'w-14 py-1 text-center')}
                            />
                          ) : (
                            c.icon
                          )}
                        </Td>
                        <Td className="font-mono text-xs text-gray-500">{c.id}</Td>
                        <Td>
                          {isEditing ? (
                            <input
                              value={draft.name}
                              onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))}
                              className={cn(inputCls, 'py-1')}
                            />
                          ) : (
                            <span className="text-gray-800">{c.name}</span>
                          )}
                        </Td>
                        <Td numeric className="tabular-nums text-gray-600">
                          {c.sortOrder}
                        </Td>
                        <Td className="text-right">
                          <div className="inline-flex items-center gap-1.5">
                            {isEditing ? (
                              <>
                                <button
                                  type="button"
                                  onClick={() => saveEdit(c.id)}
                                  disabled={updateCat.isPending}
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
                              </>
                            ) : (
                              <>
                                <button
                                  type="button"
                                  onClick={() => startEdit(c)}
                                  className="rounded-md border border-gray-300 px-2 py-1 text-xs font-semibold text-gray-600 hover:bg-gray-50"
                                >
                                  Editar
                                </button>
                                <button
                                  type="button"
                                  onClick={() => remove(c.id)}
                                  disabled={deleteCat.isPending}
                                  className="inline-flex items-center gap-1 rounded-md border border-red-200 px-2 py-1 text-xs font-semibold text-red-700 hover:bg-red-50 disabled:opacity-50"
                                >
                                  <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                                  Excluir
                                </button>
                              </>
                            )}
                          </div>
                        </Td>
                      </tr>
                    )
                  })}
                </tbody>
              </Table>
            </ScrollArea>
          )}
          {deleteCat.isError && (
            <p className="border-t border-gray-200 px-4 py-2 text-xs text-red-700">
              {deleteCat.error instanceof Error ? deleteCat.error.message : 'Falha ao excluir'}
            </p>
          )}
        </Section>
      </div>
    </AdminShell>
  )
}
