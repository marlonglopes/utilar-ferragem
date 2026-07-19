import { Skeleton } from '@/components/ui'

/**
 * Fallback do <Suspense> enquanto o chunk da rota carrega.
 * Imita o esqueleto de uma listagem (título + grid de cards) para evitar
 * layout shift perceptível na troca de rota.
 */
export function PageFallback() {
  return (
    <div className="container py-8" role="status" aria-busy="true" aria-label="Carregando página">
      <Skeleton className="h-4 w-48 mb-6" />
      <Skeleton className="h-8 w-72 mb-8" />
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
        {Array.from({ length: 8 }).map((_, i) => (
          <div key={i} className="flex flex-col gap-3">
            <Skeleton className="aspect-square w-full rounded-xl" />
            <Skeleton className="h-3 w-3/4" />
            <Skeleton className="h-3 w-1/2" />
          </div>
        ))}
      </div>
      <span className="sr-only">Carregando…</span>
    </div>
  )
}
