import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  commitBatch,
  createBatch,
  createProfile,
  isImportApiEnabled,
  listBatches,
  suggestMapping,
} from '@/lib/adminImportApi'
import { validateImportFile } from '@/lib/adminImportFormat'
import type {
  ColumnMapping,
  CommitResult,
  ImportBatch,
  ImportField,
  ImportParser,
  ImportPlan,
  SuggestResponse,
} from '@/lib/adminImportTypes'

/**
 * Máquina de estados dos quatro passos da importação.
 *
 * O estado vive num hook e não nos componentes porque a regra mais importante
 * da tela é uma INVARIANTE entre passos, não um detalhe de um passo:
 *
 *   trocar o arquivo ou o mapeamento INVALIDA o plano e o resultado.
 *
 * Sem centralizar, o caminho "subo A → confiro o diff → troco para B → aprovo"
 * aplicaria o plano de A com a cara de B. Aqui `plan` e `commit` são zerados em
 * qualquer mudança a montante, por construção.
 *
 * O commit é deliberadamente um passo separado e explícito: nada nesta máquina
 * encadeia dry-run → commit automaticamente.
 */

export type ImportStep = 'upload' | 'mapping' | 'review' | 'done'

export const IMPORT_STEPS: { id: ImportStep; label: string }[] = [
  { id: 'upload', label: 'Enviar planilha' },
  { id: 'mapping', label: 'Conferir mapeamento' },
  { id: 'review', label: 'Revisar o que vai mudar' },
  { id: 'done', label: 'Aprovar' },
]

/** `null` = coluna ignorada de propósito (o padrão para não reconhecida). */
export type MappingDraft = Record<string, ColumnMapping | null>

export interface ImportWizard {
  step: ImportStep
  file: File | null
  fileError: string | null
  suggestion: SuggestResponse | null
  mapping: MappingDraft
  profileName: string
  supplierId: string
  plan: ImportPlan | null
  commit: CommitResult | null
  committedBatchId: string | null
  busy: 'suggest' | 'dryrun' | 'commit' | null
  error: string | null
  history: { data: ImportBatch[] | undefined; isLoading: boolean; refetch: () => void }
  /** Campos críticos (preço/custo/SKU) ainda sem mapeamento confirmado. */
  unmappedCritical: ImportField[]
  selectFile: (file: File | null) => void
  setColumnField: (column: string, field: ImportField | '') => void
  setColumnParser: (column: string, parser: ImportParser) => void
  setProfileName: (name: string) => void
  setSupplierId: (id: string) => void
  goToMapping: () => void
  runDryRun: () => Promise<void>
  approve: () => Promise<void>
  backTo: (step: ImportStep) => void
  reset: () => void
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : 'Falha inesperada na importação.'
}

export function useImportWizard(): ImportWizard {
  const [step, setStep] = useState<ImportStep>('upload')
  const [file, setFile] = useState<File | null>(null)
  const [fileError, setFileError] = useState<string | null>(null)
  const [suggestion, setSuggestion] = useState<SuggestResponse | null>(null)
  const [mapping, setMapping] = useState<MappingDraft>({})
  const [profileName, setProfileName] = useState('')
  const [supplierId, setSupplierId] = useState('')
  const [plan, setPlan] = useState<ImportPlan | null>(null)
  const [commit, setCommit] = useState<CommitResult | null>(null)
  const [committedBatchId, setCommittedBatchId] = useState<string | null>(null)
  const [busy, setBusy] = useState<ImportWizard['busy']>(null)
  const [error, setError] = useState<string | null>(null)

  const abortRef = useRef<AbortController | null>(null)

  // Upload em voo é cancelado ao sair da tela: sem isso a resposta de uma
  // planilha grande chega para um componente desmontado e o operador que voltou
  // para o painel paga a banda de um trabalho que ninguém vai ver.
  useEffect(() => () => abortRef.current?.abort(), [])

  const history = useQuery({
    queryKey: ['admin', 'import', 'batches'],
    queryFn: () => listBatches(),
    gcTime: 2 * 60 * 1000,
    retry: 1,
  })

  const reset = useCallback(() => {
    abortRef.current?.abort()
    abortRef.current = null
    setStep('upload')
    setFile(null)
    setFileError(null)
    setSuggestion(null)
    setMapping({})
    setProfileName('')
    setSupplierId('')
    setPlan(null)
    setCommit(null)
    setCommittedBatchId(null)
    setBusy(null)
    setError(null)
  }, [])

  const selectFile = useCallback(
    (next: File | null) => {
      // Qualquer plano anterior morre AQUI. É a invariante central da tela.
      setPlan(null)
      setCommit(null)
      setCommittedBatchId(null)
      setError(null)
      setSuggestion(null)
      setMapping({})
      if (!next) {
        setFile(null)
        setFileError(null)
        return
      }
      const invalid = validateImportFile(next)
      if (invalid) {
        setFile(null)
        setFileError(invalid)
        return
      }
      setFile(next)
      setFileError(null)
      setProfileName((prev) => prev || next.name.replace(/\.[^.]+$/, ''))

      abortRef.current?.abort()
      const ctl = new AbortController()
      abortRef.current = ctl
      setBusy('suggest')
      void suggestMapping(next, ctl.signal)
        .then((res) => {
          if (ctl.signal.aborted) return
          setSuggestion(res)
          const draft: MappingDraft = {}
          for (const c of res.columns) {
            // Coluna não reconhecida entra como IGNORADA, nunca chutada num
            // campo. Palpite silencioso é como "OBS VENDEDOR" vira descrição.
            draft[c.column] = c.recognized && c.field ? { field: c.field, parser: c.parser } : null
          }
          setMapping(draft)
          setStep('mapping')
        })
        .catch((err: unknown) => {
          if (ctl.signal.aborted) return
          setError(messageOf(err))
        })
        .finally(() => {
          if (!ctl.signal.aborted) setBusy(null)
        })
    },
    [],
  )

  const setColumnField = useCallback((column: string, field: ImportField | '') => {
    setPlan(null)
    setCommit(null)
    setMapping((prev) => ({
      ...prev,
      [column]: field === '' ? null : { field, parser: prev[column]?.parser },
    }))
  }, [])

  const setColumnParser = useCallback((column: string, parser: ImportParser) => {
    setPlan(null)
    setCommit(null)
    setMapping((prev) => {
      const cur = prev[column]
      return cur ? { ...prev, [column]: { ...cur, parser } } : prev
    })
  }, [])

  const confirmedColumns = useMemo(() => {
    const out: Record<string, ColumnMapping> = {}
    for (const [col, m] of Object.entries(mapping)) if (m) out[col] = m
    return out
  }, [mapping])

  const unmappedCritical = useMemo(() => {
    const mapped = new Set(Object.values(confirmedColumns).map((m) => m.field))
    // `sku` é a chave de identidade: sem ele toda linha é rejeitada e o lote
    // inteiro é desperdício. Vale avisar ANTES do dry-run, não depois.
    return (['sku', 'name', 'price'] as ImportField[]).filter((f) => !mapped.has(f))
  }, [confirmedColumns])

  const goToMapping = useCallback(() => {
    if (suggestion) setStep('mapping')
  }, [suggestion])

  const runDryRun = useCallback(async () => {
    if (!file) return
    setError(null)
    setCommit(null)
    setCommittedBatchId(null)
    setBusy('dryrun')
    const ctl = new AbortController()
    abortRef.current = ctl
    try {
      // O perfil é criado ANTES do lote porque é ele que registra o que o humano
      // confirmou. O backend versiona por nome — reimportar amanhã com o mesmo
      // nome gera a versão seguinte em vez de sobrescrever, e a trilha continua
      // sabendo qual mapeamento gerou qual lote.
      const profile = await createProfile({
        name: profileName.trim() || file.name.replace(/\.[^.]+$/, ''),
        description: `Mapeamento confirmado na tela de importação a partir de ${file.name}`,
        columns: confirmedColumns,
        // `publishOnImport` NÃO é oferecido pela tela, de propósito: publicar é
        // decisão humana por item, não efeito colateral de uma importação.
        options: { archiveMissing: false },
      })
      const result = await createBatch(
        { file, profileId: profile.id, supplierId: supplierId.trim() || undefined, mapping: confirmedColumns },
        ctl.signal,
      )
      if (ctl.signal.aborted) return
      setPlan(result)
      setStep('review')
    } catch (err) {
      if (!ctl.signal.aborted) setError(messageOf(err))
    } finally {
      if (!ctl.signal.aborted) setBusy(null)
    }
  }, [file, profileName, confirmedColumns, supplierId])

  const approve = useCallback(async () => {
    if (!plan) return
    setError(null)
    setBusy('commit')
    try {
      const res = await commitBatch(plan.batchId, plan)
      setCommit(res.result)
      setCommittedBatchId(res.batchId)
      setStep('done')
      void history.refetch()
    } catch (err) {
      // Falha de commit NÃO volta o passo: o lote pode ter aplicado parte das
      // linhas (uma transação por linha), e mandar o operador de volta para a
      // revisão sugeriria que nada aconteceu.
      setError(messageOf(err))
    } finally {
      setBusy(null)
    }
  }, [plan, history])

  const backTo = useCallback(
    (target: ImportStep) => {
      if (target === 'upload') {
        reset()
        return
      }
      if (target === 'mapping' && suggestion) setStep('mapping')
      if (target === 'review' && plan) setStep('review')
    },
    [reset, suggestion, plan],
  )

  return {
    step,
    file,
    fileError,
    suggestion,
    mapping,
    profileName,
    supplierId,
    plan,
    commit,
    committedBatchId,
    busy,
    error,
    history: {
      data: history.data,
      isLoading: history.isLoading,
      refetch: () => void history.refetch(),
    },
    unmappedCritical,
    selectFile,
    setColumnField,
    setColumnParser,
    setProfileName,
    setSupplierId,
    goToMapping,
    runDryRun,
    approve,
    backTo,
    reset,
  }
}

export { isImportApiEnabled }
