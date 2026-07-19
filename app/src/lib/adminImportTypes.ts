/**
 * Contratos da ingestão de produtos — espelho fiel do catalog-service.
 *
 * Fonte da verdade: `services/catalog-service/internal/handler/admin_import.go`
 * e `services/catalog-service/internal/ingest/{profile,pipeline,commit}.go`.
 * Nada aqui é inventado: cada campo abaixo existe no JSON que o backend devolve.
 *
 * O pipeline é DELIBERADAMENTE em dois passos (dry-run → commit) e os tipos
 * refletem isso: `ImportPlan` é a PREVISÃO (nada foi escrito) e `CommitResult` é
 * o RESULTADO REAL. São tipos distintos de propósito — confundir os dois na tela
 * faria o operador acreditar que importou o que apenas foi simulado.
 */

// ---------------------------------------------------------------------------
// Campos e parsers (whitelist do backend)
// ---------------------------------------------------------------------------

/** Campos do domínio que a ingestão pode preencher. Whitelist de `profile.go`. */
export const IMPORT_FIELDS = [
  'sku',
  'name',
  'category',
  'price',
  'originalPrice',
  'cost',
  'stock',
  'brand',
  'description',
  'unitOfMeasure',
  'qtyStep',
  'barcode',
  'weightKg',
  'lengthCm',
  'widthCm',
  'heightCm',
  'supplierId',
  'supplierSku',
  'ncm',
  'cfop',
  'cest',
  'origem',
  'imageUrl',
  'status',
  'specs',
] as const

export type ImportField = (typeof IMPORT_FIELDS)[number]

export const IMPORT_PARSERS = [
  'text',
  'money_br',
  'number',
  'percent',
  'date',
  'code',
  'bool',
] as const

export type ImportParser = (typeof IMPORT_PARSERS)[number]

/**
 * Grau de certeza da sugestão automática.
 *
 * Existe para a tela DESTACAR o que precisa de conferência humana. Sem
 * graduação, 40 palpites com o mesmo peso visual são confirmados no automático
 * — que é exatamente como um "VLR CUSTO" mapeado em `price` chega ao catálogo.
 */
export type ImportConfidence = 'exact' | 'high' | 'low'

// ---------------------------------------------------------------------------
// Passo 1/2 — sugestão de mapeamento
// ---------------------------------------------------------------------------

export interface ColumnSuggestion {
  /** Cabeçalho como veio no arquivo — é o que o operador vê na planilha dele. */
  column: string
  field?: ImportField
  parser?: ImportParser
  confidence?: ImportConfidence
  /** Por que o sistema achou que essa coluna é preço. Torna a conferência informada. */
  matchedAlias?: string
  /** `false` = coluna desconhecida. Não é erro: fica sem mapear e é ignorada. */
  recognized: boolean
}

export interface SuggestResponse {
  filename: string
  format: string
  header: string[]
  columns: ColumnSuggestion[]
  /** Até 5 linhas de dados, chaveadas pelo cabeçalho. */
  sample: Record<string, string>[]
  summary: {
    columns: number
    recognized: number
    ignored: string[]
    needsReview: string[]
    rows: number
  }
  fields: string[]
}

// ---------------------------------------------------------------------------
// Perfil de mapeamento
// ---------------------------------------------------------------------------

export interface ColumnMapping {
  field: ImportField
  parser?: ImportParser
  /** Quando preenchido, a coluna vira uma chave dentro de `specs`. */
  specKey?: string
}

/** Travas de segurança do lote. Perfil omisso NÃO desliga trava nenhuma. */
export interface ImportOptions {
  maxPriceDropPct?: number
  maxPriceRisePct?: number
  archiveMissing?: boolean
  /** Padrão false — produto importado entra como rascunho. A UI não oferece ligar. */
  publishOnImport?: boolean
  defaultCategory?: string
  defaultUnit?: string
  headerRow?: number
  sheet?: string
}

export interface ProfileInput {
  name: string
  description?: string
  kind?: string
  columns: Record<string, ColumnMapping>
  defaults?: Partial<Record<ImportField, string>>
  options?: ImportOptions
}

export interface ImportProfile {
  id: string
  name: string
  version: number
  kind?: string
  description?: string
  createdAt?: string
}

// ---------------------------------------------------------------------------
// Passo 3 — dry-run
// ---------------------------------------------------------------------------

export type ImportAction = 'create' | 'update' | 'skip' | 'review' | 'reject'

export interface RowIssue {
  field?: string
  message: string
}

export interface ImportRow {
  /** Linha NA PLANILHA. É assim que a pessoa acha o erro no Excel. */
  rowNumber: number
  raw?: Record<string, string>
  mapped?: Record<string, unknown>
  sku?: string
  action: ImportAction
  errors?: RowIssue[] | null
  warnings?: RowIssue[] | null
  productId?: string
  /** Diff de preço — o que a revisão mostra em âmbar. */
  oldPrice?: number
  newPrice?: number
  dropPct?: number
}

export interface ImportSummary {
  total: number
  creates: number
  updates: number
  skips: number
  reviews: number
  rejects: number
  toArchive: number
}

export interface ImportPlan {
  batchId: string
  status: string
  dryRun: boolean
  summary: ImportSummary
  rows: ImportRow[]
  warnings?: string[] | null
}

// ---------------------------------------------------------------------------
// Passo 4 — commit
// ---------------------------------------------------------------------------

/**
 * O que REALMENTE aconteceu. Note `failed`: o backend usa uma transação por
 * linha justamente para que uma linha ruim não aborte o lote, então um commit
 * pode ser parcial — e a tela precisa mostrar isso, não a previsão do dry-run.
 */
export interface CommitResult {
  created: number
  updated: number
  skipped: number
  archived: number
  rejected: number
  /** Retidos para revisão humana — NÃO foram aplicados. */
  held: number
  failed: number
  errors?: RowIssue[] | null
}

export interface CommitResponse {
  batchId: string
  status: string
  result: CommitResult
}

// ---------------------------------------------------------------------------
// Histórico
// ---------------------------------------------------------------------------

export type BatchStatus = 'uploaded' | 'staged' | 'validated' | 'committed' | 'failed'

export interface ImportBatch {
  id: string
  filename: string
  format: string
  status: BatchStatus | string
  supplierId?: string
  profile?: string
  profileVersion?: number
  summary: Omit<ImportSummary, 'skips'> & { skips?: number }
  createdBy?: string
  createdAt: string
  committedAt?: string
  error?: string
}
