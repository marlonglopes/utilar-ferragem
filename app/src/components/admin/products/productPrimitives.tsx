import type { ReactNode } from 'react'
import { AlertTriangle } from 'lucide-react'
import { cn } from '@/lib/cn'
import { SEVERITY_TEXT } from '@/components/admin/tokens'
import { formatCents, formatPercent } from '@/lib/adminFormat'
import { STATUS_LABEL, STATUS_PILL, readMargin } from '@/lib/adminProductFormat'
import type { ProductStatus } from '@/lib/adminProductTypes'

/**
 * Primitivas da gestão de produtos: o chip de status e a leitura de margem.
 *
 * A leitura de margem é o componente central desta feature. Ela aparece na
 * lista (compacta, por linha) e no formulário (grande, ao vivo enquanto o dono
 * digita preço e custo) — e as duas versões precisam dar o MESMO veredito,
 * pelo mesmo cálculo, ou a tela contradiz a si mesma.
 */

export function StatusPill({ status }: { status: ProductStatus }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset',
        STATUS_PILL[status],
      )}
    >
      {STATUS_LABEL[status]}
    </span>
  )
}

/**
 * Reais a partir de um valor em REAIS (não centavos).
 *
 * O catálogo trafega preço e custo como `float64` de reais — é o contrato do
 * catalog-service (`price NUMERIC`), diferente do payment-service, que trabalha
 * em centavos inteiros. Converter aqui, no ponto de exibição, mantém o resto do
 * painel usando o mesmo `formatCents` de sempre e evita duas formatações de
 * dinheiro convivendo na mesma tela.
 */
export function Reais({ value, className }: { value: number | null | undefined; className?: string }) {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return <span className={cn('text-gray-400', className)}>—</span>
  }
  return (
    <span className={cn('tabular-nums', value < 0 && 'text-red-700', className)}>
      {formatCents(Math.round(value * 100))}
    </span>
  )
}

/**
 * Margem em uma célula de tabela.
 *
 * Sem custo cadastrado mostra "—" com um título explicativo, e **não** 0%:
 * "0,0%" seria lido como "este produto não dá lucro", que é uma afirmação que
 * ninguém fez — o que existe é ausência de dado.
 */
export function MarginCell({ price, cost }: { price: number; cost: number | null | undefined }) {
  const m = readMargin(price, cost)
  if (m.unknown || m.fraction === null) {
    return (
      <span className="text-gray-400" title="Custo não cadastrado — a margem não pode ser calculada">
        —
      </span>
    )
  }
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 font-semibold tabular-nums',
        m.severity ? SEVERITY_TEXT[m.severity] : 'text-gray-700',
      )}
    >
      {m.belowCost && <AlertTriangle className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />}
      {formatPercent(m.fraction)}
    </span>
  )
}

/**
 * Leitura de margem ao vivo, para o formulário de edição.
 *
 * É o número que evita cadastrar produto no prejuízo, então ele fica visível
 * enquanto o preço e o custo são digitados — não atrás de um botão "calcular",
 * nem só depois de salvar. Mostra margem, lucro unitário e, quando o preço cai
 * abaixo do custo, um aviso explícito.
 *
 * O aviso NÃO bloqueia o salvamento. Venda abaixo do custo existe de verdade
 * (queima de estoque, item de isca, erro de tabela do fornecedor que a loja
 * decide absorver) e a importação também não bloqueia: ela **retém para
 * revisão humana**. O equivalente aqui é obrigar a ver o aviso antes de
 * confirmar — que é o que a tela de edição faz.
 */
export function MarginReadout({
  price,
  cost,
  className,
}: {
  price: number
  cost: number | null | undefined
  className?: string
}) {
  const m = readMargin(price, cost)

  return (
    <div
      className={cn(
        'rounded-lg border p-3',
        m.belowCost ? 'border-red-300 bg-red-50' : 'border-gray-200 bg-gray-50',
        className,
      )}
      // `polite` e não `assertive`: o valor muda a cada tecla digitada no campo
      // de preço, e um leitor de tela interrompendo a digitação a cada dígito
      // tornaria o formulário inutilizável para quem depende dele.
      aria-live="polite"
      data-testid="margin-readout"
    >
      <div className="flex flex-wrap items-baseline gap-x-6 gap-y-2">
        <div>
          <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Margem</p>
          <p
            className={cn(
              'font-display text-2xl font-bold tabular-nums',
              m.fraction === null
                ? 'text-gray-400'
                : m.severity
                  ? SEVERITY_TEXT[m.severity]
                  : 'text-gray-900',
            )}
            data-testid="margin-value"
          >
            {m.fraction === null ? '—' : formatPercent(m.fraction)}
          </p>
        </div>
        <div>
          <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Lucro por unidade</p>
          <p className="font-display text-lg font-semibold text-gray-900">
            <Reais value={m.profit} />
          </p>
        </div>
      </div>

      {m.unknown && (
        <p className="mt-2 text-xs text-gray-600">
          Preencha o <strong>custo</strong> para ver a margem. Sem ele, a loja vende no escuro.
        </p>
      )}

      {m.belowCost && (
        <p
          role="alert"
          className="mt-2 flex items-start gap-2 text-xs font-medium leading-relaxed text-red-800"
        >
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span>
            <strong>Preço abaixo do custo.</strong> Cada unidade vendida dá prejuízo de{' '}
            <Reais value={Math.abs(m.profit ?? 0)} className="font-semibold" />. A importação de
            planilha retém linhas assim para revisão — confirme que é intencional antes de salvar.
          </span>
        </p>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Campos de formulário
// ---------------------------------------------------------------------------

const CONTROL =
  'w-full rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue disabled:bg-gray-50 disabled:text-gray-500'

export function Field({
  label,
  hint,
  htmlFor,
  required,
  error,
  children,
  className,
}: {
  label: string
  hint?: string
  htmlFor: string
  required?: boolean
  error?: string
  children: ReactNode
  className?: string
}) {
  return (
    <div className={cn('min-w-0', className)}>
      <label htmlFor={htmlFor} className="block text-xs font-semibold text-gray-700">
        {label}
        {required && <span className="ml-0.5 text-red-600">*</span>}
      </label>
      <div className="mt-1">{children}</div>
      {error ? (
        <p className="mt-1 text-xs font-medium text-red-700">{error}</p>
      ) : (
        hint && <p className="mt-1 text-xs text-gray-500">{hint}</p>
      )}
    </div>
  )
}

export function TextInput({
  className,
  ...rest
}: React.InputHTMLAttributes<HTMLInputElement>) {
  return <input className={cn(CONTROL, className)} {...rest} />
}

export function TextArea({
  className,
  ...rest
}: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea className={cn(CONTROL, 'min-h-24 resize-y', className)} {...rest} />
}

export function Select({
  className,
  children,
  ...rest
}: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select className={cn(CONTROL, className)} {...rest}>
      {children}
    </select>
  )
}

/**
 * Campo de dinheiro.
 *
 * `inputMode="decimal"` abre o teclado numérico no celular — o dono vai
 * conferir preço no telefone, e um teclado alfabético ali é um imposto a cada
 * edição.
 *
 * `autoComplete="off"` no campo de CUSTO especificamente: o preenchimento
 * automático do navegador guarda valores digitados em formulário, e custo de
 * fornecedor não pode ficar no perfil do Chrome da máquina da loja.
 */
export function MoneyInput({
  className,
  ...rest
}: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      type="number"
      step="0.01"
      min="0"
      inputMode="decimal"
      autoComplete="off"
      className={cn(CONTROL, 'tabular-nums', className)}
      {...rest}
    />
  )
}
