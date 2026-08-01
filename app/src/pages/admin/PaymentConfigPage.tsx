import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, XCircle } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import { Chip, ErrorState, Section } from '@/components/admin/primitives'
import {
  fetchPaymentConfig,
  isPaymentConfigEnabled,
  METHOD_LABEL,
  PROVIDER_LABEL,
} from '@/lib/adminPaymentApi'

/**
 * Config de pagamento (LEITURA). Qual PSP está ativo, métodos e saúde da
 * credencial. Trocar de PSP ou a credencial é variável de ambiente
 * (PSP_PROVIDER + *_SECRET) — segredo não se edita por tela, então aqui é só
 * status. Só admin.
 */
export default function PaymentConfigPage() {
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['admin-payment-config'],
    queryFn: fetchPaymentConfig,
    staleTime: 15_000,
  })

  return (
    <AdminShell
      title="Pagamento"
      description="Qual gateway processa o dinheiro, métodos aceitos e saúde da credencial."
    >
      <div className="space-y-4">
        {!isPaymentConfigEnabled && (
          <p className="rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-amber-50/60 p-3 text-xs leading-relaxed text-gray-700">
            <strong>Modo demonstração.</strong> O serviço de pagamento não está configurado (
            <code className="font-mono">VITE_API_URL</code> vazio): o status abaixo é inventado.
          </p>
        )}

        {isError ? (
          <ErrorState
            message={error instanceof Error ? error.message : 'Falha ao carregar'}
            onRetry={() => void refetch()}
          />
        ) : (
          <Section title="Gateway ativo">
            <div className="grid gap-4 p-4 sm:grid-cols-3">
              <div>
                <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                  Provedor (PSP)
                </p>
                <p className="mt-1 text-lg font-bold text-gray-900">
                  {isLoading
                    ? '…'
                    : (PROVIDER_LABEL[data?.provider ?? ''] ?? data?.provider ?? '—')}
                </p>
              </div>

              <div>
                <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                  Métodos aceitos
                </p>
                <div className="mt-1 flex flex-wrap gap-1.5">
                  {(data?.methods ?? []).map((m) => (
                    <Chip key={m} className="bg-brand-blue-light text-brand-blue">
                      {METHOD_LABEL[m] ?? m}
                    </Chip>
                  ))}
                </div>
              </div>

              <div>
                <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">Saúde</p>
                <div className="mt-1">
                  {isLoading ? (
                    <span className="text-gray-400">…</span>
                  ) : data?.healthy ? (
                    <span className="inline-flex items-center gap-1.5 text-sm font-semibold text-emerald-700">
                      <CheckCircle2 className="h-4 w-4" aria-hidden="true" /> Operante
                    </span>
                  ) : (
                    <span className="inline-flex items-center gap-1.5 text-sm font-semibold text-red-700">
                      <XCircle className="h-4 w-4" aria-hidden="true" /> Degradado — verifique a
                      credencial
                    </span>
                  )}
                </div>
              </div>
            </div>
          </Section>
        )}

        <p className="rounded-md border border-gray-200 bg-gray-50 p-3 text-xs leading-relaxed text-gray-600">
          <strong>Como trocar o gateway ou a credencial:</strong> é configuração de ambiente (
          <code className="font-mono">PSP_PROVIDER</code> e a chave secreta do provedor), não se
          edita por aqui — segredo nunca passa pela tela. Esta página é só o retrato do que está
          ativo agora.
        </p>
      </div>
    </AdminShell>
  )
}
