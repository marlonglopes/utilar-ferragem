import { useState } from 'react'
import { UserPlus, UserCheck, X, Search, ArrowLeft } from 'lucide-react'
import { Modal, Button, Input, Select } from '@/components/ui'
import { formatCPF, formatCNPJ, formatPhone } from '@/lib/format'
import { useBalcaoCustomers } from '@/hooks/useBalcaoCustomers'
import { isCNPJ, type BalcaoCustomer, type CustomerSegment } from '@/store/balcaoStore'

const SEGMENT_LABEL: Record<CustomerSegment, string> = {
  varejo: 'Varejo',
  atacado: 'Atacado',
  construtora: 'Construtora',
}

function maskDocument(value: string): string {
  return isCNPJ(value) ? formatCNPJ(value) : formatCPF(value)
}

export interface CustomerBlockProps {
  customer: BalcaoCustomer | null
  onChange: (customer: BalcaoCustomer | null) => void
}

/**
 * Identificação do cliente no balcão — ligada em `/api/v1/store/customers`.
 *
 * O fluxo tem dois passos e a ordem importa: BUSCA primeiro, cadastro só se não
 * achar. Antes o botão "Buscar" abria direto o cadastro, o que fazia o vendedor
 * redigitar um cliente que já existia na base a cada visita — e criava um
 * cliente que vivia só na comanda.
 *
 * 404 não é erro: o contrato do backend é "um ou nenhum", e "nenhum" é o
 * caminho normal para abrir o cadastro rápido já com o documento preenchido.
 */
export function CustomerBlock({ customer, onChange }: CustomerBlockProps) {
  const { lookup, create, searching, saving } = useBalcaoCustomers()

  const [open, setOpen] = useState(false)
  const [step, setStep] = useState<'search' | 'form'>('search')
  const [doc, setDoc] = useState('')
  const [name, setName] = useState('')
  const [phone, setPhone] = useState('')
  const [segment, setSegment] = useState<CustomerSegment>('varejo')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  function openSearch() {
    setStep('search')
    setDoc(customer ? maskDocument(customer.document) : '')
    setError('')
    setNotice('')
    setOpen(true)
  }

  /** Editar um cliente já identificado pula a busca: o documento já é conhecido. */
  function openEdit() {
    setStep('form')
    setDoc(customer ? maskDocument(customer.document) : '')
    setName(customer?.name ?? '')
    setPhone(customer ? formatPhone(customer.phone) : '')
    setSegment(customer?.segment ?? 'varejo')
    setError('')
    setNotice('')
    setOpen(true)
  }

  async function search() {
    const digits = doc.replace(/\D/g, '')
    if (digits.length !== 11 && digits.length !== 14) {
      return setError('CPF (11 dígitos) ou CNPJ (14 dígitos).')
    }
    setError('')
    setNotice('')
    try {
      const result = await lookup(digits)
      if (result.found) {
        onChange(result.customer)
        setOpen(false)
        return
      }
      // Não achou → cadastro rápido, já com o documento digitado.
      setName('')
      setPhone('')
      setSegment('varejo')
      setNotice('Cliente não encontrado. Faça o cadastro rápido abaixo.')
      setStep('form')
    } catch (err) {
      setError(
        err instanceof Error && /failed to fetch|networkerror|load failed/i.test(err.message)
          ? 'Sem conexão para consultar. Cadastre manualmente ou tente de novo.'
          : err instanceof Error
            ? err.message
            : 'Não foi possível consultar o cliente.'
      )
    }
  }

  async function save() {
    const digitsDoc = doc.replace(/\D/g, '')
    const digitsPhone = phone.replace(/\D/g, '')
    if (!name.trim()) return setError('Informe o nome do cliente.')
    if (digitsDoc.length !== 11 && digitsDoc.length !== 14) {
      return setError('CPF (11 dígitos) ou CNPJ (14 dígitos).')
    }
    // Obrigatório: a Appmax recusa a cobrança sem celular do pagador.
    if (digitsPhone.length < 10) return setError('Telefone é obrigatório para cobrar (Appmax).')

    setError('')
    try {
      const created = await create({
        document: digitsDoc,
        name: name.trim(),
        phone: digitsPhone,
        segment,
      })
      onChange(created)
      setOpen(false)
    } catch (err) {
      // Falhar o cadastro não pode travar a venda: o pedido de balcão aceita o
      // snapshot do cliente sem `customerId`. Registra local e segue.
      onChange({ name: name.trim(), document: digitsDoc, phone: digitsPhone, segment })
      setOpen(false)
      void err
    }
  }

  return (
    <div className="border-b border-gray-200 p-4">
      {customer ? (
        <div className="flex items-start gap-3">
          <span className="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-brand-blue-light">
            <UserCheck className="h-5 w-5 text-brand-blue" aria-hidden="true" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <p className="truncate font-semibold text-gray-900">{customer.name}</p>
              <span className="shrink-0 rounded-full bg-brand-blue-light px-2 py-0.5 text-[11px] font-bold text-brand-blue">
                {SEGMENT_LABEL[customer.segment]}
              </span>
            </div>
            <p className="font-mono text-xs text-gray-500">{maskDocument(customer.document)}</p>
            <p className="text-xs text-gray-500">{formatPhone(customer.phone)}</p>
          </div>
          <div className="flex shrink-0 gap-1">
            <button
              type="button"
              onClick={openEdit}
              aria-label="Editar cliente"
              className="flex h-12 w-12 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100"
            >
              <UserPlus className="h-5 w-5" aria-hidden="true" />
            </button>
            <button
              type="button"
              onClick={() => onChange(null)}
              aria-label="Remover cliente"
              className="flex h-12 w-12 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100"
            >
              <X className="h-5 w-5" aria-hidden="true" />
            </button>
          </div>
        </div>
      ) : (
        <button
          type="button"
          onClick={openSearch}
          className="flex min-h-[56px] w-full items-center gap-3 rounded-lg border border-dashed border-gray-300 px-3 text-left hover:border-brand-orange hover:bg-orange-50"
        >
          <Search className="h-5 w-5 shrink-0 text-gray-400" aria-hidden="true" />
          <span>
            <span className="block text-sm font-semibold text-gray-900">Identificar cliente</span>
            <span className="block text-xs text-gray-500">Buscar por CPF/CNPJ ou cadastrar</span>
          </span>
        </button>
      )}

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title={step === 'search' ? 'Buscar cliente' : 'Cadastro rápido'}
        size="sm"
      >
        <div className="flex flex-col gap-3">
          <Input
            label="CPF / CNPJ"
            value={doc}
            inputMode="numeric"
            autoFocus
            readOnly={step === 'form'}
            onChange={(e) => setDoc(maskDocument(e.target.value))}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && step === 'search') void search()
            }}
            placeholder="000.000.000-00"
            className="h-12 text-base"
          />

          {step === 'search' && (
            <>
              <Button
                size="lg"
                fullWidth
                loading={searching}
                onClick={() => void search()}
                className="h-12"
              >
                <Search className="mr-2 h-5 w-5" aria-hidden="true" />
                Buscar
              </Button>
              <button
                type="button"
                onClick={() => {
                  setNotice('')
                  setError('')
                  setStep('form')
                }}
                className="flex h-12 items-center justify-center rounded-lg border border-gray-300 text-sm font-semibold text-gray-700 hover:bg-gray-100"
              >
                Cadastrar sem buscar
              </button>
            </>
          )}

          {step === 'form' && (
            <>
              {notice && (
                <p role="status" className="text-sm font-semibold text-amber-700">
                  {notice}
                </p>
              )}
              <Input
                label="Nome"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Nome do cliente"
                className="h-12 text-base"
              />
              <Input
                label="Telefone"
                value={phone}
                inputMode="tel"
                onChange={(e) => setPhone(formatPhone(e.target.value))}
                placeholder="(11) 99999-0000"
                hint="Obrigatório — a operadora exige o celular do pagador."
                className="h-12 text-base"
              />
              <Select
                label="Segmento"
                value={segment}
                onChange={(e) => setSegment(e.target.value as CustomerSegment)}
                options={(Object.keys(SEGMENT_LABEL) as CustomerSegment[]).map((s) => ({
                  value: s,
                  label: SEGMENT_LABEL[s],
                }))}
                className="h-12 text-base"
              />
            </>
          )}

          {error && (
            <p role="alert" className="text-sm font-semibold text-red-600">
              {error}
            </p>
          )}

          {step === 'form' && (
            <>
              <Button
                size="lg"
                fullWidth
                loading={saving}
                onClick={() => void save()}
                className="mt-1 h-12"
              >
                Salvar cliente
              </Button>
              <button
                type="button"
                onClick={() => {
                  setNotice('')
                  setError('')
                  setStep('search')
                }}
                className="flex h-12 items-center justify-center gap-2 rounded-lg text-sm font-semibold text-gray-600 hover:bg-gray-100"
              >
                <ArrowLeft className="h-4 w-4" aria-hidden="true" />
                Voltar para a busca
              </button>
            </>
          )}
        </div>
      </Modal>
    </div>
  )
}
