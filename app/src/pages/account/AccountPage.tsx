import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { User, MapPin, CreditCard, LogOut, Plus, Pencil, Trash2, Check, Package } from 'lucide-react'
import { useAuthStore } from '@/store/authStore'
import { useNavigate } from 'react-router-dom'
import { Input } from '@/components/ui'
import { formatCEP } from '@/lib/format'
import { cn } from '@/lib/cn'
import OrdersTab from './OrdersTab'

type Tab = 'profile' | 'addresses' | 'payment' | 'orders'

interface Address {
  id: string
  label: string
  cep: string
  street: string
  number: string
  complement: string
  neighborhood: string
  city: string
  state: string
  isDefault: boolean
}

const EMPTY_ADDR: Omit<Address, 'id' | 'isDefault'> = {
  label: '',
  cep: '',
  street: '',
  number: '',
  complement: '',
  neighborhood: '',
  city: '',
  state: '',
}

async function fetchCEP(cep: string) {
  const digits = cep.replace(/\D/g, '')
  if (digits.length !== 8) return null
  try {
    const res = await fetch(`https://viacep.com.br/ws/${digits}/json/`)
    const data = await res.json()
    if (data.erro) return null
    return data as { logradouro: string; bairro: string; localidade: string; uf: string }
  } catch {
    return null
  }
}

function ProfileTab() {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const [name, setName] = useState(user?.name ?? '')
  const [saved, setSaved] = useState(false)

  function save(e: React.FormEvent) {
    e.preventDefault()
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  return (
    <form onSubmit={save} className="flex flex-col gap-4 max-w-sm">
      <Input
        label={t('auth.name')}
        value={name}
        onChange={(e) => setName(e.target.value)}
        required
      />
      <Input
        type="email"
        label={t('auth.email')}
        value={user?.email ?? ''}
        disabled
        hint="E-mail não pode ser alterado aqui."
      />
      <button
        type="submit"
        className={cn(
          'h-10 rounded-xl font-semibold text-sm transition-colors',
          saved
            ? 'bg-green-600 text-white'
            : 'bg-brand-orange hover:bg-brand-orange-dark text-white'
        )}
      >
        {saved ? 'Salvo!' : t('account.saveProfile')}
      </button>
    </form>
  )
}

function AddressForm({
  initial,
  onSave,
  onCancel,
}: {
  initial?: Partial<Address>
  onSave: (addr: Omit<Address, 'id' | 'isDefault'>) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const [form, setForm] = useState({ ...EMPTY_ADDR, ...initial })
  const [cepError, setCepError] = useState('')
  const [loadingCep, setLoadingCep] = useState(false)

  function setField(field: keyof typeof form, value: string) {
    setForm((p) => ({ ...p, [field]: value }))
  }

  async function handleCEPBlur() {
    setCepError('')
    setLoadingCep(true)
    const data = await fetchCEP(form.cep)
    setLoadingCep(false)
    if (!data) { setCepError(t('account.cepNotFound')); return }
    setForm((p) => ({
      ...p,
      street: data.logradouro,
      neighborhood: data.bairro,
      city: data.localidade,
      state: data.uf,
    }))
  }

  return (
    <div className="flex flex-col gap-3 border border-gray-200 rounded-xl p-4">
      <Input
        label="Nome do endereço (ex: Casa, Trabalho)"
        value={form.label}
        onChange={(e) => setField('label', e.target.value)}
        placeholder="Casa"
      />
      <Input
        label={t('account.cep')}
        value={form.cep}
        onChange={(e) => setField('cep', formatCEP(e.target.value))}
        onBlur={handleCEPBlur}
        inputMode="numeric"
        maxLength={9}
        placeholder="00000-000"
        error={cepError}
        hint={loadingCep ? 'Buscando...' : undefined}
      />
      <div className="grid grid-cols-3 gap-2">
        <div className="col-span-2">
          <Input label={t('account.street')} value={form.street} onChange={(e) => setField('street', e.target.value)} />
        </div>
        <Input label={t('account.number')} value={form.number} onChange={(e) => setField('number', e.target.value)} />
      </div>
      <Input label={t('account.complement')} value={form.complement} onChange={(e) => setField('complement', e.target.value)} placeholder="Apto, bloco..." />
      <Input label={t('account.neighborhood')} value={form.neighborhood} onChange={(e) => setField('neighborhood', e.target.value)} />
      <div className="grid grid-cols-3 gap-2">
        <div className="col-span-2">
          <Input label={t('account.city')} value={form.city} onChange={(e) => setField('city', e.target.value)} />
        </div>
        <Input label={t('account.state')} value={form.state} onChange={(e) => setField('state', e.target.value)} maxLength={2} />
      </div>
      <div className="flex gap-2 mt-1">
        <button
          onClick={() => onSave(form)}
          className="flex-1 h-9 rounded-lg bg-brand-orange hover:bg-brand-orange-dark text-white text-sm font-semibold transition-colors"
        >
          {t('account.saveAddress')}
        </button>
        <button
          onClick={onCancel}
          className="h-9 px-4 rounded-lg border border-gray-300 text-sm text-gray-600 hover:bg-gray-50 transition-colors"
        >
          {t('account.cancel')}
        </button>
      </div>
    </div>
  )
}

function AddressesTab() {
  const { t } = useTranslation()
  const [addresses, setAddresses] = useState<Address[]>([])
  const [adding, setAdding] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)

  function saveNew(data: Omit<Address, 'id' | 'isDefault'>) {
    setAddresses((p) => [
      ...p,
      { ...data, id: crypto.randomUUID(), isDefault: p.length === 0 },
    ])
    setAdding(false)
  }

  function saveEdit(id: string, data: Omit<Address, 'id' | 'isDefault'>) {
    setAddresses((p) => p.map((a) => (a.id === id ? { ...a, ...data } : a)))
    setEditingId(null)
  }

  function remove(id: string) {
    setAddresses((p) => {
      const next = p.filter((a) => a.id !== id)
      if (next.length > 0 && !next.some((a) => a.isDefault)) next[0].isDefault = true
      return next
    })
  }

  function setDefault(id: string) {
    setAddresses((p) => p.map((a) => ({ ...a, isDefault: a.id === id })))
  }

  return (
    <div className="flex flex-col gap-4 max-w-lg">
      {addresses.length === 0 && !adding && (
        <p className="text-sm text-gray-400">{t('account.noAddresses')}</p>
      )}

      {addresses.map((addr) =>
        editingId === addr.id ? (
          <AddressForm
            key={addr.id}
            initial={addr}
            onSave={(data) => saveEdit(addr.id, data)}
            onCancel={() => setEditingId(null)}
          />
        ) : (
          <div key={addr.id} className="border border-gray-200 rounded-xl p-4 flex flex-col gap-1">
            <div className="flex items-center justify-between">
              <span className="font-semibold text-sm text-gray-900">
                {addr.label || addr.street}
                {addr.isDefault && (
                  <span className="ml-2 text-[10px] font-bold uppercase tracking-wide bg-brand-orange text-white px-1.5 py-0.5 rounded-full">
                    {t('account.defaultAddress')}
                  </span>
                )}
              </span>
              <div className="flex gap-2">
                {!addr.isDefault && (
                  <button onClick={() => setDefault(addr.id)} className="text-gray-400 hover:text-brand-orange transition-colors" title={t('account.setDefault')}>
                    <Check className="h-4 w-4" />
                  </button>
                )}
                <button onClick={() => setEditingId(addr.id)} className="text-gray-400 hover:text-brand-blue transition-colors">
                  <Pencil className="h-4 w-4" />
                </button>
                <button onClick={() => remove(addr.id)} className="text-gray-400 hover:text-red-500 transition-colors">
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>
            </div>
            <p className="text-sm text-gray-600">
              {addr.street}{addr.number ? `, ${addr.number}` : ''}{addr.complement ? ` - ${addr.complement}` : ''}
            </p>
            <p className="text-sm text-gray-500">{addr.neighborhood}, {addr.city} – {addr.state} · {addr.cep}</p>
          </div>
        )
      )}

      {adding ? (
        <AddressForm onSave={saveNew} onCancel={() => setAdding(false)} />
      ) : (
        <button
          onClick={() => setAdding(true)}
          className="flex items-center gap-2 h-10 px-4 rounded-xl border border-dashed border-gray-300 text-sm text-gray-500 hover:border-brand-orange hover:text-brand-orange transition-colors self-start"
        >
          <Plus className="h-4 w-4" />
          {t('account.addAddress')}
        </button>
      )}
    </div>
  )
}

export default function AccountPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const logout = useAuthStore((s) => s.logout)
  const user = useAuthStore((s) => s.user)
  const [tab, setTab] = useState<Tab>('profile')

  const tabs: { id: Tab; label: string; icon: typeof User }[] = [
    { id: 'orders', label: t('account.orders'), icon: Package },
    { id: 'profile', label: t('account.profile'), icon: User },
    { id: 'addresses', label: t('account.addresses'), icon: MapPin },
    { id: 'payment', label: t('account.paymentMethods'), icon: CreditCard },
  ]

  function handleLogout() {
    logout()
    navigate('/')
  }

  return (
    <div className="container py-6">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="font-display font-bold text-2xl text-gray-900">{t('account.title')}</h1>
          <p className="text-sm text-gray-400 mt-0.5">{user?.email}</p>
        </div>
        <button
          onClick={handleLogout}
          className="flex items-center gap-1.5 text-sm text-gray-500 hover:text-red-500 transition-colors"
        >
          <LogOut className="h-4 w-4" />
          {t('auth.logout')}
        </button>
      </div>

      <div className="flex gap-1 border-b border-gray-200 mb-6">
        {tabs.map(({ id, label, icon: Icon }) => (
          <button
            key={id}
            onClick={() => setTab(id)}
            className={cn(
              'flex items-center gap-1.5 px-4 py-2.5 text-sm font-semibold border-b-2 -mb-px transition-colors',
              tab === id
                ? 'border-brand-orange text-brand-orange'
                : 'border-transparent text-gray-500 hover:text-gray-800'
            )}
          >
            <Icon className="h-4 w-4" />
            {label}
          </button>
        ))}
      </div>

      {tab === 'orders' && <OrdersTab />}
      {tab === 'profile' && <ProfileTab />}
      {tab === 'addresses' && <AddressesTab />}
      {tab === 'payment' && (
        <div className="flex flex-col items-center justify-center py-12 gap-3 text-gray-400">
          <CreditCard className="h-10 w-10" />
          <p className="text-sm">{t('account.paymentStub')}</p>
        </div>
      )}
    </div>
  )
}
