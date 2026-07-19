import type { Severity } from '@/lib/adminTypes'

/**
 * Tokens visuais do painel admin.
 *
 * REGRA CENTRAL: **cor semântica ≠ cor de marca.**
 *
 * `brand.orange` e `brand.blue` são identidade da Utilar — cabeçalho, série
 * principal do gráfico, seleção. Eles NUNCA significam "bom" ou "ruim". Estado
 * usa exclusivamente a escala semântica abaixo (verde / âmbar / vermelho), que
 * não aparece em nenhum lugar decorativo. Se laranja significasse "atenção" em
 * uma tela e "marca" na outra, o painel perderia a capacidade de alarmar.
 *
 * Além disso, estado se codifica em **forma além de cor**: toda severidade sai
 * acompanhada de um ponto, uma faixa lateral ou um ícone, para que quem não
 * distingue as cores (e a impressão em preto e branco que o contador vai fazer)
 * continue lendo o painel.
 */

export const SEVERITY_LABEL: Record<Severity, string> = {
  ok: 'Normal',
  warn: 'Atenção',
  critical: 'Crítico',
}

/** Pill completa (fundo + texto + borda). Usada em chips de status. */
export const SEVERITY_PILL: Record<Severity, string> = {
  ok: 'bg-emerald-50 text-emerald-800 ring-1 ring-inset ring-emerald-600/20',
  warn: 'bg-amber-50 text-amber-900 ring-1 ring-inset ring-amber-600/25',
  critical: 'bg-red-50 text-red-800 ring-1 ring-inset ring-red-600/25',
}

/** Ponto sólido — o canal de forma que acompanha a cor. */
export const SEVERITY_DOT: Record<Severity, string> = {
  ok: 'bg-emerald-600',
  warn: 'bg-amber-500',
  critical: 'bg-red-600',
}

/** Faixa lateral esquerda de um card/linha — severidade legível a 2 metros. */
export const SEVERITY_BAR: Record<Severity, string> = {
  ok: 'border-l-4 border-l-emerald-500',
  warn: 'border-l-4 border-l-amber-500',
  critical: 'border-l-4 border-l-red-600',
}

/** Só o texto — para números que carregam a própria severidade. */
export const SEVERITY_TEXT: Record<Severity, string> = {
  ok: 'text-emerald-700',
  warn: 'text-amber-700',
  critical: 'text-red-700',
}

/** Hex para marcas SVG (o Tailwind não alcança atributos `fill`/`stroke`). */
export const SEVERITY_HEX: Record<Severity, string> = {
  ok: '#0ca30c',
  warn: '#d98a00',
  critical: '#d03b3b',
}

/**
 * Cores de série dos gráficos. Identidade da marca — nunca significado.
 * `muted` é a série de contexto (período anterior, fundo da sparkline).
 */
export const CHART = {
  primary: '#1B3E8A', // brand.blue
  primarySoft: 'rgba(27, 62, 138, 0.10)', // área preenchida ~10%
  accent: '#F47920', // brand.orange — segunda série / destaque do último ponto
  accentSoft: 'rgba(244, 121, 32, 0.12)',
  grid: '#E5E7EB', // gray-200, recessivo
  axis: '#9CA3AF', // gray-400
  surface: '#FFFFFF',
} as const

/**
 * Delta bom ou ruim depende da métrica: vendas subindo é bom, taxa de erro
 * subindo é ruim. `higherIsBetter` decide a cor — nunca o sinal sozinho.
 */
export function deltaSeverity(delta: number | null, higherIsBetter = true): Severity | null {
  if (delta === null || delta === 0) return null
  const good = higherIsBetter ? delta > 0 : delta < 0
  return good ? 'ok' : 'warn'
}
