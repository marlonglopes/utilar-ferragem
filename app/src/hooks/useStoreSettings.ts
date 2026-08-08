import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchStoreSettings, updateAnnouncement, type Announcement } from '@/lib/storeSettingsApi'

const KEY = ['store', 'settings']

// Lida pela vitrine (público) e pelo painel. staleTime folgado: o aviso não
// muda de segundo em segundo, e uma cotação nova não vale um refetch por foco.
export function useStoreSettings() {
  return useQuery({
    queryKey: KEY,
    queryFn: fetchStoreSettings,
    staleTime: 60_000,
  })
}

export function useUpdateAnnouncement() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (a: Announcement) => updateAnnouncement(a),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  })
}
