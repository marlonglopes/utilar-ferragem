import { useCallback, useRef, useState } from 'react'
import { AlertTriangle, FileSpreadsheet, Loader2, UploadCloud } from 'lucide-react'
import { cn } from '@/lib/cn'
import {
  ACCEPTED_EXTENSIONS,
  MAX_UPLOAD_BYTES,
  formatBytes,
} from '@/lib/adminImportFormat'
import { formatCount } from '@/lib/adminFormat'

/**
 * Passo 1 — enviar a planilha.
 *
 * O arrastar-e-soltar é a interação principal (o arquivo vem do e-mail do
 * fornecedor, já aberto ao lado), mas o `<input type="file">` continua existindo
 * de verdade, não escondido atrás de um `div` com `onClick`: é o que dá teclado,
 * leitor de tela e o seletor de arquivos do celular de graça.
 *
 * A validação aqui é diagnóstico, não defesa — o backend tem o mesmo teto. O
 * ganho é dizer "esse .pdf não serve" antes de gastar minutos de rede da loja.
 */
export function ImportDropzone({
  file,
  error,
  loading,
  detectedRows,
  detectedColumns,
  onSelect,
}: {
  file: File | null
  error: string | null
  loading: boolean
  detectedRows?: number
  detectedColumns?: number
  onSelect: (file: File | null) => void
}) {
  const [dragging, setDragging] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const onDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault()
      setDragging(false)
      const dropped = e.dataTransfer.files?.[0]
      if (dropped) onSelect(dropped)
    },
    [onSelect],
  )

  return (
    <div className="p-3 sm:p-4">
      <div
        data-testid="import-dropzone"
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        className={cn(
          'rounded-lg border-2 border-dashed px-4 py-8 text-center transition-colors',
          dragging ? 'border-brand-blue bg-brand-blue-light' : 'border-gray-300 bg-gray-50',
          error && 'border-red-300 bg-red-50',
        )}
      >
        {loading ? (
          <Loader2 className="mx-auto h-8 w-8 animate-spin text-brand-blue" aria-hidden="true" />
        ) : (
          <UploadCloud className="mx-auto h-8 w-8 text-gray-400" aria-hidden="true" />
        )}
        <p className="mt-3 text-sm font-semibold text-gray-800">
          {loading ? 'Lendo o arquivo e detectando as colunas…' : 'Arraste a planilha aqui'}
        </p>
        <p className="mt-1 text-xs text-gray-500">
          {ACCEPTED_EXTENSIONS.join(', ')} · até {formatBytes(MAX_UPLOAD_BYTES)}
        </p>

        <label
          className={cn(
            'mt-4 inline-flex cursor-pointer items-center gap-2 rounded-md bg-brand-blue px-3 py-2 text-xs font-semibold text-white hover:bg-brand-blue/90',
            loading && 'pointer-events-none opacity-60',
          )}
        >
          Escolher arquivo
          <input
            ref={inputRef}
            type="file"
            className="sr-only"
            accept={ACCEPTED_EXTENSIONS.join(',')}
            disabled={loading}
            onChange={(e) => {
              onSelect(e.target.files?.[0] ?? null)
              // Zera o input: sem isso, escolher o MESMO arquivo de novo (depois
              // de corrigi-lo no Excel) não dispara `change` e a tela parece
              // travada — o modo de falha clássico de input de arquivo.
              e.target.value = ''
            }}
          />
        </label>

        <p className="mx-auto mt-4 max-w-md text-[11px] leading-relaxed text-gray-500">
          Prefira <strong>CSV UTF-8</strong> quando o fornecedor oferecer: o Excel converte código
          de barras para notação científica e transforma código em data ao salvar em XLSX.
        </p>
      </div>

      {error && (
        <div
          role="alert"
          className="mt-3 flex items-start gap-2 rounded-md border border-red-200 border-l-4 border-l-red-600 bg-red-50 p-3"
        >
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-red-700" aria-hidden="true" />
          <div className="min-w-0">
            <p className="text-sm font-semibold text-red-800">Arquivo não aceito</p>
            <p className="mt-0.5 text-xs leading-relaxed text-red-700">{error}</p>
          </div>
        </div>
      )}

      {file && !error && (
        <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 rounded-md border border-gray-200 bg-white p-3">
          <FileSpreadsheet className="h-5 w-5 shrink-0 text-brand-blue" aria-hidden="true" />
          <span className="min-w-0 flex-1 truncate text-sm font-medium text-gray-900">{file.name}</span>
          <span className="text-xs tabular-nums text-gray-500">{formatBytes(file.size)}</span>
          {detectedRows !== undefined && (
            <span className="text-xs tabular-nums text-gray-500">
              {formatCount(detectedRows)} linhas
            </span>
          )}
          {detectedColumns !== undefined && (
            <span className="text-xs tabular-nums text-gray-500">
              {formatCount(detectedColumns)} colunas
            </span>
          )}
        </div>
      )}
    </div>
  )
}

export default ImportDropzone
