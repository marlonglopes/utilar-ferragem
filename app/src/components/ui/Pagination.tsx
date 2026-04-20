import { cn } from '@/lib/cn'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export interface PaginationProps {
  page: number
  totalPages: number
  onPageChange: (page: number) => void
  className?: string
}

function Pagination({ page, totalPages, onPageChange, className }: PaginationProps) {
  const { t } = useTranslation()

  const pages = Array.from({ length: totalPages }, (_, i) => i + 1)
  const visiblePages = pages.filter((p) => Math.abs(p - page) <= 2)

  if (totalPages <= 1) return null

  return (
    <nav aria-label={t('pagination.page')} className={cn('flex items-center gap-1', className)}>
      <button
        onClick={() => onPageChange(page - 1)}
        disabled={page <= 1}
        className="flex h-8 w-8 items-center justify-center rounded-lg border border-gray-300 text-gray-600 hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed"
        aria-label={t('pagination.previous')}
      >
        <ChevronLeft className="h-4 w-4" aria-hidden />
      </button>

      {visiblePages[0] > 1 && (
        <>
          <PageButton p={1} current={page} onPageChange={onPageChange} />
          {visiblePages[0] > 2 && <span className="px-1 text-gray-400">…</span>}
        </>
      )}

      {visiblePages.map((p) => (
        <PageButton key={p} p={p} current={page} onPageChange={onPageChange} />
      ))}

      {visiblePages[visiblePages.length - 1] < totalPages && (
        <>
          {visiblePages[visiblePages.length - 1] < totalPages - 1 && (
            <span className="px-1 text-gray-400">…</span>
          )}
          <PageButton p={totalPages} current={page} onPageChange={onPageChange} />
        </>
      )}

      <button
        onClick={() => onPageChange(page + 1)}
        disabled={page >= totalPages}
        className="flex h-8 w-8 items-center justify-center rounded-lg border border-gray-300 text-gray-600 hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed"
        aria-label={t('pagination.next')}
      >
        <ChevronRight className="h-4 w-4" aria-hidden />
      </button>
    </nav>
  )
}

function PageButton({
  p,
  current,
  onPageChange,
}: {
  p: number
  current: number
  onPageChange: (page: number) => void
}) {
  const active = p === current
  return (
    <button
      onClick={() => onPageChange(p)}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'flex h-8 w-8 items-center justify-center rounded-lg text-sm font-medium transition-colors',
        active
          ? 'bg-brand-orange text-white'
          : 'border border-gray-300 text-gray-700 hover:bg-gray-50'
      )}
    >
      {p}
    </button>
  )
}

export { Pagination }
