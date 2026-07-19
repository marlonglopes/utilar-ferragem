import type { Severity } from '@/lib/adminTypes'
import type {
  ImportAction,
  ImportConfidence,
  ImportField,
  ImportRow,
  ImportSummary,
} from '@/lib/adminImportTypes'

/**
 * Rótulos, severidades e validações da tela de importação.
 *
 * Fica separado dos componentes porque é a parte da tela que precisa de teste
 * unitário direto: é aqui que mora a decisão de "isto merece alarme" — e essa
 * decisão é o que a tela de revisão existe para tomar.
 *
 * ⚠️ Preço aqui é REAL (float), não centavo. O pipeline de ingestão trabalha em
 * `NUMERIC(12,2)` do Postgres e o JSON traz `34.9`, não `3490`. Usar
 * `formatCents` destes valores mostraria "R$ 0,35" no lugar de "R$ 34,90" — por
 * isso existe um formatador próprio aqui em vez de reaproveitar o do razão.
 */

// ---------------------------------------------------------------------------
// Dinheiro (reais, não centavos)
// ---------------------------------------------------------------------------

const BRL = new Intl.NumberFormat('pt-BR', {
  style: 'currency',
  currency: 'BRL',
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})

export function formatReais(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—'
  return BRL.format(value)
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return '—'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toLocaleString('pt-BR', { maximumFractionDigits: 0 })} KB`
  return `${(bytes / (1024 * 1024)).toLocaleString('pt-BR', { maximumFractionDigits: 1 })} MB`
}

// ---------------------------------------------------------------------------
// Ações
// ---------------------------------------------------------------------------

export const ACTION_LABEL: Record<ImportAction, string> = {
  create: 'Criar',
  update: 'Atualizar',
  skip: 'Sem mudança',
  review: 'Retido',
  reject: 'Rejeitado',
}

/**
 * Severidade da ação. `review` é ÂMBAR e não vermelho de propósito: a linha não
 * está errada, está esperando um humano — tratá-la como erro faria o operador
 * ignorá-la junto com as rejeições.
 */
export const ACTION_SEVERITY: Record<ImportAction, Severity | null> = {
  create: 'ok',
  update: null,
  skip: null,
  review: 'warn',
  reject: 'critical',
}

/** Classe do chip da ação. Cor + palavra: a cor sozinha não é canal suficiente. */
export const ACTION_CHIP: Record<ImportAction, string> = {
  create: 'bg-emerald-50 text-emerald-800 ring-1 ring-inset ring-emerald-600/20',
  update: 'bg-blue-50 text-blue-800 ring-1 ring-inset ring-blue-600/20',
  skip: 'bg-gray-100 text-gray-600 ring-1 ring-inset ring-gray-300',
  review: 'bg-amber-50 text-amber-900 ring-1 ring-inset ring-amber-600/25',
  reject: 'bg-red-50 text-red-800 ring-1 ring-inset ring-red-600/25',
}

// ---------------------------------------------------------------------------
// Confiança do mapeamento
// ---------------------------------------------------------------------------

export const CONFIDENCE_LABEL: Record<ImportConfidence, string> = {
  exact: 'Exato',
  high: 'Provável',
  low: 'Palpite',
}

export const CONFIDENCE_HINT: Record<ImportConfidence, string> = {
  exact: 'O cabeçalho é exatamente um nome conhecido deste campo.',
  high: 'O nome conhecido aparece dentro do cabeçalho. Confira o dado da amostra.',
  low: 'Casou por abreviação. Confirme olhando a amostra antes de aceitar.',
}

/**
 * `low` e não-reconhecida são onde o humano erra — precisam saltar aos olhos.
 * `exact` é deliberadamente discreto: destacar tudo é não destacar nada.
 */
export function confidenceSeverity(
  confidence: ImportConfidence | undefined,
  recognized: boolean,
): Severity | null {
  if (!recognized) return 'warn'
  if (confidence === 'low') return 'critical'
  if (confidence === 'high') return 'warn'
  return null
}

/** Precisa de conferência explícita do operador antes de virar perfil. */
export function needsHumanReview(confidence: ImportConfidence | undefined, recognized: boolean): boolean {
  return recognized && confidence !== 'exact'
}

// ---------------------------------------------------------------------------
// Variação de preço — o motivo de existir a tela de revisão
// ---------------------------------------------------------------------------

/** Mesmos padrões do backend (`ingest.DefaultMaxPrice{Drop,Rise}Pct`). */
export const DEFAULT_MAX_DROP_PCT = 30
export const DEFAULT_MAX_RISE_PCT = 300

/** Variação relativa em PONTOS PERCENTUAIS assinados. -99.9 = quase zerou. */
export function priceChangePct(oldPrice?: number, newPrice?: number): number | null {
  if (oldPrice === undefined || newPrice === undefined) return null
  if (!Number.isFinite(oldPrice) || !Number.isFinite(newPrice) || oldPrice <= 0) return null
  return ((newPrice - oldPrice) / oldPrice) * 100
}

/**
 * Quão alarmante é a mudança de preço.
 *
 * O erro de vírgula (`1.234,56` lido como `1,23`) é o modo de falha mais caro do
 * catálogo e aparece exatamente como uma queda de ~99%. Por isso a queda tem
 * limite muito mais apertado que a subida: subir 200% é suspeito, cair 99% é
 * quase certamente um bug de parsing que zera a margem de um SKU inteiro.
 */
export function priceChangeSeverity(
  oldPrice?: number,
  newPrice?: number,
  maxDropPct = DEFAULT_MAX_DROP_PCT,
  maxRisePct = DEFAULT_MAX_RISE_PCT,
): Severity | null {
  const pct = priceChangePct(oldPrice, newPrice)
  if (pct === null || pct === 0) return null
  if (pct < 0) {
    const drop = Math.abs(pct)
    if (drop >= maxDropPct) return 'critical'
    if (drop >= maxDropPct / 3) return 'warn'
    return null
  }
  if (pct >= maxRisePct) return 'critical'
  if (pct >= maxDropPct) return 'warn'
  return null
}

/** "−99,9%" / "+12,4%". Sinal explícito: "12,4%" não diz se subiu ou caiu. */
export function formatPriceDelta(oldPrice?: number, newPrice?: number): string {
  const pct = priceChangePct(oldPrice, newPrice)
  if (pct === null) return '—'
  const sign = pct > 0 ? '+' : pct < 0 ? '−' : ''
  return `${sign}${Math.abs(pct).toLocaleString('pt-BR', {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  })}%`
}

// ---------------------------------------------------------------------------
// Contadores
// ---------------------------------------------------------------------------

export interface CounterTile {
  key: keyof ImportSummary
  label: string
  value: number
  severity: Severity | null
  hint: string
}

/**
 * Os contadores do topo da revisão, na ordem em que importam para a decisão.
 *
 * `toArchive` vem por último e sempre visível mesmo em zero: arquivamento por
 * ausência é a operação mais silenciosamente destrutiva do lote, e um contador
 * que só aparece quando é diferente de zero não ensina que ele existe.
 */
export function summaryCounters(summary: ImportSummary): CounterTile[] {
  return [
    {
      key: 'creates',
      label: 'Criar',
      value: summary.creates,
      severity: summary.creates > 0 ? 'ok' : null,
      hint: 'produtos novos, como rascunho',
    },
    {
      key: 'updates',
      label: 'Atualizar',
      value: summary.updates,
      severity: null,
      hint: 'SKUs que já existem',
    },
    {
      key: 'reviews',
      label: 'Reter',
      value: summary.reviews,
      severity: summary.reviews > 0 ? 'warn' : null,
      hint: 'variação de preço fora do limite',
    },
    {
      key: 'rejects',
      label: 'Rejeitar',
      value: summary.rejects,
      severity: summary.rejects > 0 ? 'critical' : null,
      hint: 'linhas inválidas, não entram',
    },
    {
      key: 'skips',
      label: 'Sem mudança',
      value: summary.skips,
      severity: null,
      hint: 'idênticas ao catálogo',
    },
    {
      key: 'toArchive',
      label: 'Arquivar',
      value: summary.toArchive,
      severity: summary.toArchive > 0 ? 'warn' : null,
      hint: 'sumiram da planilha (nunca apagados)',
    },
  ]
}

/** Linhas que exigem decisão primeiro. `skip` num lote de 4.000 é ruído. */
export function sortRowsByAttention(rows: ImportRow[]): ImportRow[] {
  const weight: Record<ImportAction, number> = { reject: 0, review: 1, update: 2, create: 3, skip: 4 }
  return [...rows].sort((a, b) => {
    const d = (weight[a.action] ?? 9) - (weight[b.action] ?? 9)
    return d !== 0 ? d : a.rowNumber - b.rowNumber
  })
}

// ---------------------------------------------------------------------------
// Validação do arquivo — upload é entrada hostil
// ---------------------------------------------------------------------------

/** Mesmo teto do backend (`ingest.MaxFileBytes`). */
export const MAX_UPLOAD_BYTES = 32 * 1024 * 1024

export const ACCEPTED_EXTENSIONS = ['.csv', '.xlsx', '.json'] as const

/**
 * Valida antes de subir.
 *
 * O backend já limita — esta checagem não é a defesa, é a cortesia: subir 40 MB
 * por uma rede de loja para receber um 400 é minutos perdidos, e a mensagem
 * genérica do servidor não diz qual arquivo o operador deveria ter escolhido.
 * A defesa continua sendo o servidor; aqui só melhoramos o diagnóstico.
 */
export function validateImportFile(file: File): string | null {
  const name = file.name.toLowerCase()
  const ext = ACCEPTED_EXTENSIONS.find((e) => name.endsWith(e))
  if (!ext) {
    return `Formato não aceito. Envie um arquivo ${ACCEPTED_EXTENSIONS.join(', ')} — o que veio foi "${file.name}".`
  }
  if (file.size === 0) {
    return 'O arquivo está vazio (0 bytes). Verifique se o download da planilha terminou.'
  }
  if (file.size > MAX_UPLOAD_BYTES) {
    return `Arquivo de ${formatBytes(file.size)} excede o limite de ${formatBytes(MAX_UPLOAD_BYTES)}. Divida a planilha ou envie em CSV, que é bem menor que XLSX.`
  }
  return null
}

// ---------------------------------------------------------------------------
// Campos do domínio
// ---------------------------------------------------------------------------

export const FIELD_LABEL: Record<ImportField, string> = {
  sku: 'SKU (código)',
  name: 'Nome do produto',
  category: 'Categoria',
  price: 'Preço de venda',
  originalPrice: 'Preço "de" (cheio)',
  cost: 'Custo',
  stock: 'Estoque',
  brand: 'Marca',
  description: 'Descrição',
  unitOfMeasure: 'Unidade de medida',
  qtyStep: 'Passo de quantidade',
  barcode: 'Código de barras (EAN)',
  weightKg: 'Peso (kg)',
  lengthCm: 'Comprimento (cm)',
  widthCm: 'Largura (cm)',
  heightCm: 'Altura (cm)',
  supplierId: 'Fornecedor',
  supplierSku: 'Código do fornecedor',
  ncm: 'NCM',
  cfop: 'CFOP',
  cest: 'CEST',
  origem: 'Origem (fiscal)',
  imageUrl: 'URL da imagem',
  status: 'Status',
  specs: 'Ficha técnica (specs)',
}

/** Campos cujo erro de mapeamento custa dinheiro imediato. */
export const CRITICAL_FIELDS: ImportField[] = ['price', 'cost', 'sku']

export function fieldLabel(field?: string): string {
  if (!field) return 'Ignorar esta coluna'
  return FIELD_LABEL[field as ImportField] ?? field
}
