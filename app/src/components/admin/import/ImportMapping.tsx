import { AlertTriangle, ArrowRight, EyeOff } from 'lucide-react'
import { cn } from '@/lib/cn'
import { SEVERITY_BAR, SEVERITY_PILL } from '@/components/admin/tokens'
import {
  CONFIDENCE_HINT,
  CONFIDENCE_LABEL,
  CRITICAL_FIELDS,
  confidenceSeverity,
  fieldLabel,
} from '@/lib/adminImportFormat'
import { IMPORT_FIELDS, IMPORT_PARSERS } from '@/lib/adminImportTypes'
import type { ImportField, ImportParser, SuggestResponse } from '@/lib/adminImportTypes'
import type { MappingDraft } from '@/hooks/useImportWizard'

/**
 * Passo 2 — conferir o de/para das colunas.
 *
 * Três decisões que fazem esta tela funcionar:
 *
 * 1. **A amostra de dados fica embaixo de cada coluna.** Uma coluna chamada
 *    "VALOR" pode ser preço ou custo, e o cabeçalho não desempata — só o dado
 *    desempata. Mapear custo em `price` é o desastre nº 1 da ingestão e o único
 *    remédio é o operador ver o número antes de confirmar.
 * 2. **A ordem é a do ARQUIVO, não a da confiança.** O operador confere com a
 *    planilha aberta ao lado; reordenar quebraria a correspondência visual. O
 *    destaque de confiança carrega o peso, e um aviso no topo conta quantas
 *    precisam de olho.
 * 3. **Confiança baixa e coluna não reconhecida têm faixa lateral**, não só cor
 *    de texto: é o que sobrevive à impressão e ao daltonismo, e é onde o humano
 *    erra.
 */
export function ImportMapping({
  suggestion,
  mapping,
  profileName,
  supplierId,
  unmappedCritical,
  onFieldChange,
  onParserChange,
  onProfileNameChange,
  onSupplierIdChange,
}: {
  suggestion: SuggestResponse
  mapping: MappingDraft
  profileName: string
  supplierId: string
  unmappedCritical: ImportField[]
  onFieldChange: (column: string, field: ImportField | '') => void
  onParserChange: (column: string, parser: ImportParser) => void
  onProfileNameChange: (name: string) => void
  onSupplierIdChange: (id: string) => void
}) {
  const needsReview = suggestion.columns.filter(
    (c) => !c.recognized || (c.confidence && c.confidence !== 'exact'),
  )

  return (
    <div className="p-3 sm:p-4">
      {needsReview.length > 0 && (
        <div className="mb-4 flex items-start gap-2 rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-amber-50/60 p-3">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-700" aria-hidden="true" />
          <p className="text-xs leading-relaxed text-gray-700">
            <strong className="text-amber-900">
              {needsReview.length} de {suggestion.columns.length} colunas precisam de conferência.
            </strong>{' '}
            O sistema sugere pelo nome do cabeçalho — quem decide é você. Olhe a amostra de dados
            embaixo de cada coluna antes de aceitar, principalmente em preço e custo.
          </p>
        </div>
      )}

      {unmappedCritical.length > 0 && (
        <div className="mb-4 flex items-start gap-2 rounded-md border border-gray-200 border-l-4 border-l-red-600 bg-red-50 p-3">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-red-700" aria-hidden="true" />
          <p className="text-xs leading-relaxed text-red-800">
            <strong>Sem mapeamento para: {unmappedCritical.map(fieldLabel).join(', ')}.</strong> Sem
            SKU nenhuma linha pode ser identificada e o lote inteiro é rejeitado — confira antes de
            simular.
          </p>
        </div>
      )}

      <div className="space-y-2">
        {suggestion.columns.map((col) => {
          const current = mapping[col.column] ?? null
          const severity = confidenceSeverity(col.confidence, col.recognized)
          const isCritical = current && CRITICAL_FIELDS.includes(current.field)
          const samples = suggestion.sample
            .map((row) => row[col.column])
            .filter((v) => v !== undefined && v !== '')

          return (
            <div
              key={col.column}
              data-testid={`mapping-row-${col.column}`}
              data-confidence={col.recognized ? (col.confidence ?? 'exact') : 'unrecognized'}
              className={cn(
                'rounded-md border border-gray-200 bg-white p-3',
                severity && SEVERITY_BAR[severity],
                !current && 'opacity-70',
              )}
            >
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
                {/* Lado da planilha */}
                <div className="min-w-0 flex-1">
                  <p className="truncate font-mono text-sm font-semibold text-gray-900">
                    {col.column}
                  </p>
                  <div className="mt-1 flex flex-wrap items-center gap-1.5">
                    {!col.recognized ? (
                      <span className="inline-flex items-center gap-1 rounded-full bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-900 ring-1 ring-inset ring-amber-600/25">
                        <EyeOff className="h-3 w-3" aria-hidden="true" />
                        Não reconhecida
                      </span>
                    ) : (
                      col.confidence && (
                        <span
                          title={CONFIDENCE_HINT[col.confidence]}
                          className={cn(
                            'rounded-full px-2 py-0.5 text-[11px] font-medium',
                            severity
                              ? SEVERITY_PILL[severity]
                              : 'bg-gray-100 text-gray-600 ring-1 ring-inset ring-gray-300',
                          )}
                        >
                          {CONFIDENCE_LABEL[col.confidence]}
                        </span>
                      )
                    )}
                    {col.matchedAlias && (
                      <span className="text-[11px] text-gray-500">
                        casou com “{col.matchedAlias}”
                      </span>
                    )}
                  </div>
                </div>

                <ArrowRight className="hidden h-4 w-4 shrink-0 text-gray-400 sm:block" aria-hidden="true" />

                {/* Lado do domínio */}
                <div className="flex shrink-0 flex-wrap items-center gap-2">
                  <label className="sr-only" htmlFor={`field-${col.column}`}>
                    Campo do catálogo para a coluna {col.column}
                  </label>
                  <select
                    id={`field-${col.column}`}
                    value={current?.field ?? ''}
                    onChange={(e) => onFieldChange(col.column, e.target.value as ImportField | '')}
                    className={cn(
                      'rounded-md border px-2 py-1.5 text-xs focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue',
                      isCritical
                        ? 'border-brand-blue bg-brand-blue-light font-semibold text-brand-blue'
                        : 'border-gray-300 bg-white text-gray-700',
                    )}
                  >
                    <option value="">Ignorar esta coluna</option>
                    {IMPORT_FIELDS.map((f) => (
                      <option key={f} value={f}>
                        {fieldLabel(f)}
                      </option>
                    ))}
                  </select>

                  {current && (
                    <>
                      <label className="sr-only" htmlFor={`parser-${col.column}`}>
                        Conversão da coluna {col.column}
                      </label>
                      <select
                        id={`parser-${col.column}`}
                        value={current.parser ?? 'text'}
                        onChange={(e) => onParserChange(col.column, e.target.value as ImportParser)}
                        className="rounded-md border border-gray-300 bg-white px-2 py-1.5 text-xs text-gray-600 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue"
                      >
                        {IMPORT_PARSERS.map((p) => (
                          <option key={p} value={p}>
                            {p}
                          </option>
                        ))}
                      </select>
                    </>
                  )}
                </div>
              </div>

              {/* A AMOSTRA. É o que desempata "VALOR = preço ou custo?". */}
              <div className="mt-2 flex flex-wrap items-center gap-1.5 border-t border-gray-100 pt-2">
                <span className="text-[11px] uppercase tracking-wide text-gray-400">Amostra</span>
                {samples.length === 0 ? (
                  <span className="text-[11px] italic text-gray-400">
                    coluna vazia nas primeiras linhas
                  </span>
                ) : (
                  samples.map((v, i) => (
                    <code
                      key={`${col.column}-${i}`}
                      className="max-w-[14rem] truncate rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[11px] text-gray-700"
                    >
                      {v}
                    </code>
                  ))
                )}
              </div>
            </div>
          )
        })}
      </div>

      {/* O mapeamento confirmado vira PERFIL — dado versionado, não código. */}
      <div className="mt-4 grid gap-3 rounded-md border border-gray-200 bg-gray-50 p-3 sm:grid-cols-2">
        <div>
          <label htmlFor="profile-name" className="block text-xs font-semibold text-gray-700">
            Salvar mapeamento como perfil
          </label>
          <input
            id="profile-name"
            value={profileName}
            onChange={(e) => onProfileNameChange(e.target.value)}
            placeholder="Ex.: Fornecedor Central"
            className="mt-1 w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue"
          />
          <p className="mt-1 text-[11px] leading-relaxed text-gray-500">
            Na próxima planilha deste fornecedor o de/para já vem pronto. Reusar o mesmo nome cria
            uma nova versão — nada é sobrescrito.
          </p>
        </div>
        <div>
          <label htmlFor="supplier-id" className="block text-xs font-semibold text-gray-700">
            Fornecedor (opcional)
          </label>
          <input
            id="supplier-id"
            value={supplierId}
            onChange={(e) => onSupplierIdChange(e.target.value)}
            placeholder="Ex.: FORN-CENTRAL"
            className="mt-1 w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue"
          />
          <p className="mt-1 text-[11px] leading-relaxed text-gray-500">
            Delimita o escopo do lote a este fornecedor. Importar a planilha de um jamais mexe no
            catálogo de outro.
          </p>
        </div>
      </div>
    </div>
  )
}

export default ImportMapping
