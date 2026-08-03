import { useCallback, useEffect, useRef, useState } from 'react'
import { AlertTriangle, CheckCircle2, RotateCw, Trash2, UploadCloud, XCircle } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import { Section } from '@/components/admin/primitives'
import { cn } from '@/lib/cn'
import { compressImage } from '@/lib/imageCompress'
import {
  isBulkImagesEnabled,
  resolveBySku,
  runPool,
  skuCandidates,
  skuFromFilename,
  uploadProductImage,
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
let seq = 0

/**
 * Upload de imagens EM LOTE por SKU — o que faltava para dar foto a centenas de
 * produtos sem abrir um por um.
 *
 * A loja fotografa cada produto no celular, nomeia o arquivo pelo SKU (6320.jpg),
 * solta tudo aqui; casamos por SKU, comprimimos no cliente e subimos em paralelo
 * (5 por vez) pelo pipeline de normalização que já existe. Progresso e retry por
 * arquivo; foto errada nunca acontece porque o casamento é pelo código.
 */
export default function BulkImagesPage() {
  const [items, setItems] = useState<Item[]>([])
  const [dragging, setDragging] = useState(false)
  const [resolving, setResolving] = useState(false)
  const [uploading, setUploading] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

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

  const addFiles = useCallback(async (files: File[]) => {
    const imgs = files.filter((f) => f.type.startsWith('image/'))
    if (imgs.length === 0) return
    const novos: Item[] = imgs.map((file) => {
      const candidates = skuCandidates(file.name)
      return {
        uid: `f${seq++}`,
        file,
        candidates,
        sku: skuFromFilename(file.name),
        previewUrl: URL.createObjectURL(file),
        status: 'pending' as Status,
        progress: 0,
      }
    })
    setItems((prev) => [...prev, ...novos])

    // Resolve todos os candidatos em lote e casa cada arquivo ao 1º que existir.
    setResolving(true)
    try {
      const skus = Array.from(new Set(novos.flatMap((n) => n.candidates).filter(Boolean)))
      const matches = await resolveBySku(skus)
      const bySku = new Map(matches.map((m) => [m.sku, m]))
      setItems((prev) =>
        prev.map((it) => {
          if (it.status !== 'pending' || it.match) return it
          const hit = it.candidates.find((c) => bySku.has(c))
          return hit
            ? { ...it, match: bySku.get(hit), sku: hit }
            : { ...it, status: 'unmatched' as Status }
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
    void addFiles(Array.from(e.dataTransfer.files))
  }
  const onPaste = (e: React.ClipboardEvent) => {
    const files = Array.from(e.clipboardData.files)
    if (files.length) void addFiles(files)
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
    setUploading(true)
    await runPool(fila, uploadOne, CONCURRENCY)
    setUploading(false)
  }, [items, uploadOne])

  const removeItem = (uid: string) => {
    setItems((prev) => {
      const it = prev.find((x) => x.uid === uid)
      if (it) URL.revokeObjectURL(it.previewUrl)
      return prev.filter((x) => x.uid !== uid)
    })
  }
  const clearDone = () => {
    setItems((prev) => {
      prev.filter((x) => x.status === 'done').forEach((x) => URL.revokeObjectURL(x.previewUrl))
      return prev.filter((x) => x.status !== 'done')
    })
  }

  const matched = items.filter((it) => it.match)
  const unmatched = items.filter((it) => it.status === 'unmatched')
  const pendentes = matched.filter((it) => it.status === 'pending' || it.status === 'error').length
  const enviados = matched.filter((it) => it.status === 'done').length

  return (
    <AdminShell
      title="Imagens em lote"
      description="Solte fotos nomeadas pelo SKU (ex.: 6320.jpg); casamos por código e subimos em paralelo."
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
            Arraste imagens aqui, clique para selecionar ou cole (Ctrl+V)
          </p>
          <p className="text-xs text-gray-500">
            Nomeie cada arquivo pelo SKU do produto — <span className="font-mono">6320.jpg</span>.
            Várias fotos do mesmo produto: <span className="font-mono">6320-2.jpg</span>.
          </p>
          <input
            ref={inputRef}
            type="file"
            accept="image/*"
            multiple
            className="hidden"
            onChange={(e) => {
              void addFiles(Array.from(e.target.files ?? []))
              e.target.value = ''
            }}
          />
        </div>

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

        {/* Grade de casados */}
        {matched.length > 0 && (
          <Section title="Casados por SKU">
            <div className="grid grid-cols-2 gap-3 p-3 sm:grid-cols-3 lg:grid-cols-4 sm:p-4">
              {matched.map((it) => (
                <ItemCard
                  key={it.uid}
                  item={it}
                  onRetry={() => void uploadOne(it)}
                  onRemove={() => removeItem(it.uid)}
                  busy={uploading}
                />
              ))}
            </div>
          </Section>
        )}

        {/* Não casados */}
        {unmatched.length > 0 && (
          <Section
            title="Sem produto"
            description="O SKU do nome do arquivo não bate com nenhum produto — confira o nome."
          >
            <ul className="divide-y divide-gray-100">
              {unmatched.map((it) => (
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
        <img
          src={item.previewUrl}
          alt={item.match?.name ?? item.sku}
          className="h-full w-full object-contain"
        />
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
