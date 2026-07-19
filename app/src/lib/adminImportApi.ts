import { useAuthStore } from '@/store/authStore'
import { AdminApiError } from '@/lib/adminApi'
import type {
  ColumnMapping,
  CommitResponse,
  ImportBatch,
  ImportPlan,
  ImportProfile,
  ProfileInput,
  SuggestResponse,
} from '@/lib/adminImportTypes'
import { mockBatches, mockCommit, mockPlan, mockSuggest } from '@/lib/adminImportMock'

/**
 * Camada de rede da ingestão de produtos (catalog-service).
 *
 * Separada de `adminApi.ts` porque fala com OUTRO serviço (catálogo, não
 * pagamento/pedido) e porque o upload tem uma política própria:
 *
 * - **`multipart/form-data` sem `Content-Type` manual.** Definir o header à mão
 *   quebra o upload: o boundary é gerado pelo `FormData` e só o navegador sabe
 *   qual é. Header fixo produz um 400 do servidor que parece problema de
 *   arquivo e não é.
 * - **`AbortSignal` obrigatório nas rotas de arquivo.** Planilha grande demora;
 *   sem cancelamento, sair da tela deixa o upload rodando e a resposta chega
 *   para um componente desmontado.
 * - **`cache: 'no-store'`**, mesma razão do resto do painel: dado de catálogo e
 *   custo não fica no cache de disco de uma máquina de loja.
 *
 * ⚠️ O guard de rota não é fronteira de segurança. Toda rota abaixo é
 * `RequireAdmin` no catalog-service.
 */

const CATALOG_URL = import.meta.env.VITE_CATALOG_URL ?? ''

/** Sem catalog-service configurado, a tela roda em demonstração. */
export const isImportApiEnabled = CATALOG_URL !== ''

const BASE = '/api/v1/admin/import'

function authHeaders(): Record<string, string> {
  // Lido a cada chamada, nunca capturado em closure: o refresh-on-401 pode ter
  // trocado o access token no meio de um upload longo.
  const token = useAuthStore.getState().user?.token ?? null
  return token ? { Authorization: `Bearer ${token}` } : {}
}

async function failure(res: Response): Promise<never> {
  const body = (await res.json().catch(() => ({}))) as { error?: string; code?: string }
  if (res.status === 403) {
    throw new AdminApiError('Sua conta não tem permissão de administrador.', 403, 'forbidden')
  }
  if (res.status === 413) {
    throw new AdminApiError(
      'O servidor recusou o arquivo por tamanho. Envie em CSV ou divida a planilha.',
      413,
      'too_large',
    )
  }
  throw new AdminApiError(body.error ?? `HTTP ${res.status}`, res.status, body.code)
}

async function send<T>(path: string, init: RequestInit): Promise<T> {
  const res = await fetch(`${CATALOG_URL}${BASE}${path}`, {
    cache: 'no-store',
    ...init,
    headers: { ...authHeaders(), ...(init.headers ?? {}) },
  })
  if (!res.ok) await failure(res)
  return (await res.json()) as T
}

function fileBody(file: File): FormData {
  const fd = new FormData()
  fd.append('file', file)
  return fd
}

// ---------------------------------------------------------------------------
// Passo 1/2 — detectar colunas e sugerir o mapeamento
// ---------------------------------------------------------------------------

export async function suggestMapping(file: File, signal?: AbortSignal): Promise<SuggestResponse> {
  if (!isImportApiEnabled) return mockSuggest(file)
  return send<SuggestResponse>('/suggest', { method: 'POST', body: fileBody(file), signal })
}

// ---------------------------------------------------------------------------
// Perfil de mapeamento — o que o humano CONFIRMOU
// ---------------------------------------------------------------------------

export async function listProfiles(): Promise<ImportProfile[]> {
  if (!isImportApiEnabled) return []
  const res = await send<{ data: ImportProfile[] }>('/profiles', { method: 'GET' })
  return res.data ?? []
}

export async function createProfile(input: ProfileInput): Promise<ImportProfile> {
  if (!isImportApiEnabled) {
    return { id: `demo-profile-${Date.now().toString(36)}`, name: input.name, version: 1 }
  }
  return send<ImportProfile>('/profiles', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

// ---------------------------------------------------------------------------
// Passo 3 — staging + DRY-RUN (não escreve nada em produtos)
// ---------------------------------------------------------------------------

export interface DryRunInput {
  file: File
  profileId: string
  supplierId?: string
  /** Só para o modo demonstração, que não tem perfil salvo no servidor. */
  mapping?: Record<string, ColumnMapping>
}

export async function createBatch(input: DryRunInput, signal?: AbortSignal): Promise<ImportPlan> {
  if (!isImportApiEnabled) return mockPlan(input.file, input.mapping ?? {})
  const qs = new URLSearchParams({ profileId: input.profileId })
  if (input.supplierId) qs.set('supplierId', input.supplierId)
  return send<ImportPlan>(`/batches?${qs.toString()}`, {
    method: 'POST',
    body: fileBody(input.file),
    signal,
  })
}

// ---------------------------------------------------------------------------
// Passo 4 — commit (a aprovação humana)
// ---------------------------------------------------------------------------

export async function commitBatch(batchId: string, plan?: ImportPlan): Promise<CommitResponse> {
  if (!isImportApiEnabled) {
    if (!plan) throw new AdminApiError('Nenhum lote em memória para aplicar.', 400)
    return { batchId, status: 'committed', result: mockCommit(plan) }
  }
  return send<CommitResponse>(`/batches/${encodeURIComponent(batchId)}/commit`, { method: 'POST' })
}

// ---------------------------------------------------------------------------
// Histórico
// ---------------------------------------------------------------------------

export async function listBatches(limit = 50): Promise<ImportBatch[]> {
  if (!isImportApiEnabled) return mockBatches()
  const res = await send<{ data: ImportBatch[] }>(`/batches?limit=${limit}`, { method: 'GET' })
  return res.data ?? []
}
