import { useCallback, useMemo, useState } from 'react'
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query'
import {
  bulkSetStatus,
  createAdminProduct,
  deleteProductImage,
  fetchAdminProduct,
  fetchAdminProducts,
  fetchProductImages,
  patchAdminProduct,
  reorderProductImages,
  setProductImageCover,
  uploadProductImages,
} from '@/lib/adminProductsApi'
import { imageOrderPayload, moveImage, promoteCover, sortImages } from '@/lib/adminProductFormat'
import type {
  AdminProductDetail,
  AdminProductPage,
  AdminProductQuery,
  BulkStatusResult,
  ImageRejection,
  ProductImageRecord,
  ProductInput,
  ProductStatus,
} from '@/lib/adminProductTypes'

/**
 * Queries e mutações da gestão de produtos.
 *
 * Duas políticas específicas daqui:
 *
 * - **`gcTime` curto (2 min)**, como no resto do painel: estas respostas
 *   carregam `cost`, e o cache do TanStack é memória da aba. Sair da tela e o
 *   custo evaporar da memória é o comportamento desejado, não um efeito
 *   colateral.
 * - **Sem `refetchOnWindowFocus`.** O dono conferindo margem produto a produto
 *   não pode ver a tabela se rearranjar sozinha porque ele foi olhar o
 *   WhatsApp.
 */

const PRODUCT_GC_TIME = 2 * 60 * 1000

const base = { gcTime: PRODUCT_GC_TIME, retry: 1, refetchOnWindowFocus: false } as const

export const productKeys = {
  all: ['admin', 'products'] as const,
  list: (q: AdminProductQuery) =>
    [
      'admin',
      'products',
      'list',
      q.q ?? '',
      q.category ?? '',
      q.status ?? '',
      q.sort,
      q.dir,
      q.page,
      q.pageSize,
    ] as const,
  detail: (id: string) => ['admin', 'products', 'detail', id] as const,
  images: (id: string) => ['admin', 'products', 'images', id] as const,
}

export function useAdminProductList(q: AdminProductQuery): UseQueryResult<AdminProductPage> {
  return useQuery({
    queryKey: productKeys.list(q),
    queryFn: () => fetchAdminProducts(q),
    // Mantém a página anterior visível enquanto a nova carrega: sem isso, cada
    // tecla digitada na busca pisca a tabela inteira para o esqueleto.
    placeholderData: (prev) => prev,
    ...base,
  })
}

export function useAdminProduct(id: string | undefined): UseQueryResult<AdminProductDetail> {
  return useQuery({
    queryKey: productKeys.detail(id ?? ''),
    queryFn: () => fetchAdminProduct(id as string),
    enabled: Boolean(id),
    ...base,
  })
}

/**
 * Salva a edição.
 *
 * **Sem atualização otimista no formulário**, e é uma decisão, não uma
 * omissão: o servidor valida preço, unidade, código de barras e faixas, e pode
 * NORMALIZAR o que foi enviado (unidade vira minúscula, barcode perde
 * pontuação). Pintar o valor digitado como se fosse o salvo deixaria a tela
 * mostrando "UN" enquanto o banco guardou "un" — e, no caso de recusa,
 * mostrando um preço que não existe em lugar nenhum.
 *
 * O que a mutação faz é substituir o cache pelo objeto que o SERVIDOR
 * devolveu. É a única fonte que sabe o que ficou gravado.
 */
export function useSaveProduct(
  id: string,
): UseMutationResult<AdminProductDetail, Error, ProductInput> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: ProductInput) => patchAdminProduct(id, input),
    onSuccess: (saved) => {
      qc.setQueryData(productKeys.detail(id), saved)
      void qc.invalidateQueries({ queryKey: productKeys.all })
    },
  })
}

export function useCreateProduct(): UseMutationResult<AdminProductDetail, Error, ProductInput> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: ProductInput) => createAdminProduct(input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: productKeys.all })
    },
  })
}

/** Publicar/despublicar em lote. Devolve `{ok, failed}` — falha parcial é normal. */
export function useBulkStatus(): UseMutationResult<
  BulkStatusResult,
  Error,
  { items: Array<{ id: string; name: string }>; status: ProductStatus }
> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ items, status }) => bulkSetStatus(items, status),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: productKeys.all })
    },
  })
}

// ---------------------------------------------------------------------------
// Imagens
// ---------------------------------------------------------------------------

export interface ImageManagerState {
  images: ProductImageRecord[]
  loading: boolean
  /** Erro de rede/permissão. Recusa de arquivo NÃO passa por aqui. */
  error: string | null
  /** Recusas por arquivo do último envio — o motivo individual, nunca um total. */
  rejections: ImageRejection[]
  /** Quantos arquivos entraram no último envio, para o resumo honesto. */
  lastAccepted: number
  uploading: boolean
  busy: boolean
  upload: (files: File[]) => Promise<void>
  move: (from: number, to: number) => Promise<void>
  makeCover: (imageId: string) => Promise<void>
  remove: (imageId: string) => Promise<void>
  dismissRejections: () => void
}

/**
 * Estado da galeria de imagens de um produto.
 *
 * A reordenação é **otimista com reversão**: arrastar precisa parecer
 * instantâneo (senão a foto volta para o lugar antigo por um instante e o
 * arrastar fica intragável), mas se o `PUT .../order` falhar a lista volta ao
 * que o servidor tem — nunca fica exibindo uma ordem que só existe na tela.
 * Por isso a lista anterior é capturada antes da chamada e restaurada no
 * `catch`, e a resposta do servidor substitui o palpite otimista no sucesso.
 */
export function useProductImages(productId: string | undefined): ImageManagerState {
  const qc = useQueryClient()
  const key = productKeys.images(productId ?? '')

  const query = useQuery({
    queryKey: key,
    queryFn: () => fetchProductImages(productId as string),
    enabled: Boolean(productId),
    ...base,
  })

  const [error, setError] = useState<string | null>(null)
  const [rejections, setRejections] = useState<ImageRejection[]>([])
  const [lastAccepted, setLastAccepted] = useState(0)
  const [uploading, setUploading] = useState(false)
  const [busy, setBusy] = useState(false)

  const images = useMemo(() => sortImages(query.data ?? []), [query.data])

  const setImages = useCallback(
    (next: ProductImageRecord[]) => {
      qc.setQueryData(key, next)
    },
    [qc, key],
  )

  const upload = useCallback(
    async (files: File[]) => {
      if (!productId || files.length === 0) return
      setUploading(true)
      setError(null)
      setRejections([])
      try {
        const res = await uploadProductImages(productId, files)
        setLastAccepted(res.uploaded.length)
        setRejections(res.rejected ?? [])
        // Sempre reconsulta: o servidor pode ter deduplicado (mesmo hash devolve
        // a linha existente), e nesse caso concatenar o que veio duplicaria a
        // foto na tela sem ter duplicado nada no banco.
        setImages(await fetchProductImages(productId))
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Falha ao enviar as imagens.')
      } finally {
        setUploading(false)
      }
    },
    [productId, setImages],
  )

  const move = useCallback(
    async (from: number, to: number) => {
      if (!productId) return
      const previous = images
      const optimistic = moveImage(images, from, to)
      if (optimistic === previous) return
      setImages(optimistic)
      setBusy(true)
      setError(null)
      try {
        setImages(await reorderProductImages(productId, imageOrderPayload(optimistic)))
      } catch (err) {
        setImages(previous)
        setError(err instanceof Error ? err.message : 'Não foi possível salvar a nova ordem.')
      } finally {
        setBusy(false)
      }
    },
    [productId, images, setImages],
  )

  const makeCover = useCallback(
    async (imageId: string) => {
      if (!productId) return
      const previous = images
      const optimistic = promoteCover(images, imageId)
      if (optimistic === previous) return
      setImages(optimistic)
      setBusy(true)
      setError(null)
      try {
        setImages(await setProductImageCover(productId, imageId))
      } catch (err) {
        setImages(previous)
        setError(err instanceof Error ? err.message : 'Não foi possível definir a capa.')
      } finally {
        setBusy(false)
      }
    },
    [productId, images, setImages],
  )

  const remove = useCallback(
    async (imageId: string) => {
      if (!productId) return
      const previous = images
      // Remoção NÃO é otimista: apagar é destrutivo e irreversível pela tela.
      // Sumir com a foto antes da confirmação faria o dono acreditar que
      // perdeu a imagem quando o servidor recusou — e ele reenviaria à toa.
      setBusy(true)
      setError(null)
      try {
        setImages(await deleteProductImage(productId, imageId))
      } catch (err) {
        setImages(previous)
        setError(err instanceof Error ? err.message : 'Não foi possível remover a imagem.')
      } finally {
        setBusy(false)
      }
    },
    [productId, images, setImages],
  )

  return {
    images,
    loading: query.isLoading,
    error: error ?? (query.isError ? (query.error as Error).message : null),
    rejections,
    lastAccepted,
    uploading,
    busy,
    upload,
    move,
    makeCover,
    remove,
    dismissRejections: () => setRejections([]),
  }
}
