import type {
  ColumnMapping,
  ColumnSuggestion,
  CommitResult,
  ImportBatch,
  ImportConfidence,
  ImportField,
  ImportPlan,
  ImportRow,
  ImportSummary,
  SuggestResponse,
} from '@/lib/adminImportTypes'
import { IMPORT_FIELDS } from '@/lib/adminImportTypes'

/**
 * Modo demonstração da ingestão.
 *
 * Sem catalog-service configurado, a tela precisa funcionar de ponta a ponta —
 * é assim que o dono vai vê-la antes de existir servidor. Duas decisões:
 *
 * 1. **O CSV enviado é lido de verdade**, no navegador. Cabeçalho, amostra e
 *    contagem de linhas saem do arquivo real, não de uma constante. Uma tela de
 *    mapeamento demonstrada com colunas fictícias não prova nada sobre a
 *    planilha do fornecedor — e é justamente a planilha real que se quer testar
 *    (`scripts/ingestao/exemplo-fornecedor.csv` tem os problemas plantados).
 * 2. **As regras de validação são um ESPELHO REDUZIDO do backend**, não a
 *    verdade. Cobrem os casos que a tela precisa saber desenhar (SKU ausente,
 *    categoria inexistente, fórmula de Excel, preço abaixo do custo, queda
 *    grande de preço). O julgamento real é sempre do servidor.
 *
 * ⚠️ Nada daqui roda quando `VITE_CATALOG_URL` está configurado.
 */

// ---------------------------------------------------------------------------
// Leitura de CSV no navegador
// ---------------------------------------------------------------------------

interface ParsedTable {
  header: string[]
  rows: string[][]
}

/** Delimitador por contagem no cabeçalho — fornecedor brasileiro manda `;`. */
function detectDelimiter(line: string): string {
  const counts = [';', ',', '\t'].map((d) => [d, line.split(d).length] as const)
  return counts.sort((a, b) => b[1] - a[1])[0][0]
}

function splitCsvLine(line: string, delim: string): string[] {
  const out: string[] = []
  let cur = ''
  let quoted = false
  for (let i = 0; i < line.length; i++) {
    const ch = line[i]
    if (quoted) {
      if (ch === '"' && line[i + 1] === '"') {
        cur += '"'
        i++
      } else if (ch === '"') {
        quoted = false
      } else {
        cur += ch
      }
      continue
    }
    // Aspas SÓ delimitam quando abrem o campo. Sem esta condição, a polegada em
    // `Vergalhão CA-50 3/8" barra 12m` abre um campo citado no meio da linha e
    // engole o resto — o preço some e a linha vira "preço ausente". Catálogo de
    // ferragem é cheio de 1/2", 3/8", 3/4": é o caso comum, não o exótico.
    if (ch === '"' && cur === '') quoted = true
    else if (ch === delim) {
      out.push(cur.trim())
      cur = ''
    } else cur += ch
  }
  out.push(cur.trim())
  return out
}

export function parseCsv(text: string): ParsedTable {
  // BOM do Excel: sem remover, o primeiro cabeçalho vira "\ufeffCODIGO" e nada
  // mapeia — o modo de falha mais bobo e mais frequente de CSV brasileiro.
  const clean = text.replace(/^\ufeff/, '')
  const lines = clean.split(/\r?\n/).filter((l) => l.trim() !== '')
  if (lines.length === 0) return { header: [], rows: [] }
  const delim = detectDelimiter(lines[0])
  return {
    header: splitCsvLine(lines[0], delim),
    rows: lines.slice(1).map((l) => splitCsvLine(l, delim)),
  }
}

async function readTable(file: File): Promise<ParsedTable> {
  const name = file.name.toLowerCase()
  if (name.endsWith('.csv')) {
    return parseCsv(await file.text())
  }
  if (name.endsWith('.json')) {
    try {
      const data = JSON.parse(await file.text()) as Record<string, unknown>[]
      if (Array.isArray(data) && data.length > 0) {
        const header = Object.keys(data[0])
        return { header, rows: data.map((o) => header.map((h) => String(o[h] ?? ''))) }
      }
    } catch {
      /* cai no fallback abaixo */
    }
  }
  // XLSX não é legível sem backend. Em vez de fingir que leu, devolvemos a
  // planilha-exemplo — e a tela deixa claro que está em modo demonstração.
  return FALLBACK_TABLE
}

const FALLBACK_TABLE: ParsedTable = {
  header: ['CODIGO', 'DESCRICAO DO PRODUTO', 'GRUPO', 'MARCA', 'UN', 'VLR VENDA', 'VLR CUSTO', 'ESTOQUE'],
  rows: [
    ['FORN-0001', 'Cimento CP-II-E-32 saco 50kg', 'construcao', 'Votoran', 'saco', 'R$ 34,90', 'R$ 27,10', '240'],
    ['FORN-0002', 'Argamassa AC-III 20kg', 'construcao', 'Quartzolit', 'saco', '28,40', '21,90', '180'],
    ['FORN-0004', 'Furadeira de Impacto 750W', 'ferramentas', 'Bosch', 'un', 'R$ 429,90', 'R$ 296,00', '12'],
    ['FORN-0014', 'Categoria que não existe', 'departamento-inventado', 'Marca Y', 'un', 'R$ 10,00', 'R$ 5,00', '5'],
    ['', 'Linha sem código', 'ferramentas', 'Marca Z', 'un', 'R$ 99,00', 'R$ 70,00', '3'],
    ['FORN-0017', 'Cimento CP-V ARI saco 50kg', 'construcao', 'Votoran', 'saco', 'R$ 1,23', 'R$ 31,40', '80'],
  ],
}

// ---------------------------------------------------------------------------
// Sugestão de mapeamento (espelho reduzido de ingest.SuggestColumns)
// ---------------------------------------------------------------------------

const ALIASES: Partial<Record<ImportField, string[]>> = {
  sku: ['sku', 'codigo', 'cod', 'referencia', 'ref'],
  name: ['descricao do produto', 'nome', 'produto', 'descricao', 'item'],
  category: ['categoria', 'grupo', 'familia', 'departamento', 'secao'],
  price: ['vlr venda', 'preco', 'preco venda', 'valor', 'pvenda'],
  cost: ['vlr custo', 'custo', 'preco custo', 'valor de custo'],
  stock: ['estoque', 'qtd', 'quantidade', 'saldo', 'qtde'],
  brand: ['marca', 'fabricante'],
  unitOfMeasure: ['un', 'unidade', 'und', 'um', 'medida'],
  barcode: ['ean', 'gtin', 'codigo de barras', 'barcode'],
  weightKg: ['peso', 'peso kg'],
  ncm: ['ncm'],
  cfop: ['cfop'],
  description: ['detalhes', 'observacao', 'obs'],
  supplierSku: ['codigo fornecedor', 'ref fornecedor'],
  imageUrl: ['imagem', 'foto', 'url imagem'],
}

const PARSER_BY_FIELD: Partial<Record<ImportField, ColumnMapping['parser']>> = {
  price: 'money_br',
  cost: 'money_br',
  originalPrice: 'money_br',
  stock: 'number',
  weightKg: 'number',
  qtyStep: 'number',
  sku: 'code',
  barcode: 'code',
  ncm: 'code',
  cfop: 'code',
}

function normalizeHeader(s: string): string {
  return s
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, ' ')
    .trim()
}

/**
 * Três passadas por nível de confiança, e não uma por coluna — igual ao
 * backend. Se a primeira coluna casasse fraco e tomasse o campo, a coluna
 * seguinte que casava exatamente ficaria sem mapeamento: a ordem das colunas no
 * arquivo não deve decidir a qualidade do mapeamento.
 */
export function suggestColumns(header: string[]): ColumnSuggestion[] {
  const cands: { field: ImportField; alias: string }[] = []
  for (const field of IMPORT_FIELDS) {
    for (const a of ALIASES[field] ?? []) cands.push({ field, alias: normalizeHeader(a) })
  }
  cands.sort((a, b) => b.alias.length - a.alias.length || a.alias.localeCompare(b.alias))

  const used = new Set<ImportField>()
  const best: ({ field: ImportField; conf: ImportConfidence; alias: string } | null)[] = header.map(
    () => null,
  )

  for (let pass = 0; pass < 3; pass++) {
    header.forEach((h, i) => {
      if (best[i]) return
      const n = normalizeHeader(h)
      if (!n) return
      for (const c of cands) {
        if (used.has(c.field)) continue
        let conf: ImportConfidence | null = null
        if (pass === 0 && n === c.alias) conf = 'exact'
        else if (pass === 1 && new RegExp(`\\b${c.alias.replace(/\s+/g, '\\s+')}\\b`).test(n)) conf = 'high'
        else if (pass === 2 && c.alias.length >= 4 && n.replace(/\s/g, '').includes(c.alias.replace(/\s/g, '')))
          conf = 'low'
        if (conf) {
          best[i] = { field: c.field, conf, alias: c.alias }
          used.add(c.field)
          return
        }
      }
    })
  }

  return header.map((column, i) => {
    const m = best[i]
    if (!m) return { column, recognized: false }
    return {
      column,
      field: m.field,
      parser: PARSER_BY_FIELD[m.field] ?? 'text',
      confidence: m.conf,
      matchedAlias: m.alias,
      recognized: true,
    }
  })
}

export async function mockSuggest(file: File): Promise<SuggestResponse> {
  const table = await readTable(file)
  const columns = suggestColumns(table.header)
  const sample = table.rows.slice(0, 5).map((r) => {
    const o: Record<string, string> = {}
    table.header.forEach((h, i) => {
      o[h] = r[i] ?? ''
    })
    return o
  })
  return {
    filename: file.name,
    format: file.name.toLowerCase().endsWith('.csv') ? 'csv' : 'xlsx',
    header: table.header,
    columns,
    sample,
    summary: {
      columns: table.header.length,
      recognized: columns.filter((c) => c.recognized).length,
      ignored: columns.filter((c) => !c.recognized).map((c) => c.column),
      needsReview: columns.filter((c) => c.recognized && c.confidence !== 'exact').map((c) => c.column),
      rows: table.rows.length,
    },
    fields: [...IMPORT_FIELDS],
  }
}

// ---------------------------------------------------------------------------
// Dry-run
// ---------------------------------------------------------------------------

/** As 8 categorias do seed. `departamento-inventado` cai fora — de propósito. */
const KNOWN_CATEGORIES = [
  'construcao',
  'ferramentas',
  'eletrica',
  'hidraulica',
  'pintura',
  'seguranca',
  'fixacao',
  'jardim',
]

/** `R$ 1.234,56` → 1234.56. Vazio, `-`, `N/A` e `#REF!` viram null, não zero. */
export function parseMoneyBR(raw: string): number | null {
  const s = (raw ?? '').trim()
  if (s === '' || s === '-' || /^n\/?a$/i.test(s) || s.startsWith('#')) return null
  const cleaned = s.replace(/[R$\s\u00a0]/gi, '').replace(/\./g, '').replace(',', '.')
  const n = Number(cleaned)
  return Number.isFinite(n) ? n : null
}

/** Célula que começa com `=`, `+`, `-` ou `@` é vetor de injeção de fórmula. */
function looksLikeFormula(s: string): boolean {
  return /^[=+@]/.test(s.trim()) || /^-[a-zA-Z]/.test(s.trim())
}

export async function mockPlan(file: File, mapping: Record<string, ColumnMapping>): Promise<ImportPlan> {
  const table = await readTable(file)
  const colOf = (field: ImportField): number => {
    const entry = Object.entries(mapping).find(([, m]) => m.field === field)
    return entry ? table.header.indexOf(entry[0]) : -1
  }
  const idx = {
    sku: colOf('sku'),
    name: colOf('name'),
    category: colOf('category'),
    price: colOf('price'),
    cost: colOf('cost'),
  }

  const rows: ImportRow[] = table.rows.map((cells, i) => {
    const at = (j: number) => (j >= 0 ? (cells[j] ?? '') : '')
    // +2: a linha 1 é o cabeçalho e o Excel conta a partir de 1. É esse número
    // que a pessoa digita no "ir para" da planilha.
    const rowNumber = i + 2
    const raw: Record<string, string> = {}
    table.header.forEach((h, j) => {
      raw[h] = cells[j] ?? ''
    })

    const sku = at(idx.sku)
    const name = at(idx.name)
    const category = at(idx.category)
    const price = parseMoneyBR(at(idx.price))
    const cost = parseMoneyBR(at(idx.cost))
    const base: ImportRow = { rowNumber, sku, raw, mapped: { sku, name, category, price, cost }, action: 'create' }

    if (!sku.trim()) {
      return {
        ...base,
        action: 'reject',
        errors: [{ field: 'sku', message: 'SKU ausente — sem código não há como identificar o produto; a linha não é importada' }],
      }
    }
    if (looksLikeFormula(name)) {
      return {
        ...base,
        action: 'reject',
        errors: [{ field: 'name', message: 'conteúdo com aparência de fórmula de planilha — recusado por segurança' }],
      }
    }
    if (price === null) {
      return {
        ...base,
        action: 'reject',
        errors: [{ field: 'price', message: 'preço ausente ou ilegível' }],
      }
    }
    if (idx.category >= 0 && !KNOWN_CATEGORIES.includes(category)) {
      return {
        ...base,
        action: 'reject',
        errors: [{ field: 'category', message: `categoria "${category}" não existe no catálogo` }],
      }
    }
    if (cost !== null && price < cost) {
      // Preço abaixo do custo é quase sempre erro de vírgula. Retém, não rejeita:
      // pode ser promoção legítima, e essa decisão é humana.
      return {
        ...base,
        action: 'review',
        oldPrice: Number((cost * 1.28).toFixed(2)),
        newPrice: price,
        warnings: [
          { field: 'price', message: `preço (${price}) abaixo do custo (${cost}) — confira separador decimal` },
        ],
      }
    }
    // Um terço dos SKUs "já existe" no catálogo, para a demonstração ter
    // atualização e diff de preço, não só criação.
    if (i % 3 === 1) {
      const oldPrice = Number((price * 1.09).toFixed(2))
      return { ...base, action: 'update', oldPrice, newPrice: price }
    }
    return base
  })

  const count = (a: string) => rows.filter((r) => r.action === a).length
  const summary: ImportSummary = {
    total: rows.length,
    creates: count('create'),
    updates: count('update'),
    skips: count('skip'),
    reviews: count('review'),
    rejects: count('reject'),
    toArchive: 0,
  }

  return {
    batchId: `demo-${Date.now().toString(36)}`,
    status: 'validated',
    dryRun: true,
    summary,
    rows,
    warnings: ['Modo demonstração: nenhum servidor foi contatado e nada será gravado.'],
  }
}

/**
 * Resultado do commit no mock: derivado do plano, sem inventar falha.
 *
 * `held` recebe os retidos porque retido NÃO é aplicado — repetir a previsão do
 * dry-run como se fosse resultado é exatamente o erro que a tela deve evitar.
 */
export function mockCommit(plan: ImportPlan): CommitResult {
  return {
    created: plan.summary.creates,
    updated: plan.summary.updates,
    skipped: plan.summary.skips,
    archived: plan.summary.toArchive,
    rejected: plan.summary.rejects,
    held: plan.summary.reviews,
    failed: 0,
  }
}

export function mockBatches(): ImportBatch[] {
  return [
    {
      id: '9f1c2a44-1111-4a2b-9c31-0a1b2c3d4e5f',
      filename: 'tabela-fornecedor-julho.csv',
      format: 'csv',
      status: 'committed',
      supplierId: 'FORN-CENTRAL',
      profile: 'Fornecedor Central',
      profileVersion: 3,
      summary: { total: 412, creates: 38, updates: 351, rejects: 12, reviews: 11, toArchive: 0 },
      createdBy: 'Marlon (dono)',
      createdAt: '2026-07-16T09:12:00Z',
      committedAt: '2026-07-16T09:31:00Z',
    },
    {
      id: '9f1c2a44-2222-4a2b-9c31-0a1b2c3d4e5f',
      filename: 'reposicao-eletrica.xlsx',
      format: 'xlsx',
      status: 'validated',
      supplierId: 'FORN-ELETRO',
      profile: 'Distribuidora Elétrica',
      profileVersion: 1,
      // Lote conferido e NUNCA aprovado. É o caso mais traiçoeiro do histórico:
      // o operador acha que importou e nada foi aplicado.
      summary: { total: 96, creates: 12, updates: 71, rejects: 5, reviews: 8, toArchive: 0 },
      createdBy: 'Marlon (dono)',
      createdAt: '2026-07-17T14:40:00Z',
    },
    {
      id: '9f1c2a44-3333-4a2b-9c31-0a1b2c3d4e5f',
      filename: 'precos.xlsx',
      format: 'xlsx',
      status: 'failed',
      profile: 'Fornecedor Central',
      profileVersion: 3,
      summary: { total: 0, creates: 0, updates: 0, rejects: 0, reviews: 0, toArchive: 0 },
      createdBy: 'Marlon (dono)',
      createdAt: '2026-07-15T11:02:00Z',
      error: 'não foi possível ler o arquivo: aba "Plan1" não encontrada',
    },
  ]
}
