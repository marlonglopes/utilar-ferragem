import { useCallback, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, Check } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import { ErrorState, LoadingRows } from '@/components/admin/primitives'
import { ProductForm } from '@/components/admin/products/ProductForm'
import { ProductImageManager } from '@/components/admin/products/ProductImageManager'
import { StatusPill } from '@/components/admin/products/productPrimitives'
import { useAdminProduct, useSaveProduct } from '@/hooks/useAdminProducts'
import type { ProductInput } from '@/lib/adminProductTypes'

/**
 * Edição de um produto.
 *
 * A tela junta as duas coisas que o dono não conseguia fazer sem terminal:
 * corrigir os dados do item (com a margem calculada ao vivo) e gerenciar as
 * fotos (o pipeline de upload que existia no backend sem nenhuma interface).
 *
 * O formulário é **remontado por `key` quando o produto carrega**. Sem isso,
 * o `useState` inicial do formulário guardaria os campos vazios do primeiro
 * render e o dono veria um formulário em branco sobre um produto existente —
 * e salvaria por cima.
 */
export default function AdminProductEditPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data, isLoading, isError, error, refetch } = useAdminProduct(id)
  const save = useSaveProduct(id ?? '')
  const [saved, setSaved] = useState(false)

  const onSubmit = useCallback(
    (input: ProductInput) => {
      setSaved(false)
      save.mutate(input, {
        // A confirmação só aparece depois que o SERVIDOR respondeu, e o
        // formulário é remontado com o que ele devolveu (a mutação substitui o
        // cache). Se ele recusou ou normalizou um campo, a tela mostra o valor
        // real — nunca o que foi digitado.
        onSuccess: () => setSaved(true),
      })
    },
    [save],
  )

  return (
    <AdminShell
      title={data?.name ?? 'Editar produto'}
      description={data?.sku ? `SKU ${data.sku}` : 'Carregando…'}
      toolbar={
        <div className="flex flex-wrap items-center gap-2">
          {data && <StatusPill status={data.status} />}
          <Link
            to="/admin/produtos"
            className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 px-3 py-1.5 text-xs font-semibold text-gray-700 hover:bg-gray-50"
          >
            <ArrowLeft className="h-3.5 w-3.5" aria-hidden="true" />
            Voltar à lista
          </Link>
        </div>
      }
    >
      {isLoading ? (
        <LoadingRows rows={6} />
      ) : isError || !data ? (
        <ErrorState
          message={error instanceof Error ? error.message : 'Produto não encontrado.'}
          onRetry={() => {
            void refetch()
          }}
        />
      ) : (
        <div className="space-y-4">
          {saved && !save.isPending && (
            <p
              role="status"
              className="flex items-center gap-2 rounded-md border border-green-200 border-l-4 border-l-green-600 bg-green-50 p-3 text-xs font-semibold text-green-900"
            >
              <Check className="h-4 w-4 shrink-0" aria-hidden="true" />
              Alterações salvas.
            </p>
          )}

          <ProductForm
            key={data.id + (data.updatedAt ?? '')}
            product={data}
            saving={save.isPending}
            error={save.isError ? (save.error as Error).message : null}
            submitLabel="Salvar alterações"
            onSubmit={onSubmit}
            onCancel={() => navigate('/admin/produtos')}
          />

          <ProductImageManager productId={data.id} />
        </div>
      )}
    </AdminShell>
  )
}
