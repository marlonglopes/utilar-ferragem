import { TOP_LEVEL_CATEGORIES } from '@/lib/taxonomy'
import type { AdminProductDetail, ProductInput, ProductStatus } from '@/lib/adminProductTypes'

/**
 * Conversão entre o produto do servidor e o estado do formulário.
 *
 * Mora fora do componente para ser testável sem montar tela — `diffInput` é a
 * peça que decide o que é REESCRITO no banco a cada salvamento, e isso merece
 * teste direto, não teste através de um `fireEvent`.
 *
 * O estado do formulário é de STRING, não de número, e isso é deliberado: um
 * campo numérico controlado por `number` não deixa o usuário apagar o conteúdo
 * para digitar outro valor (o `0` volta sozinho e ele fica lutando com o
 * cursor). A conversão acontece na leitura, num ponto só.
 */

export type FormState = Record<string, string>

export function toForm(p: Partial<AdminProductDetail>): FormState {
  const s = (v: unknown): string => (v === null || v === undefined ? '' : String(v))
  return {
    name: s(p.name),
    sku: s(p.sku),
    category: s(p.category) || TOP_LEVEL_CATEGORIES[0].slug,
    brand: s(p.brand),
    description: s(p.description),
    price: s(p.price),
    cost: s(p.cost),
    stock: s(p.stock),
    unitOfMeasure: s(p.unitOfMeasure) || 'un',
    barcode: s(p.barcode),
    weightKg: s(p.weightKg),
    lengthCm: s(p.lengthCm),
    widthCm: s(p.widthCm),
    heightCm: s(p.heightCm),
    ncm: s(p.ncm),
    cfop: s(p.cfop),
    cest: s(p.cest),
    origem: s(p.origem),
    status: s(p.status) || 'draft',
    specs: p.specs ? JSON.stringify(p.specs, null, 2) : '',
  }
}

/** `''` → `undefined` (não mexe no PATCH). Número inválido também. */
export function num(v: string): number | undefined {
  if (v.trim() === '') return undefined
  const n = Number(v.replace(',', '.'))
  return Number.isFinite(n) ? n : undefined
}

export function str(v: string): string | undefined {
  return v.trim() === '' ? undefined : v.trim()
}

/**
 * Envia só o que MUDOU.
 *
 * O PATCH do servidor é parcial (`ausente = não mexe`). Mandar o formulário
 * inteiro a cada salvamento reescreveria todos os campos fiscais com o que
 * estava na tela — inclusive por cima do que outra pessoa alterou enquanto o
 * formulário estava aberto. Diferença contra o estado inicial é o que torna
 * "editei o preço" uma escrita de um campo só.
 */
export function diffInput(initial: FormState, current: FormState): ProductInput {
  const out: ProductInput = {}
  const changed = (k: string): boolean => initial[k] !== current[k]

  if (changed('name')) out.name = current.name.trim()
  if (changed('sku')) out.sku = str(current.sku)
  if (changed('category')) out.category = current.category
  if (changed('brand')) out.brand = str(current.brand)
  if (changed('description')) out.description = current.description
  if (changed('price')) out.price = num(current.price)
  // Custo: string vazia significa "apagar o custo" (null), e não "não mexer" —
  // é a única forma de a tela desfazer um custo cadastrado por engano.
  if (changed('cost')) out.cost = current.cost.trim() === '' ? null : (num(current.cost) ?? null)
  if (changed('stock')) out.stock = num(current.stock)
  if (changed('unitOfMeasure')) out.unitOfMeasure = current.unitOfMeasure
  if (changed('barcode')) out.barcode = str(current.barcode)
  if (changed('weightKg')) out.weightKg = current.weightKg.trim() === '' ? null : (num(current.weightKg) ?? null)
  if (changed('lengthCm')) out.lengthCm = current.lengthCm.trim() === '' ? null : (num(current.lengthCm) ?? null)
  if (changed('widthCm')) out.widthCm = current.widthCm.trim() === '' ? null : (num(current.widthCm) ?? null)
  if (changed('heightCm')) out.heightCm = current.heightCm.trim() === '' ? null : (num(current.heightCm) ?? null)
  if (changed('ncm')) out.ncm = str(current.ncm)
  if (changed('cfop')) out.cfop = str(current.cfop)
  if (changed('cest')) out.cest = str(current.cest)
  if (changed('origem')) out.origem = current.origem.trim() === '' ? null : (num(current.origem) ?? null)
  if (changed('status')) out.status = current.status as ProductStatus
  if (changed('specs')) {
    try {
      const parsed = JSON.parse(current.specs || '{}') as Record<string, string>
      out.specs = parsed
    } catch {
      // Ficha técnica inválida é barrada em `validate()` antes de chegar aqui.
    }
  }
  return out
}
