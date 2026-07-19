import { AlertTriangle, Loader2 } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import { Section } from '@/components/admin/primitives'
import { ImportStepper } from '@/components/admin/import/ImportStepper'
import { ImportDropzone } from '@/components/admin/import/ImportDropzone'
import { ImportMapping } from '@/components/admin/import/ImportMapping'
import { ImportDiff } from '@/components/admin/import/ImportDiff'
import { ImportApprovePanel, ImportResult } from '@/components/admin/import/ImportApprove'
import { ImportHistory } from '@/components/admin/import/ImportHistory'
import { useImportWizard, type ImportStep } from '@/hooks/useImportWizard'
import { isImportApiEnabled } from '@/lib/adminImportApi'

/**
 * Importação de produtos por planilha.
 *
 * Até aqui a ingestão só existia por API/script — quem não abre o terminal não
 * conseguia atualizar o catálogo. Esta tela dá o mesmo pipeline ao navegador,
 * SEM encurtá-lo: o backend é deliberadamente em dois tempos (dry-run e depois
 * commit) e a tela reflete isso com dois botões distintos, em passos distintos.
 *
 * Um botão único "importar" seria mais cômodo e é exatamente o que não pode
 * existir: ingestão de catálogo é a operação mais destrutiva de uma loja — um
 * mapeamento errado de coluna zera o preço de 4.000 SKUs em um segundo.
 */
export default function AdminImportPage() {
  const w = useImportWizard()

  const reachable: ImportStep[] = ['upload']
  if (w.suggestion) reachable.push('mapping')
  if (w.plan) reachable.push('review')

  return (
    <AdminShell
      title="Importar produtos"
      description="Planilha do fornecedor → conferência → aprovação. Nada entra na loja sem passar por você."
      toolbar={<ImportStepper current={w.step} reachable={reachable} onNavigate={w.backTo} />}
    >
      <div className="space-y-4">
        {!isImportApiEnabled && (
          <p className="rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-amber-50/60 p-3 text-xs leading-relaxed text-gray-700">
            <strong>Modo demonstração.</strong> O catálogo não está configurado
            (<code className="font-mono">VITE_CATALOG_URL</code> vazio): a planilha é lida no seu
            navegador, as regras aplicadas são uma amostra das do servidor e{' '}
            <strong>nada é gravado</strong>. Serve para conhecer o fluxo, não para importar de
            verdade.
          </p>
        )}

        {w.error && (
          <div
            role="alert"
            className="flex items-start gap-2 rounded-md border border-red-200 border-l-4 border-l-red-600 bg-red-50 p-3"
          >
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-red-700" aria-hidden="true" />
            <div className="min-w-0">
              <p className="text-sm font-semibold text-red-800">A operação não foi concluída</p>
              <p className="mt-0.5 text-xs leading-relaxed text-red-700">{w.error}</p>
            </div>
          </div>
        )}

        {/* ---------------------------------------------------- Passo 1 */}
        {w.step === 'upload' && (
          <Section
            title="1. Enviar a planilha"
            description="CSV, XLSX ou JSON. O arquivo é lido e conferido antes de qualquer escrita."
          >
            <ImportDropzone
              file={w.file}
              error={w.fileError}
              loading={w.busy === 'suggest'}
              onSelect={w.selectFile}
            />
          </Section>
        )}

        {/* ---------------------------------------------------- Passo 2 */}
        {w.step === 'mapping' && w.suggestion && (
          <Section
            title="2. Conferir o mapeamento das colunas"
            description={`${w.suggestion.filename} · ${w.suggestion.summary.rows} linhas · ${w.suggestion.summary.columns} colunas`}
            actions={
              <div className="flex flex-wrap items-center gap-2">
                <button
                  type="button"
                  onClick={() => w.backTo('upload')}
                  className="rounded-md border border-gray-300 px-3 py-1.5 text-xs font-semibold text-gray-700 hover:bg-gray-50"
                >
                  Trocar arquivo
                </button>
                <button
                  type="button"
                  onClick={() => void w.runDryRun()}
                  disabled={w.busy === 'dryrun'}
                  data-testid="dryrun-button"
                  className="inline-flex items-center gap-2 rounded-md bg-brand-blue px-3 py-1.5 text-xs font-semibold text-white hover:bg-brand-blue/90 disabled:cursor-not-allowed disabled:bg-gray-300"
                >
                  {w.busy === 'dryrun' && <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />}
                  {w.busy === 'dryrun' ? 'Simulando…' : 'Simular importação'}
                </button>
              </div>
            }
          >
            <ImportMapping
              suggestion={w.suggestion}
              mapping={w.mapping}
              profileName={w.profileName}
              supplierId={w.supplierId}
              unmappedCritical={w.unmappedCritical}
              onFieldChange={w.setColumnField}
              onParserChange={w.setColumnParser}
              onProfileNameChange={w.setProfileName}
              onSupplierIdChange={w.setSupplierId}
            />
            {w.busy === 'dryrun' && (
              // Estado honesto: planilha grande demora e o servidor tem 90 s de
              // teto. Silêncio aqui faz o operador clicar de novo.
              <p
                aria-live="polite"
                className="border-t border-gray-200 px-3 py-2 text-xs text-gray-600 sm:px-4"
              >
                Lendo a planilha e comparando linha a linha com o catálogo. Pode levar alguns
                minutos em arquivos grandes — a tela continua utilizável e nada foi gravado ainda.
              </p>
            )}
          </Section>
        )}

        {/* ---------------------------------------------------- Passo 3 + 4 */}
        {w.step === 'review' && w.plan && (
          <>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h2 className="font-display text-sm font-bold text-gray-900">
                3. Revisar o que vai mudar
              </h2>
              <button
                type="button"
                onClick={() => w.backTo('mapping')}
                className="rounded-md border border-gray-300 px-3 py-1.5 text-xs font-semibold text-gray-700 hover:bg-gray-50"
              >
                Voltar ao mapeamento
              </button>
            </div>
            <ImportDiff plan={w.plan} />
            <ImportApprovePanel
              plan={w.plan}
              busy={w.busy === 'commit'}
              onApprove={() => void w.approve()}
            />
          </>
        )}

        {/* ---------------------------------------------------- Resultado */}
        {w.step === 'done' && (
          <ImportResult
            result={w.commit}
            batchId={w.committedBatchId}
            plan={w.plan}
            error={w.error}
            onRestart={w.reset}
          />
        )}

        <ImportHistory batches={w.history.data} loading={w.history.isLoading} />
      </div>
    </AdminShell>
  )
}
