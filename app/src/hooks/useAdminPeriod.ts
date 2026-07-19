import { useCallback, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import type { AdminPeriod, AdminPeriodPreset } from '@/lib/adminTypes'

/**
 * Período do painel, guardado na URL (`?de=…&ate=…`).
 *
 * Na URL e não no store: o dono manda o link "olha a venda desta semana" para o
 * contador e o filtro vai junto. E, principalmente, **nada de auditoria toca
 * `localStorage`** — a URL é estado de navegação, não armazenamento.
 */

function iso(d: Date): string {
  // Local, não UTC: `toISOString()` em São Paulo devolve o dia seguinte depois
  // das 21h, e o "hoje" do painel viraria amanhã no fim do expediente.
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function shift(days: number, base: Date): Date {
  const d = new Date(base)
  d.setDate(d.getDate() - days)
  return d
}

export function presetToPeriod(preset: AdminPeriodPreset, base = new Date()): AdminPeriod {
  const to = iso(base)
  switch (preset) {
    case 'today':
      return { from: to, to }
    case 'week':
      return { from: iso(shift(6, base)), to }
    case 'month':
      return { from: iso(shift(29, base)), to }
    case 'quarter':
      return { from: iso(shift(89, base)), to }
    case 'year':
      return { from: iso(shift(364, base)), to }
  }
}

export const PERIOD_PRESETS: Array<{ key: AdminPeriodPreset; label: string }> = [
  { key: 'today', label: 'Hoje' },
  { key: 'week', label: '7 dias' },
  { key: 'month', label: '30 dias' },
  { key: 'quarter', label: '90 dias' },
  { key: 'year', label: '12 meses' },
]

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/

export interface UseAdminPeriod {
  period: AdminPeriod
  preset: AdminPeriodPreset | null
  setPreset: (preset: AdminPeriodPreset) => void
  setPeriod: (period: AdminPeriod) => void
}

export function useAdminPeriod(fallback: AdminPeriodPreset = 'month'): UseAdminPeriod {
  const [params, setParams] = useSearchParams()

  const period = useMemo<AdminPeriod>(() => {
    const from = params.get('de')
    const to = params.get('ate')
    // Query string é entrada não confiável — data malformada cai no padrão em
    // vez de virar `from=<script>` numa querystring de API.
    if (from && to && DATE_RE.test(from) && DATE_RE.test(to) && from <= to) {
      return { from, to }
    }
    return presetToPeriod(fallback)
  }, [params, fallback])

  const preset = useMemo<AdminPeriodPreset | null>(() => {
    const found = PERIOD_PRESETS.find((p) => {
      const r = presetToPeriod(p.key)
      return r.from === period.from && r.to === period.to
    })
    return found?.key ?? null
  }, [period])

  const setPeriod = useCallback(
    (next: AdminPeriod) => {
      setParams(
        (prev) => {
          const sp = new URLSearchParams(prev)
          sp.set('de', next.from)
          sp.set('ate', next.to)
          // Trocar o período reinicia a paginação — senão o usuário fica numa
          // página 7 que não existe mais no novo recorte e vê tabela vazia.
          sp.delete('pagina')
          return sp
        },
        { replace: true },
      )
    },
    [setParams],
  )

  const setPreset = useCallback(
    (p: AdminPeriodPreset) => setPeriod(presetToPeriod(p)),
    [setPeriod],
  )

  return { period, preset, setPreset, setPeriod }
}
