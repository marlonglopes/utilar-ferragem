import { useEffect, useState } from 'react'
import { Loader2, Info, AlertTriangle, CheckCircle2 } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import { ErrorState, LoadingRows, Section } from '@/components/admin/primitives'
import { cn } from '@/lib/cn'
import { useStoreSettings, useUpdateAnnouncement } from '@/hooks/useStoreSettings'
import type { AnnouncementLevel } from '@/lib/storeSettingsApi'

const MAX = 280

const LEVELS: { value: AnnouncementLevel; label: string; preview: string }[] = [
  { value: 'info', label: 'Informação', preview: 'bg-brand-blue text-white' },
  { value: 'warning', label: 'Aviso', preview: 'bg-amber-500 text-amber-950' },
  { value: 'success', label: 'Promoção', preview: 'bg-green-600 text-white' },
]

const PREVIEW_ICON = { info: Info, warning: AlertTriangle, success: CheckCircle2 }

/**
 * Configuração da loja — aviso da vitrine. Admin only.
 *
 * Escopo consciente: por ora só o AVISO da vitrine (a config que muda toda
 * semana e não depende de ninguém). Dado cadastral legal (CNPJ, razão social,
 * endereço fiscal) NÃO entra aqui — mora em src/lib/company.ts e depende de
 * revisão jurídica (Decreto 7.962/2013), não é campo pra editar solto.
 */
export default function StoreSettingsPage() {
  const { data, isLoading, isError, error, refetch } = useStoreSettings()
  const save = useUpdateAnnouncement()

  const [enabled, setEnabled] = useState(false)
  const [message, setMessage] = useState('')
  const [level, setLevel] = useState<AnnouncementLevel>('info')
  const [formError, setFormError] = useState('')
  const [saved, setSaved] = useState(false)

  // Hidrata o formulário quando o dado chega. Depende só do objeto carregado —
  // reidratar a cada tecla apagaria o que o dono está digitando.
  useEffect(() => {
    if (data?.announcement) {
      setEnabled(data.announcement.enabled)
      setMessage(data.announcement.message)
      setLevel(data.announcement.level)
    }
  }, [data])

  function submit(e: React.FormEvent) {
    e.preventDefault()
    setFormError('')
    setSaved(false)
    const msg = message.trim()
    // Mesmo guardrail do servidor: ligar sem mensagem mostraria barra em branco.
    if (enabled && msg === '') {
      return setFormError('Escreva a mensagem antes de ligar o aviso.')
    }
    save.mutate(
      { enabled, message: msg, level },
      {
        onSuccess: () => setSaved(true),
        onError: (err) => setFormError(err instanceof Error ? err.message : 'Falha ao salvar.'),
      }
    )
  }

  const PreviewIcon = PREVIEW_ICON[level]

  return (
    <AdminShell
      title="Configuração da loja"
      description="O aviso que aparece no topo da vitrine (promoção, horário de feriado…)."
    >
      {isLoading ? (
        <LoadingRows rows={3} />
      ) : isError ? (
        <ErrorState
          message={error instanceof Error ? error.message : 'Falha ao carregar.'}
          onRetry={() => void refetch()}
        />
      ) : (
        <div className="space-y-4">
          <Section title="Aviso da vitrine">
            <form onSubmit={submit} className="flex flex-col gap-4 p-3 sm:p-4">
              <label className="flex items-center gap-2 text-sm font-medium text-gray-800">
                <input
                  type="checkbox"
                  checked={enabled}
                  onChange={(e) => setEnabled(e.target.checked)}
                  className="h-4 w-4"
                />
                Mostrar aviso no topo da loja
              </label>

              <label className="flex flex-col gap-1 text-xs font-semibold text-gray-600">
                Mensagem
                <textarea
                  value={message}
                  onChange={(e) => setMessage(e.target.value.slice(0, MAX))}
                  rows={2}
                  placeholder="Ex.: Frete grátis nas compras acima de R$ 300 esta semana!"
                  className="rounded-md border border-gray-300 p-2 text-sm text-gray-900"
                />
                <span className="text-right text-[11px] font-normal text-gray-400">
                  {message.length}/{MAX}
                </span>
              </label>

              <fieldset className="flex flex-col gap-1 text-xs font-semibold text-gray-600">
                Tom
                <div className="flex flex-wrap gap-2">
                  {LEVELS.map((l) => (
                    <label
                      key={l.value}
                      className={cn(
                        'flex cursor-pointer items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm',
                        level === l.value
                          ? 'border-brand-blue bg-brand-blue-light font-semibold text-brand-blue'
                          : 'border-gray-300 text-gray-700'
                      )}
                    >
                      <input
                        type="radio"
                        name="level"
                        value={l.value}
                        checked={level === l.value}
                        onChange={() => setLevel(l.value)}
                        className="sr-only"
                      />
                      {l.label}
                    </label>
                  ))}
                </div>
              </fieldset>

              {/* Prévia: mostra exatamente como o cliente verá antes de salvar. */}
              <div className="flex flex-col gap-1">
                <span className="text-xs font-semibold text-gray-600">Prévia</span>
                {message.trim() === '' ? (
                  <p className="rounded-md border border-dashed border-gray-300 p-3 text-center text-sm text-gray-400">
                    (a mensagem aparece aqui)
                  </p>
                ) : (
                  <div
                    className={cn(
                      'flex items-center justify-center gap-2 rounded-md px-4 py-2 text-sm font-medium',
                      LEVELS.find((l) => l.value === level)?.preview
                    )}
                  >
                    <PreviewIcon className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
                    <span>{message.trim()}</span>
                  </div>
                )}
              </div>

              <div className="flex items-center gap-3">
                <button
                  type="submit"
                  disabled={save.isPending}
                  className="inline-flex h-10 items-center gap-1.5 rounded-md bg-brand-orange px-4 text-sm font-semibold text-gray-900 hover:bg-brand-orange-dark disabled:opacity-50"
                >
                  {save.isPending && (
                    <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
                  )}
                  Salvar
                </button>
                {saved && !save.isPending && (
                  <span className="text-sm font-medium text-green-600" role="status">
                    Salvo.
                  </span>
                )}
                {formError && (
                  <span className="text-sm font-semibold text-red-600" role="alert">
                    {formError}
                  </span>
                )}
              </div>
            </form>
          </Section>
        </div>
      )}
    </AdminShell>
  )
}
