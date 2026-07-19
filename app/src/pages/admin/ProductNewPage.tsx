import { useCallback } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import { ProductForm } from '@/components/admin/products/ProductForm'
import { useCreateProduct } from '@/hooks/useAdminProducts'
import type { ProductInput } from '@/lib/adminProductTypes'

/**
 * Cadastro de produto novo.
 *
 * **Entra como rascunho, sempre.** O status é forçado em `createAdminProduct` e
 * nem aparece no formulário: produto recém-criado não tem foto, nem ficha
 * técnica conferida, nem estoque validado — publicar direto colocaria um item
 * cru na vitrine. É a mesma decisão da importação de planilha, que também deixa
 * tudo em rascunho e delega a publicação a um segundo ato deliberado.
 *
 * Depois de criar, a navegação vai para a EDIÇÃO do item — é lá que estão as
 * imagens, e um produto sem foto não deveria ser publicado. Levar o dono de
 * volta para a lista esconderia justamente o passo seguinte.
 */
export default function AdminProductNewPage() {
  const navigate = useNavigate()
  const create = useCreateProduct()

  const onSubmit = useCallback(
    (input: ProductInput) => {
      create.mutate(input, {
        onSuccess: (product) => navigate(`/admin/produtos/${product.id}`),
      })
    },
    [create, navigate],
  )

  return (
    <AdminShell
      title="Novo produto"
      description="O item entra como rascunho — invisível na loja até você publicar."
      toolbar={
        <Link
          to="/admin/produtos"
          className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 px-3 py-1.5 text-xs font-semibold text-gray-700 hover:bg-gray-50"
        >
          <ArrowLeft className="h-3.5 w-3.5" aria-hidden="true" />
          Voltar à lista
        </Link>
      }
    >
      <div className="space-y-4">
        <p className="rounded-md border border-gray-200 border-l-4 border-l-brand-blue bg-brand-blue-light/30 p-3 text-xs leading-relaxed text-gray-700">
          Nome, categoria e preço são obrigatórios. Depois de salvar você cai na tela de edição,
          onde envia as fotos — <strong>publique só depois de ter ao menos a capa</strong>.
        </p>

        <ProductForm
          saving={create.isPending}
          error={create.isError ? (create.error as Error).message : null}
          submitLabel="Criar como rascunho"
          onSubmit={onSubmit}
          onCancel={() => navigate('/admin/produtos')}
        />
      </div>
    </AdminShell>
  )
}
