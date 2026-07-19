import { AlertTriangle } from 'lucide-react'
import { cn } from '@/lib/cn'
import {
  EmptyState,
  LoadingRows,
  ScrollArea,
  Section,
  Table,
  Td,
  Th,
} from '@/components/admin/primitives'
import { formatCount } from '@/lib/adminFormat'
import { formatDateTime } from '@/lib/format'
import type { ImportBatch } from '@/lib/adminImportTypes'

/**
 * Histórico de lotes.
 *
 * Responde "o que foi importado, quando, por quem e com que resultado" — e
 * principalmente "qual importação estragou o catálogo na terça".
 *
 * O status `validated` recebe destaque próprio porque é o estado mais
 * traiçoeiro: o lote foi conferido e NUNCA aprovado. Sem sinalizar, o operador
 * lembra de ter subido a planilha, vê a linha no histórico e conclui que
 * importou — enquanto nada foi aplicado.
 */

const STATUS_STYLE: Record<string, { label: string; chip: string }> = {
  committed: { label: 'Aplicado', chip: 'bg-emerald-50 text-emerald-800 ring-emerald-600/20' },
  validated: { label: 'Conferido, não aplicado', chip: 'bg-amber-50 text-amber-900 ring-amber-600/25' },
  failed: { label: 'Falhou', chip: 'bg-red-50 text-red-800 ring-red-600/25' },
  uploaded: { label: 'Enviado', chip: 'bg-gray-100 text-gray-600 ring-gray-300' },
  staged: { label: 'Em preparo', chip: 'bg-gray-100 text-gray-600 ring-gray-300' },
}

export function ImportHistory({
  batches,
  loading,
}: {
  batches: ImportBatch[] | undefined
  loading: boolean
}) {
  return (
    <Section
      title="Histórico de importações"
      description="Todo lote fica registrado, inclusive o que foi conferido e nunca aprovado."
    >
      {loading && !batches && <LoadingRows rows={4} />}
      {batches && batches.length === 0 && (
        <EmptyState
          title="Nenhuma importação ainda"
          description="Os lotes aparecem aqui assim que a primeira planilha for enviada."
        />
      )}
      {batches && batches.length > 0 && (
        <ScrollArea>
          <Table>
            <thead>
              <tr>
                <Th>Quando</Th>
                <Th>Arquivo</Th>
                <Th>Perfil</Th>
                <Th>Quem</Th>
                <Th numeric>Criou</Th>
                <Th numeric>Atualizou</Th>
                <Th numeric>Reteve</Th>
                <Th numeric>Rejeitou</Th>
                <Th>Situação</Th>
              </tr>
            </thead>
            <tbody>
              {batches.map((b) => {
                const st = STATUS_STYLE[b.status] ?? { label: b.status, chip: 'bg-gray-100 text-gray-600 ring-gray-300' }
                return (
                  <tr key={b.id} className="hover:bg-gray-50">
                    <Td className="whitespace-nowrap text-xs tabular-nums text-gray-600">
                      {formatDateTime(b.committedAt ?? b.createdAt)}
                    </Td>
                    <Td className="max-w-[16rem]">
                      <span className="block truncate font-medium text-gray-900">{b.filename}</span>
                      <span className="block text-[11px] uppercase text-gray-400">{b.format}</span>
                    </Td>
                    <Td className="whitespace-nowrap text-xs text-gray-600">
                      {b.profile ? `${b.profile} v${b.profileVersion ?? 1}` : '—'}
                    </Td>
                    <Td className="whitespace-nowrap text-xs text-gray-600">{b.createdBy || '—'}</Td>
                    <Td numeric className="text-xs text-gray-700">{formatCount(b.summary.creates)}</Td>
                    <Td numeric className="text-xs text-gray-700">{formatCount(b.summary.updates)}</Td>
                    <Td numeric className={cn('text-xs', b.summary.reviews > 0 ? 'font-semibold text-amber-700' : 'text-gray-400')}>
                      {formatCount(b.summary.reviews)}
                    </Td>
                    <Td numeric className={cn('text-xs', b.summary.rejects > 0 ? 'font-semibold text-red-700' : 'text-gray-400')}>
                      {formatCount(b.summary.rejects)}
                    </Td>
                    <Td>
                      <span
                        className={cn(
                          'inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-semibold ring-1 ring-inset',
                          st.chip,
                        )}
                      >
                        {st.label}
                      </span>
                      {b.error && (
                        <span className="mt-1 flex items-start gap-1 text-[11px] leading-snug text-red-700">
                          <AlertTriangle className="mt-0.5 h-3 w-3 shrink-0" aria-hidden="true" />
                          {b.error}
                        </span>
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
  )
}

export default ImportHistory
