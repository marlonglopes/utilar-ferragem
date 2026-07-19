import type { LedgerEntry } from '@/lib/adminTypes'

/**
 * Export do livro-razão para o contador.
 *
 * O cabeçalho e a formatação abaixo são o CONTRATO do arquivo: quando o
 * `GET /api/v1/admin/accounting/export?format=csv` existir no payment-service,
 * ele deve emitir exatamente estas colunas, nesta ordem, com este separador —
 * o contador não pode perceber diferença entre o arquivo gerado no cliente
 * (modo mock) e o do servidor.
 *
 * Escolhas deliberadas para o Excel brasileiro:
 * - **Separador `;`** — com `,` o Excel pt-BR quebra a coluna no decimal.
 * - **Decimal com vírgula** — idem.
 * - **BOM UTF-8 no início** — sem ele o Excel assume a codificação local e os
 *   acentos viram mojibake ("Estorno" vira "EstÃ´rno").
 * - **Data ISO `YYYY-MM-DD HH:mm`** — ordena como texto e não é ambígua.
 */

export const LEDGER_CSV_HEADER = [
  'data',
  'conta_codigo',
  'conta_nome',
  'natureza',
  'debito',
  'credito',
  'metodo',
  'pedido',
  'pedido_id',
  'pagamento_id',
  'psp_transacao',
  'historico',
] as const

const SEP = ';'

/** Centavos → "1234,56". Sem separador de milhar: planilha não quer. */
function csvMoney(cents: number): string {
  return (cents / 100).toFixed(2).replace('.', ',')
}

function csvDate(iso: string): string {
  // Sem `Date` para não depender do fuso da máquina: o contrato já é UTC ISO.
  return iso.slice(0, 16).replace('T', ' ')
}

/**
 * Escapa um campo. Aspas duplicadas e envelopadas quando o valor contém
 * separador, aspas ou quebra de linha — RFC 4180.
 */
function csvField(value: string | null | undefined): string {
  const s = value ?? ''
  if (s.includes(SEP) || s.includes('"') || s.includes('\n') || s.includes('\r')) {
    return `"${s.replace(/"/g, '""')}"`
  }
  return s
}

export function ledgerToCsv(entries: LedgerEntry[]): string {
  const lines = [LEDGER_CSV_HEADER.join(SEP)]
  for (const e of entries) {
    lines.push(
      [
        csvDate(e.occurredAt),
        csvField(e.accountCode),
        csvField(e.account),
        csvField(e.kind),
        csvMoney(e.debitCents),
        csvMoney(e.creditCents),
        csvField(e.method),
        csvField(e.orderNumber),
        csvField(e.orderId),
        csvField(e.paymentId),
        csvField(e.pspTransactionId),
        csvField(e.memo),
      ].join(SEP),
    )
  }
  // CRLF: é o que o Excel espera e o que a RFC manda.
  // O BOM vai como escape `\uFEFF`, não literal: caractere invisível no
  // arquivo-fonte confunde diff, lint e revisão.
  return `\uFEFF${lines.join('\r\n')}\r\n`
}

/**
 * Dispara o download no navegador via blob + object URL.
 *
 * O object URL é revogado no próximo tick: sem isso o blob (que pode ter
 * dezenas de MB de dados contábeis) fica vivo na memória da aba até o reload.
 */
export function downloadCsv(filename: string, csv: string): void {
  downloadBlob(filename, new Blob([csv], { type: 'text/csv;charset=utf-8' }))
}

/** Mesma mecânica, para um blob que já veio pronto do servidor. */
export function downloadBlob(filename: string, blob: Blob): void {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.style.display = 'none'
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

export function ledgerFilename(from: string, to: string): string {
  return `utilar-livro-razao_${from}_a_${to}.csv`
}
