import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Star } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import { EmptyState, ErrorState, LoadingRows, Section } from '@/components/admin/primitives'
import { cn } from '@/lib/cn'
import { useAdminReviews, useModerateReview } from '@/hooks/useAdminReviews'
import {
  isReviewsAdminEnabled,
  REVIEW_STATUS_LABEL,
  type AdminReview,
  type ReviewStatus,
} from '@/lib/adminReviewsApi'

const TABS: ReviewStatus[] = ['pending', 'published', 'rejected']

function Stars({ n }: { n: number }) {
  return (
    <span className="inline-flex" aria-label={`${n} de 5`}>
      {[1, 2, 3, 4, 5].map((i) => (
        <Star
          key={i}
          className={cn(
            'h-3.5 w-3.5',
            i <= n ? 'fill-brand-gold text-brand-gold' : 'text-gray-300'
          )}
          aria-hidden="true"
        />
      ))}
    </span>
  )
}

/**
 * Moderação de avaliações. Só o que a triagem automática segurou (link, contato,
 * caixa alta) chega aqui — ver internal/review/moderation.go. Aprovar publica;
 * recusar tira de vista (com nota opcional). É catálogo, então admin+vendas.
 */
export default function ReviewsPage() {
  const [params, setParams] = useSearchParams()
  const status = (params.get('status') as ReviewStatus) || 'pending'
  const setStatus = (s: ReviewStatus) =>
    setParams(
      (prev) => {
        const sp = new URLSearchParams(prev)
        sp.set('status', s)
        return sp
      },
      { replace: true }
    )

  const { data: rows = [], isLoading, isError, error, refetch } = useAdminReviews(status)

  return (
    <AdminShell
      title="Avaliações"
      description="Moderar o que a triagem automática segurou — aprovar ou recusar."
    >
      <div className="space-y-4">
        {!isReviewsAdminEnabled && (
          <p className="rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-amber-50/60 p-3 text-xs leading-relaxed text-gray-700">
            <strong>Modo demonstração.</strong> O catálogo não está configurado (
            <code className="font-mono">VITE_CATALOG_URL</code> vazio): a fila abaixo é inventada.
          </p>
        )}

        <div className="flex flex-wrap gap-1.5">
          {TABS.map((s) => (
            <button
              key={s}
              type="button"
              onClick={() => setStatus(s)}
              className={cn(
                'rounded-full px-3 py-1 text-xs font-semibold transition-colors',
                status === s
                  ? 'bg-brand-blue text-white'
                  : 'bg-white text-gray-600 ring-1 ring-inset ring-gray-300 hover:bg-gray-50'
              )}
            >
              {REVIEW_STATUS_LABEL[s]}
            </button>
          ))}
        </div>

        <Section title={REVIEW_STATUS_LABEL[status]} description={`${rows.length} avaliação(ões)`}>
          {isError ? (
            <div className="p-4">
              <ErrorState
                message={error instanceof Error ? error.message : 'Falha ao carregar'}
                onRetry={() => void refetch()}
              />
            </div>
          ) : isLoading ? (
            <LoadingRows rows={5} />
          ) : rows.length === 0 ? (
            <div className="p-4">
              <EmptyState title="Nada aqui" description="Nenhuma avaliação neste estado." />
            </div>
          ) : (
            <ul className="divide-y divide-gray-100">
              {rows.map((r) => (
                <ReviewItem key={r.id} review={r} moderable={status === 'pending'} />
              ))}
            </ul>
          )}
        </Section>
      </div>
    </AdminShell>
  )
}

function ReviewItem({ review, moderable }: { review: AdminReview; moderable: boolean }) {
  const mod = useModerateReview()
  const [note, setNote] = useState('')

  return (
    <li className="p-3 sm:p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <Stars n={review.rating} />
            <span className="text-sm font-semibold text-gray-800">
              {review.title || 'Sem título'}
            </span>
          </div>
          {review.body && <p className="mt-1 text-sm text-gray-600">{review.body}</p>}
          <p className="mt-1 text-xs text-gray-400">
            {review.authorName} · produto{' '}
            <span className="font-mono">{review.productId.slice(0, 8)}</span>
          </p>
          {review.moderationNote && (
            <p className="mt-1 text-xs text-gray-500">Nota: {review.moderationNote}</p>
          )}
        </div>

        {moderable && (
          <div className="flex flex-col items-end gap-1.5">
            <div className="flex gap-1.5">
              <button
                type="button"
                onClick={() => mod.mutate({ id: review.id, action: 'approve' })}
                disabled={mod.isPending}
                className="rounded-md bg-emerald-600 px-2.5 py-1 text-xs font-semibold text-white hover:bg-emerald-700 disabled:opacity-50"
              >
                Publicar
              </button>
              <button
                type="button"
                onClick={() =>
                  mod.mutate({ id: review.id, action: 'reject', note: note.trim() || undefined })
                }
                disabled={mod.isPending}
                className="rounded-md border border-red-200 px-2.5 py-1 text-xs font-semibold text-red-700 hover:bg-red-50 disabled:opacity-50"
              >
                Recusar
              </button>
            </div>
            <input
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="Nota da recusa (opcional)"
              className="w-52 rounded-md border border-gray-300 bg-white px-2 py-1 text-xs text-gray-900 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue"
            />
          </div>
        )}
      </div>
      {mod.isError && (
        <p className="mt-1 text-xs text-red-700">
          {mod.error instanceof Error ? mod.error.message : 'Falha ao moderar'}
        </p>
      )}
    </li>
  )
}
