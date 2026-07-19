import { useState } from 'react'
import { UserPlus, UserCheck, X, Search } from 'lucide-react'
import { Modal, Button, Input, Select } from '@/components/ui'
import { formatCPF, formatCNPJ, formatPhone } from '@/lib/format'
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
 * Identificação do cliente no balcão.
 *
 * TODO(backend): não existe busca de cliente por documento. O auth-service tem
 * `POST /auth/register` e `GET /auth/me`, mas nada como
 * `GET /api/v1/customers?document=`. Por isso o "Buscar" abre direto o cadastro
 * rápido pré-preenchido em vez de consultar a base — e o cliente cadastrado aqui
 * vive só na comanda (não vira usuário). Precisa de lookup por CPF/CNPJ + um
 * cadastro leve de cliente de balcão (sem senha/e-mail obrigatórios).
 */
export function CustomerBlock({ customer, onChange }: CustomerBlockProps) {
  const [open, setOpen] = useState(false)
  const [doc, setDoc] = useState('')
  const [name, setName] = useState('')
  const [phone, setPhone] = useState('')
  const [segment, setSegment] = useState<CustomerSegment>('varejo')
  const [error, setError] = useState('')

  function openQuickAdd() {
    setDoc(customer ? maskDocument(customer.document) : '')
    setName(customer?.name ?? '')
    setPhone(customer ? formatPhone(customer.phone) : '')
    setSegment(customer?.segment ?? 'varejo')
    setError('')
    setOpen(true)
  }

  function save() {
    const digitsDoc = doc.replace(/\D/g, '')
    const digitsPhone = phone.replace(/\D/g, '')
    if (!name.trim()) return setError('Informe o nome do cliente.')
    if (digitsDoc.length !== 11 && digitsDoc.length !== 14) {
      return setError('CPF (11 dígitos) ou CNPJ (14 dígitos).')
    }
    // Obrigatório: a Appmax recusa a cobrança sem celular do pagador.
    if (digitsPhone.length < 10) return setError('Telefone é obrigatório para cobrar (Appmax).')

    onChange({ name: name.trim(), document: digitsDoc, phone: digitsPhone, segment })
    setOpen(false)
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
              onClick={openQuickAdd}
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
          onClick={openQuickAdd}
          className="flex min-h-[56px] w-full items-center gap-3 rounded-lg border border-dashed border-gray-300 px-3 text-left hover:border-brand-orange hover:bg-orange-50"
        >
          <Search className="h-5 w-5 shrink-0 text-gray-400" aria-hidden="true" />
          <span>
            <span className="block text-sm font-semibold text-gray-900">Identificar cliente</span>
            <span className="block text-xs text-gray-500">Buscar por CPF/CNPJ ou cadastrar</span>
          </span>
        </button>
      )}

      <Modal open={open} onClose={() => setOpen(false)} title="Cliente" size="sm">
        <div className="flex flex-col gap-3">
          <Input
            label="CPF / CNPJ"
            value={doc}
            inputMode="numeric"
            autoFocus
            onChange={(e) => setDoc(maskDocument(e.target.value))}
            placeholder="000.000.000-00"
            className="h-12 text-base"
          />
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

          {error && (
            <p role="alert" className="text-sm font-semibold text-red-600">
              {error}
            </p>
          )}

          <Button size="lg" fullWidth onClick={save} className="mt-1">
            Salvar cliente
          </Button>
        </div>
      </Modal>
    </div>
  )
}
