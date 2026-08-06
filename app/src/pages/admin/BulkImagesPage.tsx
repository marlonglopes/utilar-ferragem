import { useCallback, useEffect, useRef, useState } from 'react'
import {
  AlertTriangle,
  CheckCircle2,
  Loader2,
  RotateCw,
  Trash2,
  UploadCloud,
  XCircle,
} from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import { Section } from '@/components/admin/primitives'
import { cn } from '@/lib/cn'
import { compressImage } from '@/lib/imageCompress'
import { isHeic } from '@/lib/heic'
import {
  collectFilesFromDataTransfer,
  isBulkImagesEnabled,
  planUploadOrder,
  resolveBySku,
  runPool,
  skuCandidatesForFile,
  uploadProductImage,
  type CollectedFile,
  type SkuMatch,
} from '@/lib/adminBulkImagesApi'

type Status = 'pending' | 'uploading' | 'done' | 'error' | 'unmatched'

interface Item {
  uid: string
  file: File
  sku: string
  candidates: string[]
  previewUrl: string
  match?: SkuMatch
  status: Status
  progress: number
  error?: string
}

const CONCURRENCY = 5

// Teto de previews/cards renderizados de uma vez. Um lote pode ter MILHARES de
// fotos (500 pastas x N); criar object URL + <img> pra todas trava a aba. Acima
// do teto, os itens continuam sendo casados e ENVIADOS normalmente — só não
// geram preview nem card (mostramos um resumo "+N"). Ver o teste de carga.
const PREVIEW_CAP = 150
let seq = 0

/**
 * Upload de imagens EM LOTE por SKU — o que faltava para dar foto a centenas de
 * produtos sem abrir um por um.
 *
 * A loja fotografa cada produto no celular e organiza por SKU — seja nomeando o
 * arquivo pelo código (6320.jpg) OU criando 1 PASTA por SKU (6320/foto1.jpg).
 * Solta tudo aqui (arquivos ou pastas inteiras); casamos por SKU (pasta primeiro,
 * nome do arquivo como fallback), comprimimos no cliente e subimos em paralelo
 * (5 por vez). Foto errada nunca acontece porque o casamento é pelo código.
 */
export default function BulkImagesPage() {
  const [items, setItems] = useState<Item[]>([])
  const [dragging, setDragging] = useState(false)
  const [resolving, setResolving] = useState(false)
  const [reading, setReading] = useState(false)
  const [notice, setNotice] = useState('')
  const [uploading, setUploading] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const folderInputRef = useRef<HTMLInputElement>(null)
  // Conta quantos previews (object URLs) já criamos, pra parar no PREVIEW_CAP.
  const previewCount = useRef(0)

  // webkitdirectory não é atributo tipado do React — setamos no ref.
  useEffect(() => {
    const el = folderInputRef.current
    if (el) {
      el.setAttribute('webkitdirectory', '')
      el.setAttribute('directory', '')
    }
  }, [])

  // Limpa os object URLs ao desmontar (evita vazamento de memória).
  useEffect(
    () => () => {
      items.forEach((it) => URL.revokeObjectURL(it.previewUrl))
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []
  )

  // Avisa se tentar sair com upload em andamento.
  useEffect(() => {
    if (!uploading) return
    const h = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', h)
    return () => window.removeEventListener('beforeunload', h)
  }, [uploading])

  const patch = useCallback((uid: string, up: Partial<Item>) => {
    setItems((prev) => prev.map((it) => (it.uid === uid ? { ...it, ...up } : it)))
  }, [])

  const addCollected = useCallback(async (collected: CollectedFile[]) => {
    // HEIC (foto de iPhone) entra pela extensão: no drag de pasta o navegador
    // manda esses arquivos com `type` vazio, então `startsWith('image/')`
    // sozinho os descartaria. A conversão pra JPEG acontece no envio.
    const imgs = collected.filter((c) => c.file.type.startsWith('image/') || isHeic(c.file))
    if (imgs.length === 0) {
      setNotice(
        collected.length > 0
          ? `Li ${collected.length} arquivo(s), mas nenhum é imagem (use jpg/png/webp/heic).`
          : 'Nada foi lido. Arraste pastas ou arquivos de imagem.'
      )
      return
    }
    setNotice(`${imgs.length} foto(s) lida(s) — casando por SKU…`)
    const novos: Item[] = imgs.map(({ file, path }) => {
      const candidates = skuCandidatesForFile(path)
      // Preview só até o teto — o resto é enviado sem preview (não trava a aba).
      // HEIC também fica sem preview: <img> não renderiza HEIC fora do Safari, e
      // um object URL viraria "imagem quebrada". O card mostra o SKU; a foto é
      // convertida e enviada normalmente.
      let previewUrl = ''
      if (!isHeic(file) && previewCount.current < PREVIEW_CAP) {
        previewUrl = URL.createObjectURL(file)
        previewCount.current++
      }
      return {
        uid: `f${seq++}`,
        file,
        candidates,
        sku: candidates[0] ?? '',
        previewUrl,
        status: 'pending' as Status,
        progress: 0,
      }
    })
    setItems((prev) => [...prev, ...novos])

    // Resolve todos os candidatos EM LOTES (resolveBySku fatia p/ não estourar
    // 414) e casa cada arquivo ao 1º candidato que existir (pasta primeiro).
    setResolving(true)
    try {
      const skus = Array.from(new Set(novos.flatMap((n) => n.candidates).filter(Boolean)))
      const matches = await resolveBySku(skus)
      const bySku = new Map(matches.map((m) => [m.sku, m]))
      setItems((prev) =>
        prev.map((it) => {
          if (it.status !== 'pending' || it.match) return it
          // CORREÇÃO: nunca subir no produto errado. Se a pasta e o nome do
          // arquivo casam em produtos DIFERENTES, é ambíguo → deixa sem produto
          // (o lojista confere) em vez de chutar o primeiro candidato.
          const hits = it.candidates.map((c) => bySku.get(c)).filter((m): m is SkuMatch => !!m)
          const distinct = new Map(hits.map((m) => [m.id, m]))
          if (distinct.size === 1) {
            const m = hits[0]
            return { ...it, match: m, sku: m.sku }
          }
          if (distinct.size > 1) {
            return { ...it, status: 'unmatched' as Status, error: 'ambíguo: pasta e arquivo' }
          }
          return { ...it, status: 'unmatched' as Status }
        })
      )
    } catch {
      // Falha ao resolver: deixa como pending; o usuário pode tentar de novo.
    } finally {
      setResolving(false)
    }
  }, [])

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault()
    setDragging(false)
    // Desce em pastas (webkitGetAsEntry): arrastar pastas inteiras agora funciona.
    setReading(true)
    setNotice('Lendo pastas…')
    collectFilesFromDataTransfer(e.dataTransfer)
      .then(addCollected)
      .catch((err) =>
        setNotice(`Erro ao ler: ${err instanceof Error ? err.message : 'desconhecido'}`)
      )
      .finally(() => setReading(false))
  }
  const onPaste = (e: React.ClipboardEvent) => {
    const files = Array.from(e.clipboardData.files)
    if (files.length) void addCollected(files.map((file) => ({ file, path: file.name })))
  }

  // Arquivos ou pasta (webkitdirectory): webkitRelativePath preserva a pasta=SKU.
  const onPick = (list: FileList | null) => {
    const collected = Array.from(list ?? []).map((file) => ({
      file,
      path: file.webkitRelativePath || file.name,
    }))
    if (collected.length) void addCollected(collected)
  }

  const uploadOne = useCallback(
    async (it: Item) => {
      if (!it.match) return
      patch(it.uid, { status: 'uploading', progress: 0, error: undefined })
      try {
        const compressed = await compressImage(it.file)
        await uploadProductImage(it.match.id, compressed, (pct) => patch(it.uid, { progress: pct }))
        patch(it.uid, { status: 'done', progress: 100 })
      } catch (err) {
        patch(it.uid, {
          status: 'error',
          error: err instanceof Error ? err.message : 'falha',
        })
      }
    },
    [patch]
  )

  const uploadAll = useCallback(async () => {
    const fila = items.filter(
      (it) => it.match && (it.status === 'pending' || it.status === 'error')
    )
    if (fila.length === 0) return
    // Capa determinística: agrupa por produto, ordem natural do nome; sobe cada
    // grupo EM SÉRIE (1ª foto = capa) e grupos diferentes em paralelo.
    const groups = planUploadOrder(
      fila.map((it) => ({ productId: it.match!.id, fileName: it.file.name, it }))
    )
    setUploading(true)
    await runPool(
      groups,
      async (group) => {
        for (const w of group) await uploadOne(w.it)
      },
      CONCURRENCY
    )
    setUploading(false)
  }, [items, uploadOne])

  // Revogar/decrementar FORA do updater do setState (updater tem de ser puro).
  const dropPreview = (it?: Item) => {
    if (it?.previewUrl) {
      URL.revokeObjectURL(it.previewUrl)
      previewCount.current = Math.max(0, previewCount.current - 1)
    }
  }
  const removeItem = (uid: string) => {
    dropPreview(items.find((x) => x.uid === uid))
    setItems((prev) => prev.filter((x) => x.uid !== uid))
  }
  const clearDone = () => {
    items.filter((x) => x.status === 'done').forEach(dropPreview)
    setItems((prev) => prev.filter((x) => x.status !== 'done'))
  }

  const matched = items.filter((it) => it.match)
  const unmatched = items.filter((it) => it.status === 'unmatched')
  const pendentes = matched.filter((it) => it.status === 'pending' || it.status === 'error').length
  const enviados = matched.filter((it) => it.status === 'done').length

  return (
    <AdminShell
      title="Imagens em lote"
      description="Solte fotos nomeadas pelo SKU (6320.jpg) ou 1 pasta por SKU; casamos por código e subimos em paralelo."
    >
      <div className="space-y-4">
        {!isBulkImagesEnabled && (
          <p className="rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-amber-50/60 p-3 text-xs leading-relaxed text-gray-700">
            <strong>Modo demonstração.</strong> O catálogo não está configurado (
            <code className="font-mono">VITE_CATALOG_URL</code> vazio): o casamento e o envio são
            simulados.
          </p>
        )}

        {/* Drop zone */}
        <div
          onDragOver={(e) => {
            e.preventDefault()
            setDragging(true)
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={onDrop}
          onPaste={onPaste}
          onClick={() => inputRef.current?.click()}
          role="button"
          tabIndex={0}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') inputRef.current?.click()
          }}
          aria-label="Arraste imagens aqui ou clique para selecionar"
          className={cn(
            'flex cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed p-8 text-center transition-all',
            dragging
              ? 'border-brand-orange bg-brand-orange/5 scale-[1.01]'
              : 'border-gray-300 bg-gray-50 hover:border-gray-400'
          )}
        >
          <UploadCloud className="h-9 w-9 text-gray-400" aria-hidden="true" />
          <p className="text-sm font-semibold text-gray-700">
            Arraste imagens ou pastas aqui, clique para selecionar, ou cole (Ctrl+V)
          </p>
          <p className="text-xs text-gray-500">
            Nomeie o arquivo pelo SKU (<span className="font-mono">6320.jpg</span>) ou crie{' '}
            <strong>1 pasta por SKU</strong> (<span className="font-mono">6320/foto.jpg</span>).
            Foto de iPhone (HEIC) é convertida automaticamente.
          </p>
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              folderInputRef.current?.click()
            }}
            className="mt-1 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-xs font-semibold text-gray-700 hover:bg-gray-50"
          >
            Selecionar pasta
          </button>
          <input
            ref={inputRef}
            type="file"
            // .heic/.heif explícitos: alguns seletores desktop filtram HEIC do
            // "image/*" (o SO não sabe classificar) e a foto de iPhone sumiria.
            accept="image/*,.heic,.heif"
            multiple
            className="hidden"
            onChange={(e) => {
              onPick(e.target.files)
              e.target.value = ''
            }}
          />
          {/* webkitdirectory setado via ref (atributo não tipado no React). */}
          <input
            ref={folderInputRef}
            type="file"
            multiple
            className="hidden"
            onChange={(e) => {
              onPick(e.target.files)
              e.target.value = ''
            }}
          />
        </div>

        {/* Feedback imediato do drop: lendo pastas / quantas fotos / erro. */}
        {(reading || notice) && (
          <p
            className="flex items-center gap-2 rounded-md border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-700"
            role="status"
            aria-live="polite"
          >
            {reading && (
              <Loader2
                className="h-4 w-4 shrink-0 animate-spin text-brand-orange"
                aria-hidden="true"
              />
            )}
            {reading ? 'Lendo pastas…' : notice}
          </p>
        )}

        {/* Barra de ação / resumo */}
        {items.length > 0 && (
          <div className="flex flex-wrap items-center gap-3 rounded-lg border border-gray-200 bg-white p-3">
            <span className="text-sm text-gray-700">
              <strong>{matched.length}</strong> casado(s) · <strong>{enviados}</strong> enviado(s) ·{' '}
              <strong>{pendentes}</strong> na fila
              {unmatched.length > 0 && (
                <span className="text-red-700"> · {unmatched.length} sem produto</span>
              )}
              {resolving && <span className="text-gray-400"> · casando…</span>}
            </span>
            <div className="ml-auto flex gap-2">
              {enviados > 0 && (
                <button
                  type="button"
                  onClick={clearDone}
                  className="rounded-md border border-gray-300 px-3 py-1.5 text-xs font-semibold text-gray-600 hover:bg-gray-50"
                >
                  Limpar enviados
                </button>
              )}
              <button
                type="button"
                onClick={() => void uploadAll()}
                disabled={uploading || pendentes === 0}
                className="rounded-md bg-brand-orange px-3 py-1.5 text-xs font-semibold text-gray-900 hover:bg-brand-orange-dark disabled:opacity-50"
              >
                {uploading ? 'Enviando…' : `Enviar ${pendentes} foto(s)`}
              </button>
            </div>
          </div>
        )}

        {/* Grade de casados — RENDER LIMITADO ao teto (todas são enviadas). */}
        {matched.length > 0 && (
          <Section title="Casados por SKU">
            <div className="grid grid-cols-2 gap-3 p-3 sm:grid-cols-3 lg:grid-cols-4 sm:p-4">
              {matched.slice(0, PREVIEW_CAP).map((it) => (
                <ItemCard
                  key={it.uid}
                  item={it}
                  onRetry={() => void uploadOne(it)}
                  onRemove={() => removeItem(it.uid)}
                  busy={uploading}
                />
              ))}
            </div>
            {matched.length > PREVIEW_CAP && (
              <p className="px-4 pb-3 text-xs text-gray-500">
                Mostrando {PREVIEW_CAP} de <strong>{matched.length}</strong> casados — todas serão
                enviadas.
              </p>
            )}
          </Section>
        )}

        {/* Não casados */}
        {unmatched.length > 0 && (
          <Section
            title="Sem produto"
            description="O SKU (pasta ou nome do arquivo) não bate com nenhum produto — confira."
          >
            <ul className="divide-y divide-gray-100">
              {unmatched.slice(0, PREVIEW_CAP).map((it) => (
                <li key={it.uid} className="flex items-center gap-3 p-3 text-sm">
                  <AlertTriangle className="h-4 w-4 shrink-0 text-amber-500" aria-hidden="true" />
                  <span className="font-mono text-gray-600">{it.file.name}</span>
                  <span className="text-gray-400">
                    → SKU <span className="font-mono">{it.sku || '(vazio)'}</span> não encontrado
                  </span>
                  <button
                    type="button"
                    onClick={() => removeItem(it.uid)}
                    aria-label={`Remover ${it.file.name}`}
                    className="ml-auto rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-700"
                  >
                    <Trash2 className="h-4 w-4" aria-hidden="true" />
                  </button>
                </li>
              ))}
            </ul>
            {unmatched.length > PREVIEW_CAP && (
              <p className="px-3 py-2 text-xs text-gray-500">
                Mostrando {PREVIEW_CAP} de <strong>{unmatched.length}</strong> sem produto.
              </p>
            )}
          </Section>
        )}
      </div>
    </AdminShell>
  )
}

function ItemCard({
  item,
  onRetry,
  onRemove,
  busy,
}: {
  item: Item
  onRetry: () => void
  onRemove: () => void
  busy: boolean
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-gray-200 bg-white">
      <div className="relative aspect-square bg-gray-50">
        {item.previewUrl ? (
          <img
            src={item.previewUrl}
            alt={item.match?.name ?? item.sku}
            className="h-full w-full object-contain"
          />
        ) : (
          // Além do teto de preview: sem <img> (evita broken image), só o SKU.
          <div className="flex h-full w-full items-center justify-center font-mono text-xs text-gray-400">
            {item.sku}
          </div>
        )}
        {item.match?.hasImage && item.status === 'pending' && (
          <span className="absolute left-1.5 top-1.5 rounded bg-amber-500 px-1.5 py-0.5 text-[10px] font-bold text-gray-900">
            já tem foto
          </span>
        )}
        <button
          type="button"
          onClick={onRemove}
          disabled={busy && item.status === 'uploading'}
          aria-label="Remover"
          className="absolute right-1.5 top-1.5 rounded-full bg-white/90 p-1 text-gray-500 shadow hover:bg-white hover:text-red-600 disabled:opacity-40"
        >
          <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
        </button>
        {item.status === 'uploading' && (
          <div className="absolute inset-x-0 bottom-0 h-1 bg-gray-200">
            <div
              className="h-full bg-brand-orange transition-all"
              style={{ width: `${item.progress}%` }}
            />
          </div>
        )}
      </div>
      <div className="flex items-center gap-1.5 p-2">
        <span className="min-w-0 flex-1">
          <span
            className="block truncate text-xs font-semibold text-gray-800"
            title={item.match?.name}
          >
            {item.match?.name ?? '—'}
          </span>
          <span className="block font-mono text-[11px] text-gray-400">SKU {item.sku}</span>
        </span>
        <StatusBadge item={item} onRetry={onRetry} />
      </div>
    </div>
  )
}

function StatusBadge({ item, onRetry }: { item: Item; onRetry: () => void }) {
  if (item.status === 'done')
    return (
      <span className="inline-flex items-center gap-0.5 text-xs font-semibold text-emerald-700">
        <CheckCircle2 className="h-4 w-4" aria-hidden="true" /> ok
      </span>
    )
  if (item.status === 'uploading')
    return <span className="text-xs font-semibold text-gray-500">{item.progress}%</span>
  if (item.status === 'error')
    return (
      <button
        type="button"
        onClick={onRetry}
        title={item.error}
        className="inline-flex items-center gap-0.5 text-xs font-semibold text-red-700 hover:underline"
      >
        <RotateCw className="h-3.5 w-3.5" aria-hidden="true" /> tentar de novo
      </button>
    )
  return (
    <span className="inline-flex items-center gap-0.5 text-xs text-gray-400">
      <XCircle className="h-3.5 w-3.5" aria-hidden="true" /> na fila
    </span>
  )
}
