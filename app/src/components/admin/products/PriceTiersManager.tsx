import { useMemo, useState } from 'react'
import { Plus, Trash2, Check, Loader2, Layers } from 'lucide-react'
import { Section } from '@/components/admin/primitives'
import { formatCurrency } from '@/lib/format'
import { setPriceTiers } from '@/lib/adminProductsApi'
import type { PriceTier } from '@/lib/adminProductTypes'

/**
 * Editor das FAIXAS DE ATACADO ("a partir de N unidades, sai por X"). O backend
 * já existia (PUT /price-tiers) sem nenhuma tela — é o que dava ao cliente
 * profissional o mesmo preço de quem leva 1 unidade. Substitui o conjunto
 * inteiro (não faz merge), espelhando o PUT do servidor.
 *
 * Valida no cliente as MESMAS regras do backend (min>0, sem quantidade
 * duplicada, faixa maior nunca mais cara) para o dono corrigir antes de salvar —
 * mas o servidor continua sendo a autoridade e sua mensagem é mostrada também.
 */

interface Row {
  minQty: string
  price: string
}

function toRows(tiers?: PriceTier[]): Row[] {
  if (!tiers || tiers.length === 0) return []
  return [...tiers]
    .sort((a, b) => a.minQty - b.minQty)
    .map((t) => ({ minQty: String(t.minQty), price: String(t.price) }))
}

export function PriceTiersManager({
  productId,
  initialTiers,
}: {
  productId: string
  initialTiers?: PriceTier[]
}) {
  const [rows, setRows] = useState<Row[]>(toRows(initialTiers))
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [serverError, setServerError] = useState('')

  // Valida e converte as linhas preenchidas. Devolve {tiers, error}. Linhas
  // totalmente vazias são ignoradas (o dono pode ter clicado "adicionar" e
  // desistido); uma linha PELA METADE é erro.
  const { tiers, error } = useMemo(() => validate(rows), [rows])

  const dirty = useMemo(
    () => JSON.stringify(toRows(initialTiers)) !== JSON.stringify(normalizeRows(rows)),
    [rows, initialTiers]
  )

  function patch(i: number, up: Partial<Row>) {
    setSaved(false)
    setServerError('')
    setRows((prev) => prev.map((r, idx) => (idx === i ? { ...r, ...up } : r)))
  }
  function addRow() {
    setSaved(false)
    setRows((prev) => [...prev, { minQty: '', price: '' }])
  }
  function removeRow(i: number) {
    setSaved(false)
    setServerError('')
    setRows((prev) => prev.filter((_, idx) => idx !== i))
  }

  async function save() {
    if (error) return
    setSaving(true)
    setServerError('')
    try {
      await setPriceTiers(productId, tiers)
      setSaved(true)
    } catch (err) {
      setServerError(err instanceof Error ? err.message : 'Falha ao salvar as faixas.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Section
      title="Preço por quantidade (atacado)"
      description="Cada faixa vale “a partir de” N unidades. O maior N que a compra alcança define o unitário."
    >
      <div className="space-y-3 p-3 sm:p-4">
        {rows.length === 0 ? (
          <p className="flex items-center gap-2 text-sm text-gray-500">
            <Layers className="h-4 w-4" aria-hidden="true" />
            Sem faixas de atacado — todo mundo paga o preço unitário do produto.
          </p>
        ) : (
          <ul className="flex flex-col gap-2">
            {rows.map((r, i) => (
              <li key={i} className="flex flex-wrap items-center gap-2">
                <span className="text-sm text-gray-500">a partir de</span>
                <input
                  aria-label={`Quantidade mínima da faixa ${i + 1}`}
                  value={r.minQty}
                  onChange={(e) => patch(i, { minQty: e.target.value.replace(/[^\d.,]/g, '') })}
                  inputMode="decimal"
                  placeholder="qtd"
                  className="h-10 w-24 rounded-md border border-gray-300 px-2 text-sm"
                />
                <span className="text-sm text-gray-500">un. →</span>
                <span className="text-sm text-gray-500">R$</span>
                <input
                  aria-label={`Preço unitário da faixa ${i + 1}`}
                  value={r.price}
                  onChange={(e) => patch(i, { price: e.target.value.replace(/[^\d.,]/g, '') })}
                  inputMode="decimal"
                  placeholder="0,00"
                  className="h-10 w-28 rounded-md border border-gray-300 px-2 text-sm"
                />
                <span className="text-xs text-gray-400">/un.</span>
                <button
                  type="button"
                  onClick={() => removeRow(i)}
                  aria-label={`Remover faixa ${i + 1}`}
                  className="ml-auto rounded p-1.5 text-gray-400 hover:bg-gray-100 hover:text-red-600"
                >
                  <Trash2 className="h-4 w-4" aria-hidden="true" />
                </button>
              </li>
            ))}
          </ul>
        )}

        <button
          type="button"
          onClick={addRow}
          className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 px-3 py-1.5 text-xs font-semibold text-gray-700 hover:bg-gray-50"
        >
          <Plus className="h-3.5 w-3.5" aria-hidden="true" />
          Adicionar faixa
        </button>

        {/* Prévia legível do que foi digitado — o dono confere a tabela negociada. */}
        {tiers.length > 0 && !error && (
          <p className="text-xs text-gray-500">
            {[...tiers]
              .sort((a, b) => a.minQty - b.minQty)
              .map((t) => `${t.minQty}+ un: ${formatCurrency(t.price)}`)
              .join('  ·  ')}
          </p>
        )}

        {error && <p className="text-sm font-semibold text-amber-700">{error}</p>}
        {serverError && (
          <p role="alert" className="text-sm font-semibold text-red-600">
            {serverError}
          </p>
        )}
        {saved && (
          <p className="flex items-center gap-1.5 text-sm font-semibold text-green-700">
            <Check className="h-4 w-4" aria-hidden="true" /> Faixas salvas.
          </p>
        )}

        <div>
          <button
            type="button"
            onClick={() => void save()}
            disabled={saving || !!error || !dirty}
            className="inline-flex items-center gap-2 rounded-md bg-brand-orange px-4 py-2 text-sm font-semibold text-gray-900 hover:bg-brand-orange-dark disabled:opacity-50"
          >
            {saving && <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />}
            Salvar faixas
          </button>
        </div>
      </div>
    </Section>
  )
}

// normalizeRows converte para o formato comparável (só linhas válidas, ordenadas).
function normalizeRows(rows: Row[]): Row[] {
  const { tiers } = validate(rows)
  return toRows(tiers)
}

// validate espelha as regras do backend (admin_catalog.go validateTiers).
function validate(rows: Row[]): { tiers: PriceTier[]; error: string } {
  const tiers: PriceTier[] = []
  const seen = new Set<number>()
  for (const r of rows) {
    const rawQ = r.minQty.trim()
    const rawP = r.price.trim()
    if (rawQ === '' && rawP === '') continue // linha vazia: ignorada
    if (rawQ === '' || rawP === '')
      return { tiers: [], error: 'Preencha quantidade e preço em todas as faixas.' }
    const minQty = Number(rawQ.replace(',', '.'))
    const price = Number(rawP.replace(',', '.'))
    if (!Number.isFinite(minQty) || minQty <= 0)
      return { tiers: [], error: 'A quantidade mínima deve ser maior que zero.' }
    if (!Number.isFinite(price) || price < 0)
      return { tiers: [], error: 'O preço deve ser um valor válido.' }
    if (seen.has(minQty))
      return { tiers: [], error: `Há duas faixas com a mesma quantidade (${minQty}).` }
    seen.add(minQty)
    tiers.push({ minQty, price })
  }
  // Faixa maior não pode custar mais caro que uma menor (senão comprar mais sai
  // mais caro — o oposto do atacado).
  const sorted = [...tiers].sort((a, b) => a.minQty - b.minQty)
  for (let i = 1; i < sorted.length; i++) {
    if (sorted[i].price > sorted[i - 1].price) {
      return {
        tiers: [],
        error: `A faixa de ${sorted[i].minQty}+ não pode ser mais cara que a de ${sorted[i - 1].minQty}+.`,
      }
    }
  }
  return { tiers, error: '' }
}
