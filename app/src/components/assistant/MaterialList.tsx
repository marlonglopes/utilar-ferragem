import { Link } from 'react-router-dom'
import { formatCurrency, formatNumber } from '@/lib/format'
import type { MaterialItem } from '@/lib/alice'
import { MemoriaCalculo } from './MemoriaCalculo'
import { AddAllToCart } from './AddAllToCart'
import { subtotalItem, totalLista } from './helpers'

function qtd(item: MaterialItem): string {
  return `${formatNumber(item.quantidade, undefined, { maximumFractionDigits: 3 })} ${item.unidBase}`
}

function compra(item: MaterialItem): string {
  return `${formatNumber(item.embalagens)} × ${item.unidVenda}`
}

/** Rótulo de coluna visível só no layout de cards (telas estreitas). */
function Rotulo({ children }: { children: string }) {
  return (
    <span aria-hidden="true" className="text-[11px] font-semibold text-gray-500 md:hidden">
      {children}
    </span>
  )
}

/**
 * Lista de materiais calculada pela Alice.
 *
 * Uma linha por material: quantidade consumida, o que comprar, o produto casado
 * no catálogo, preço unitário e subtotal — mais a memória de cálculo de cada
 * linha, recolhida.
 *
 * Responsividade: é uma <table> de verdade em md+, e em tela estreita as
 * mesmas linhas viram cards empilhados (`block`), em vez de uma tabela
 * espremida e ilegível. DOM único, sem duplicar conteúdo.
 *
 * Item sem produto casado mostra a quantidade e diz que não há preço. Nunca um
 * valor inventado: o cliente compraria em cima dele.
 */
export function MaterialList({
  itens,
  titulo = 'Lista de materiais',
  onNavigate,
}: {
  itens: MaterialItem[]
  titulo?: string
  onNavigate?: () => void
}) {
  if (itens.length === 0) return null

  const total = totalLista(itens)
  const semPreco = itens.filter((i) => !i.produto).length

  return (
    <section
      aria-label={titulo}
      className="rounded-xl border border-gray-200 bg-white p-3 shadow-sm"
    >
      <h3 className="mb-2 font-display text-sm font-bold text-brand-blue">{titulo}</h3>

      <table className="w-full block md:table">
        <caption className="sr-only">
          Materiais calculados, com quantidade, produto, preço unitário e subtotal.
        </caption>
        <thead className="hidden md:table-header-group">
          <tr className="text-left text-[11px] uppercase tracking-wide text-gray-500">
            <th scope="col" className="pb-1 pr-2 font-semibold">
              Quantidade
            </th>
            <th scope="col" className="pb-1 pr-2 font-semibold">
              Unidade
            </th>
            <th scope="col" className="pb-1 pr-2 font-semibold">
              Produto
            </th>
            <th scope="col" className="pb-1 pr-2 text-right font-semibold">
              Preço unit.
            </th>
            <th scope="col" className="pb-1 text-right font-semibold">
              Subtotal
            </th>
          </tr>
        </thead>

        <tbody className="block md:table-row-group">
          {itens.map((item) => {
            const sub = subtotalItem(item)
            return (
              <tr
                key={item.materialId}
                className="mb-2 block rounded-lg border border-gray-200 p-2 text-[13px] last:mb-0 md:mb-0 md:table-row md:rounded-none md:border-0 md:border-t md:border-gray-100 md:p-0"
              >
                <td className="flex items-baseline justify-between gap-2 py-0.5 md:table-cell md:py-2 md:pr-2 md:align-top">
                  <Rotulo>Quantidade</Rotulo>
                  <span className="font-semibold text-gray-900">{qtd(item)}</span>
                </td>

                <td className="flex items-baseline justify-between gap-2 py-0.5 md:table-cell md:py-2 md:pr-2 md:align-top">
                  <Rotulo>Comprar</Rotulo>
                  <span className="whitespace-nowrap text-gray-700">{compra(item)}</span>
                </td>

                <td className="flex items-baseline justify-between gap-2 py-0.5 md:table-cell md:py-2 md:pr-2 md:align-top">
                  <Rotulo>Produto</Rotulo>
                  {item.produto ? (
                    <Link
                      to={`/produto/${item.produto.slug}`}
                      onClick={onNavigate}
                      className="text-right text-brand-blue underline-offset-2 hover:underline md:text-left"
                    >
                      {item.produto.nome}
                    </Link>
                  ) : (
                    <span className="text-right text-gray-700 md:text-left">
                      {item.nome}
                      <span className="ml-1 whitespace-nowrap rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-600">
                        sem produto no catálogo
                      </span>
                    </span>
                  )}
                </td>

                <td className="flex items-baseline justify-between gap-2 py-0.5 md:table-cell md:py-2 md:pr-2 md:text-right md:align-top">
                  <Rotulo>Preço unit.</Rotulo>
                  {item.produto ? (
                    <span className="whitespace-nowrap text-gray-900">
                      {formatCurrency(item.produto.preco)}
                    </span>
                  ) : (
                    <span className="whitespace-nowrap text-gray-500">sem preço</span>
                  )}
                </td>

                <td className="flex items-baseline justify-between gap-2 py-0.5 md:table-cell md:py-2 md:text-right md:align-top">
                  <Rotulo>Subtotal</Rotulo>
                  {sub !== null ? (
                    <span className="whitespace-nowrap font-semibold text-gray-900">
                      {formatCurrency(sub)}
                    </span>
                  ) : (
                    <span className="whitespace-nowrap text-gray-500">—</span>
                  )}
                </td>

                <td className="mt-1.5 block md:table-cell md:py-2 md:pl-2 md:align-top">
                  <MemoriaCalculo item={item} />
                </td>
              </tr>
            )
          })}
        </tbody>

        <tfoot className="block md:table-footer-group">
          <tr className="mt-2 flex items-baseline justify-between border-t-2 border-brand-blue/20 pt-2 md:table-row">
            <th
              scope="row"
              colSpan={4}
              className="font-display text-sm font-bold text-gray-900 md:table-cell md:py-2 md:text-right md:pr-2"
            >
              Total
            </th>
            <td
              className="font-display text-sm font-bold text-brand-blue md:table-cell md:py-2 md:text-right"
              data-testid="material-total"
            >
              {formatCurrency(total)}
            </td>
            <td className="hidden md:table-cell" />
          </tr>
        </tfoot>
      </table>

      {semPreco > 0 && (
        <p className="mt-2 text-[12px] text-gray-600">
          O total cobre só os {itens.length - semPreco} itens com produto no catálogo.{' '}
          {semPreco === 1 ? '1 item ficou' : `${semPreco} itens ficaram`} sem preço — fale com um
          vendedor para orçar.
        </p>
      )}

      <div className="mt-3">
        <AddAllToCart itens={itens} />
      </div>
    </section>
  )
}
