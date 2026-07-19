import { Link } from 'react-router-dom'
import { formatCurrency } from '@/lib/format'
import type { LaraProduct } from '@/lib/alice'
import type { LaraMessage } from '@/hooks/useAlice'
import { AvisosSeguranca } from './AvisoSeguranca'
import { MaterialList } from './MaterialList'
import { ComplementSuggestions } from './ComplementSuggestion'

export function LaraProductCard({
  product,
  onNavigate,
}: {
  product: LaraProduct
  onNavigate?: () => void
}) {
  return (
    <Link
      to={`/produto/${product.slug}`}
      onClick={onNavigate}
      className="flex items-center justify-between gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm hover:border-brand-orange hover:bg-orange-50"
    >
      <span className="line-clamp-1 text-gray-800">{product.name}</span>
      <span className="whitespace-nowrap font-semibold text-brand-blue">
        {formatCurrency(product.price)}
      </span>
    </Link>
  )
}

/**
 * Uma mensagem da conversa, com tudo que pode vir anexado a ela.
 *
 * ORDEM IMPORTA: os avisos de segurança vêm ANTES do texto. Um aviso depois de
 * três parágrafos e uma tabela de preços é um aviso que ninguém lê.
 *
 * Compartilhado entre a bolha da loja (AliceBubble) e o painel do balcão
 * (AliceBalcao) — o layout muda, o conteúdo e as regras não.
 */
export function AliceMessage({
  message,
  onNavigate,
  maxProducts = 4,
}: {
  message: LaraMessage
  onNavigate?: () => void
  maxProducts?: number
}) {
  const isUser = message.role === 'user'

  return (
    <div className="space-y-2">
      {!isUser && <AvisosSeguranca avisos={message.avisos} />}

      {message.text && (
        <div
          className={
            isUser
              ? 'ml-auto max-w-[85%] rounded-2xl rounded-br-sm bg-brand-orange px-3 py-2 text-sm text-white'
              : 'mr-auto max-w-[85%] whitespace-pre-wrap rounded-2xl rounded-bl-sm bg-gray-100 px-3 py-2 text-sm text-gray-800'
          }
        >
          {message.text}
        </div>
      )}

      {message.materiais && message.materiais.length > 0 && (
        <MaterialList itens={message.materiais} onNavigate={onNavigate} />
      )}

      <ComplementSuggestions complementos={message.complementos} />

      {message.products && message.products.length > 0 && (
        <div className="space-y-1.5">
          {message.products.slice(0, maxProducts).map((p) => (
            <LaraProductCard key={p.slug} product={p} onNavigate={onNavigate} />
          ))}
        </div>
      )}
    </div>
  )
}

/** Badge discreto do modo vendedor — a UI muda de contexto, o operador precisa ver. */
export function ModoBadge({ mode }: { mode?: string }) {
  if (mode !== 'vendedor') return null
  return (
    <span className="rounded-full bg-brand-gold px-2 py-0.5 font-display text-[10px] font-bold uppercase tracking-wide text-brand-blue">
      balcão
    </span>
  )
}
