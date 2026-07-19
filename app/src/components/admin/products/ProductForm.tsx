import { useCallback, useMemo, useState } from 'react'
import { AlertTriangle, Loader2 } from 'lucide-react'
import { Section } from '@/components/admin/primitives'
import { TOP_LEVEL_CATEGORIES } from '@/lib/taxonomy'
import {
  Field,
  MarginReadout,
  MoneyInput,
  Select,
  TextArea,
  TextInput,
} from '@/components/admin/products/productPrimitives'
import { STATUS_HINT, STATUS_LABEL, isPriceBelowCost } from '@/lib/adminProductFormat'
import { PRODUCT_STATUSES } from '@/lib/adminProductTypes'
import { diffInput, num, toForm, type FormState } from '@/lib/adminProductForm'
import type { AdminProductDetail, ProductInput, ProductStatus } from '@/lib/adminProductTypes'

/**
 * Formulário de produto — serve tanto a edição quanto a criação.
 *
 * O estado é de STRING, não de número, e isso é deliberado: um campo numérico
 * controlado por `number` não deixa o usuário apagar o conteúdo para digitar
 * outro valor (o `0` volta sozinho e ele fica lutando com o cursor). A
 * conversão acontece na leitura, num ponto só.
 */

/** Unidades aceitas pelo `normalizeUnit` do catalog-service. */
const UNITS = ['un', 'cx', 'pc', 'kg', 'g', 'm', 'm2', 'm3', 'l', 'ml', 'sc', 'rl', 'pt', 'br'] as const

/** Origem da mercadoria — tabela do CST, usada na NF-e. */
const ORIGENS: Array<{ value: number; label: string }> = [
  { value: 0, label: '0 — Nacional' },
  { value: 1, label: '1 — Estrangeira, importação direta' },
  { value: 2, label: '2 — Estrangeira, adquirida no mercado interno' },
  { value: 3, label: '3 — Nacional, importação entre 40% e 70%' },
  { value: 4, label: '4 — Nacional, processos produtivos básicos' },
  { value: 5, label: '5 — Nacional, importação abaixo de 40%' },
  { value: 6, label: '6 — Estrangeira, sem similar nacional' },
  { value: 7, label: '7 — Estrangeira, mercado interno sem similar' },
  { value: 8, label: '8 — Nacional, importação acima de 70%' },
]

export interface ProductFormProps {
  /** Ausente = criação. */
  product?: AdminProductDetail
  saving: boolean
  /** Mensagem de erro do servidor — já traduzida pela camada de API. */
  error?: string | null
  onSubmit: (input: ProductInput) => void
  onCancel?: () => void
  submitLabel: string
}

export function ProductForm({
  product,
  saving,
  error,
  onSubmit,
  onCancel,
  submitLabel,
}: ProductFormProps) {
  const initial = useMemo(() => toForm(product ?? {}), [product])
  const [form, setForm] = useState<FormState>(initial)
  const [errors, setErrors] = useState<Record<string, string>>({})
  /** Confirmação explícita para salvar com preço abaixo do custo. */
  const [acceptedLoss, setAcceptedLoss] = useState(false)

  const set = useCallback((key: string, value: string) => {
    setForm((f) => ({ ...f, [key]: value }))
    setErrors((e) => (e[key] ? { ...e, [key]: '' } : e))
  }, [])

  const price = num(form.price) ?? 0
  const cost = form.cost.trim() === '' ? null : (num(form.cost) ?? null)
  const belowCost = isPriceBelowCost(price, cost)

  const validate = useCallback((): boolean => {
    const e: Record<string, string> = {}
    if (form.name.trim() === '') e.name = 'O nome é obrigatório.'
    if (form.category.trim() === '') e.category = 'A categoria é obrigatória.'
    if (form.price.trim() === '' || num(form.price) === undefined) {
      e.price = 'Informe um preço válido.'
    } else if ((num(form.price) ?? 0) < 0) {
      e.price = 'O preço não pode ser negativo.'
    }
    if (form.cost.trim() !== '' && (num(form.cost) ?? -1) < 0) {
      e.cost = 'O custo não pode ser negativo.'
    }
    if (form.stock.trim() !== '' && (num(form.stock) ?? -1) < 0) {
      e.stock = 'O estoque não pode ser negativo.'
    }
    if (form.specs.trim() !== '') {
      try {
        JSON.parse(form.specs)
      } catch {
        e.specs = 'A ficha técnica precisa ser um JSON válido — ex.: {"Potência": "650 W"}.'
      }
    }
    setErrors(e)
    return Object.keys(e).length === 0
  }, [form])

  const submit = useCallback(
    (ev: React.FormEvent) => {
      ev.preventDefault()
      if (!validate()) return
      if (belowCost && !acceptedLoss) return
      onSubmit(product ? diffInput(initial, form) : diffInput(toForm({}), form))
    },
    [validate, belowCost, acceptedLoss, onSubmit, product, initial, form],
  )

  const blocked = belowCost && !acceptedLoss

  return (
    <form onSubmit={submit} className="space-y-4" noValidate>
      {error && (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-md border border-red-200 border-l-4 border-l-red-600 bg-red-50 p-3"
        >
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-red-700" aria-hidden="true" />
          <div className="min-w-0">
            <p className="text-sm font-semibold text-red-800">O produto não foi salvo</p>
            <p className="mt-0.5 text-xs leading-relaxed text-red-700">{error}</p>
          </div>
        </div>
      )}

      {/* ------------------------------------------------ Identificação */}
      <Section title="Identificação" description="O que o cliente vê e o que o estoque usa para achar o item.">
        <div className="grid gap-3 p-3 sm:grid-cols-2 sm:p-4 lg:grid-cols-3">
          <Field label="Nome" htmlFor="p-name" required error={errors.name} className="sm:col-span-2 lg:col-span-3">
            <TextInput
              id="p-name"
              value={form.name}
              onChange={(e) => set('name', e.target.value)}
              placeholder="Furadeira de Impacto Bosch GSB 13 RE 650W"
            />
          </Field>

          <Field label="SKU" htmlFor="p-sku" hint="Código interno da loja." error={errors.sku}>
            <TextInput id="p-sku" value={form.sku} onChange={(e) => set('sku', e.target.value)} />
          </Field>

          <Field label="Código de barras" htmlFor="p-barcode" hint="EAN-8/13. A pontuação é removida." error={errors.barcode}>
            <TextInput
              id="p-barcode"
              inputMode="numeric"
              value={form.barcode}
              onChange={(e) => set('barcode', e.target.value)}
            />
          </Field>

          <Field label="Marca" htmlFor="p-brand">
            <TextInput id="p-brand" value={form.brand} onChange={(e) => set('brand', e.target.value)} />
          </Field>

          <Field label="Categoria" htmlFor="p-category" required error={errors.category}>
            <Select id="p-category" value={form.category} onChange={(e) => set('category', e.target.value)}>
              {TOP_LEVEL_CATEGORIES.map((c) => (
                <option key={c.slug} value={c.slug}>
                  {c.slug}
                </option>
              ))}
            </Select>
          </Field>

          <Field label="Situação" htmlFor="p-status" hint={STATUS_HINT[form.status as ProductStatus]}>
            <Select
              id="p-status"
              value={form.status}
              onChange={(e) => set('status', e.target.value)}
              // Na criação o status é sempre rascunho e o campo não existe —
              // ver `createAdminProduct`.
              disabled={!product}
            >
              {PRODUCT_STATUSES.map((s) => (
                <option key={s} value={s}>
                  {STATUS_LABEL[s]}
                </option>
              ))}
            </Select>
          </Field>

          <Field label="Descrição" htmlFor="p-description" className="sm:col-span-2 lg:col-span-3">
            <TextArea
              id="p-description"
              value={form.description}
              onChange={(e) => set('description', e.target.value)}
            />
          </Field>
        </div>
      </Section>

      {/* ------------------------------------------------ Preço e custo */}
      <Section
        title="Preço, custo e estoque"
        description="A margem é calculada enquanto você digita. É o número que evita cadastrar produto no prejuízo."
      >
        <div className="grid gap-3 p-3 sm:grid-cols-2 sm:p-4 lg:grid-cols-4">
          <Field label="Preço de venda (R$)" htmlFor="p-price" required error={errors.price}>
            <MoneyInput id="p-price" value={form.price} onChange={(e) => set('price', e.target.value)} />
          </Field>

          <Field
            label="Custo (R$)"
            htmlFor="p-cost"
            hint="Visível só no painel. Nunca vai para a loja."
            error={errors.cost}
          >
            <MoneyInput id="p-cost" value={form.cost} onChange={(e) => set('cost', e.target.value)} />
          </Field>

          <Field label="Estoque" htmlFor="p-stock" error={errors.stock}>
            <MoneyInput
              id="p-stock"
              step="0.001"
              value={form.stock}
              onChange={(e) => set('stock', e.target.value)}
            />
          </Field>

          <Field label="Unidade" htmlFor="p-unit" hint="Como o item é vendido.">
            <Select id="p-unit" value={form.unitOfMeasure} onChange={(e) => set('unitOfMeasure', e.target.value)}>
              {UNITS.map((u) => (
                <option key={u} value={u}>
                  {u}
                </option>
              ))}
            </Select>
          </Field>

          <div className="sm:col-span-2 lg:col-span-4">
            <MarginReadout price={price} cost={cost} />
          </div>

          {belowCost && (
            <label className="flex items-start gap-2 sm:col-span-2 lg:col-span-4">
              <input
                type="checkbox"
                checked={acceptedLoss}
                onChange={(e) => setAcceptedLoss(e.target.checked)}
                className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-blue focus:ring-brand-blue"
              />
              <span className="text-xs leading-relaxed text-gray-700">
                Confirmo que a venda abaixo do custo é <strong>intencional</strong> para este produto.
              </span>
            </label>
          )}
        </div>
      </Section>

      {/* ------------------------------------------------ Logística */}
      <Section title="Peso e dimensões" description="Usados pelo cálculo de frete real, em vez de uma estimativa por item.">
        <div className="grid gap-3 p-3 sm:grid-cols-2 sm:p-4 lg:grid-cols-4">
          <Field label="Peso (kg)" htmlFor="p-weight">
            <MoneyInput id="p-weight" step="0.001" value={form.weightKg} onChange={(e) => set('weightKg', e.target.value)} />
          </Field>
          <Field label="Comprimento (cm)" htmlFor="p-length">
            <MoneyInput id="p-length" step="0.1" value={form.lengthCm} onChange={(e) => set('lengthCm', e.target.value)} />
          </Field>
          <Field label="Largura (cm)" htmlFor="p-width">
            <MoneyInput id="p-width" step="0.1" value={form.widthCm} onChange={(e) => set('widthCm', e.target.value)} />
          </Field>
          <Field label="Altura (cm)" htmlFor="p-height">
            <MoneyInput id="p-height" step="0.1" value={form.heightCm} onChange={(e) => set('heightCm', e.target.value)} />
          </Field>
        </div>
      </Section>

      {/* ------------------------------------------------ Fiscal */}
      <Section title="Dados fiscais" description="Vão para a nota. Errar aqui é problema com o contador, não com o cliente.">
        <div className="grid gap-3 p-3 sm:grid-cols-2 sm:p-4 lg:grid-cols-4">
          <Field label="NCM" htmlFor="p-ncm" hint="8 dígitos.">
            <TextInput id="p-ncm" inputMode="numeric" value={form.ncm} onChange={(e) => set('ncm', e.target.value)} />
          </Field>
          <Field label="CFOP" htmlFor="p-cfop" hint="4 dígitos.">
            <TextInput id="p-cfop" inputMode="numeric" value={form.cfop} onChange={(e) => set('cfop', e.target.value)} />
          </Field>
          <Field label="CEST" htmlFor="p-cest" hint="Só para itens com substituição tributária.">
            <TextInput id="p-cest" inputMode="numeric" value={form.cest} onChange={(e) => set('cest', e.target.value)} />
          </Field>
          <Field label="Origem" htmlFor="p-origem">
            <Select id="p-origem" value={form.origem} onChange={(e) => set('origem', e.target.value)}>
              <option value="">—</option>
              {ORIGENS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </Select>
          </Field>
        </div>
      </Section>

      {/* ------------------------------------------------ Ficha técnica */}
      <Section title="Ficha técnica" description="Pares de característica e valor exibidos na página do produto.">
        <div className="p-3 sm:p-4">
          <Field
            label="Especificações (JSON)"
            htmlFor="p-specs"
            hint='Ex.: {"Potência": "650 W", "Mandril": "13 mm"}'
            error={errors.specs}
          >
            <TextArea
              id="p-specs"
              value={form.specs}
              onChange={(e) => set('specs', e.target.value)}
              className="font-mono text-xs"
              spellCheck={false}
            />
          </Field>
        </div>
      </Section>

      {/* ------------------------------------------------ Ações */}
      <div className="sticky bottom-0 flex flex-wrap items-center justify-end gap-2 border-t border-gray-200 bg-white/95 px-3 py-3 backdrop-blur sm:px-4">
        {blocked && (
          <p className="mr-auto text-xs font-medium text-red-700">
            Marque a confirmação acima para salvar com preço abaixo do custo.
          </p>
        )}
        {onCancel && (
          <button
            type="button"
            onClick={onCancel}
            className="rounded-md border border-gray-300 px-3 py-1.5 text-xs font-semibold text-gray-700 hover:bg-gray-50"
          >
            Cancelar
          </button>
        )}
        <button
          type="submit"
          disabled={saving || blocked}
          className="inline-flex items-center gap-2 rounded-md bg-brand-blue px-4 py-2 text-xs font-semibold text-white hover:bg-brand-blue/90 disabled:cursor-not-allowed disabled:bg-gray-300"
        >
          {saving && <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />}
          {saving ? 'Salvando…' : submitLabel}
        </button>
      </div>
    </form>
  )
}

export default ProductForm
