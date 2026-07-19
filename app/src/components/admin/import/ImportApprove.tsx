import { AlertTriangle, CheckCircle2, FileWarning, Loader2, ShieldCheck } from 'lucide-react'
import { cn } from '@/lib/cn'
import { formatCount } from '@/lib/adminFormat'
import type { CommitResult, ImportPlan } from '@/lib/adminImportTypes'

/**
 * Passo 4 — aprovar, e depois o que REALMENTE aconteceu.
 *
 * Duas regras que esta parte da tela existe para cumprir:
 *
 * 1. **A importação nunca publica.** Produto entra como RASCUNHO e a vitrine é
 *    uma decisão humana por item. Isso é dito com todas as letras aqui e não
 *    fica implícito num tooltip, porque a expectativa contrária ("subi a
 *    planilha, então já está vendendo") é o mal-entendido caro.
 * 2. **O resultado não é a previsão.** O backend usa uma transação POR LINHA
 *    justamente para uma linha ruim não abortar o lote — então um commit pode
 *    ser parcial. Repetir os números do dry-run como se fossem o resultado
 *    esconderia exatamente o caso em que a diferença importa.
 */

export function ImportApprovePanel({
  plan,
  busy,
  onApprove,
}: {
  plan: ImportPlan
  busy: boolean
  onApprove: () => void
}) {
  const willWrite = plan.summary.creates + plan.summary.updates
  const nothingToDo = willWrite === 0 && plan.summary.toArchive === 0

  return (
    <div className="rounded-lg border border-gray-200 border-l-4 border-l-brand-blue bg-white p-3 sm:p-4">
      <div className="flex items-start gap-2">
        <ShieldCheck className="mt-0.5 h-5 w-5 shrink-0 text-brand-blue" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <h3 className="font-display text-sm font-bold text-gray-900">Aprovar e aplicar</h3>
          <p className="mt-1 text-xs leading-relaxed text-gray-600">
            Ao confirmar, {formatCount(plan.summary.creates)} produtos serão criados e{' '}
            {formatCount(plan.summary.updates)} atualizados.{' '}
            {plan.summary.reviews > 0 && (
              <>
                As {formatCount(plan.summary.reviews)} linhas retidas <strong>não</strong> serão
                aplicadas — ficam guardadas esperando decisão.{' '}
              </>
            )}
            {plan.summary.rejects > 0 && (
              <>As {formatCount(plan.summary.rejects)} rejeitadas são ignoradas. </>
            )}
          </p>

          {/* O aviso do rascunho. Explícito, com destaque, nunca em letra miúda. */}
          <div className="mt-3 flex items-start gap-2 rounded-md bg-amber-50 p-2.5 ring-1 ring-inset ring-amber-600/25">
            <FileWarning className="mt-0.5 h-4 w-4 shrink-0 text-amber-700" aria-hidden="true" />
            <p className="text-xs leading-relaxed text-amber-900">
              <strong>Os produtos entram como rascunho.</strong> Nada desta importação aparece na
              loja automaticamente: publicar é uma decisão sua, item a item, depois de conferir
              preço e foto. Rascunho não vaza nem por link direto.
            </p>
          </div>

          <div className="mt-3 flex flex-wrap items-center gap-3">
            <button
              type="button"
              onClick={onApprove}
              disabled={busy || nothingToDo}
              data-testid="approve-button"
              className={cn(
                'inline-flex items-center gap-2 rounded-md px-4 py-2 text-sm font-semibold text-white transition-colors',
                busy || nothingToDo
                  ? 'cursor-not-allowed bg-gray-300'
                  : 'bg-emerald-600 hover:bg-emerald-700',
              )}
            >
              {busy && <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />}
              {busy ? 'Aplicando…' : `Aprovar e importar ${formatCount(willWrite)} produtos`}
            </button>
            {nothingToDo && (
              <span className="text-xs text-gray-500">
                Nada a aplicar — nenhuma linha desta planilha cria ou atualiza produto.
              </span>
            )}
          </div>
          <p className="mt-2 text-[11px] text-gray-500">
            Lote <code className="font-mono">{plan.batchId}</code> · a aprovação fica registrada na
            trilha de auditoria com seu usuário.
          </p>
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------

const RESULT_ROWS: { key: keyof CommitResult; label: string; hint: string }[] = [
  { key: 'created', label: 'Criados', hint: 'entraram como rascunho' },
  { key: 'updated', label: 'Atualizados', hint: 'SKUs que já existiam' },
  { key: 'held', label: 'Retidos', hint: 'não aplicados, esperando decisão humana' },
  { key: 'rejected', label: 'Rejeitados', hint: 'linhas inválidas, ignoradas' },
  { key: 'skipped', label: 'Sem mudança', hint: 'idênticos ao catálogo' },
  { key: 'archived', label: 'Arquivados', hint: 'sumiram da planilha — nunca apagados' },
  { key: 'failed', label: 'Falharam', hint: 'erro de escrita nesta linha' },
]

export function ImportResult({
  result,
  batchId,
  plan,
  error,
  onRestart,
}: {
  result: CommitResult | null
  batchId: string | null
  plan: ImportPlan | null
  error: string | null
  onRestart: () => void
}) {
  const partial = (result?.failed ?? 0) > 0
  const applied = (result?.created ?? 0) + (result?.updated ?? 0)

  return (
    <div className="space-y-4">
      <div
        className={cn(
          'rounded-lg border border-gray-200 bg-white p-3 sm:p-4',
          partial || error ? 'border-l-4 border-l-amber-500' : 'border-l-4 border-l-emerald-500',
        )}
      >
        <div className="flex items-start gap-2">
          {partial || error ? (
            <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-amber-600" aria-hidden="true" />
          ) : (
            <CheckCircle2 className="mt-0.5 h-5 w-5 shrink-0 text-emerald-600" aria-hidden="true" />
          )}
          <div className="min-w-0">
            <h3 className="font-display text-sm font-bold text-gray-900">
              {error
                ? 'A importação foi interrompida'
                : partial
                  ? 'Importação aplicada parcialmente'
                  : 'Importação aplicada'}
            </h3>
            <p className="mt-1 text-xs leading-relaxed text-gray-600">
              {/* Resultado REAL, nunca a previsão do dry-run repetida. */}
              {formatCount(applied)} produtos entraram no catálogo{' '}
              <strong className="text-gray-800">como rascunho</strong>
              {partial && (
                <>
                  {' '}
                  e {formatCount(result?.failed ?? 0)} linhas falharam na escrita. Como o servidor
                  grava uma linha por transação, o que entrou está no catálogo e o que falhou não —
                  reenviar o mesmo arquivo conserta o lote sem duplicar nada.
                </>
              )}
              {!partial && !error && '. Publicar continua sendo uma decisão sua, item a item.'}
            </p>
            {batchId && (
              <p className="mt-1 text-[11px] text-gray-500">
                Lote <code className="font-mono">{batchId}</code>
              </p>
            )}
          </div>
        </div>

        {error && (
          <p
            role="alert"
            className="mt-3 rounded-md border border-red-200 bg-red-50 p-2.5 text-xs leading-relaxed text-red-800"
          >
            {error}
          </p>
        )}
      </div>

      {result && (
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-7">
          {RESULT_ROWS.map((r) => {
            const value = (result[r.key] as number) ?? 0
            const alarming = r.key === 'failed' && value > 0
            return (
              <div
                key={r.key}
                data-testid={`result-${r.key}`}
                className={cn(
                  'rounded-lg border border-gray-200 bg-white p-3',
                  alarming && 'border-l-4 border-l-red-600',
                )}
              >
                <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{r.label}</p>
                <p
                  className={cn(
                    'mt-0.5 font-display text-2xl font-bold tabular-nums leading-tight',
                    value === 0 ? 'text-gray-300' : alarming ? 'text-red-700' : 'text-gray-900',
                  )}
                >
                  {formatCount(value)}
                </p>
                <p className="mt-0.5 text-[11px] leading-tight text-gray-500">{r.hint}</p>
              </div>
            )
          })}
        </div>
      )}

      {result?.errors && result.errors.length > 0 && (
        <div className="rounded-lg border border-gray-200 bg-white p-3">
          <p className="text-xs font-semibold text-gray-800">Linhas que não entraram</p>
          <ul className="mt-1.5 space-y-1">
            {result.errors.map((e, i) => (
              <li key={i} className="text-xs leading-snug text-red-800">
                {e.field && <span className="font-mono font-semibold">{e.field}: </span>}
                {e.message}
              </li>
            ))}
          </ul>
        </div>
      )}

      {plan && plan.summary.reviews > 0 && (
        <p className="rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-white p-3 text-xs leading-relaxed text-gray-700">
          <strong>{formatCount(plan.summary.reviews)} linhas continuam retidas.</strong> A variação
          de preço delas ficou fora do limite configurado. Elas ficam guardadas no lote, não afetam
          o catálogo, e esperam uma decisão humana — reter é o comportamento correto, não uma falha.
        </p>
      )}

      <button
        type="button"
        onClick={onRestart}
        className="rounded-md border border-gray-300 px-3 py-2 text-sm font-semibold text-gray-700 hover:bg-gray-50"
      >
        Importar outra planilha
      </button>
    </div>
  )
}
