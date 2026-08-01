import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Search, ChevronRight } from 'lucide-react'
import { Seo } from '@/components/seo/Seo'
import { Breadcrumb } from '@/components/ui'
import { GUIDE_CATEGORIES, GUIDES, guidesByCategory, type Guide } from '@/lib/helpGuides'

/**
 * Central de GUIAS DE OPERAÇÃO (equipe): como usar balcão e painel. Separada do
 * FAQ do cliente (/ajuda). Deep-linkável — as telas do painel/balcão apontam
 * para os artigos individuais em /ajuda/operacao/<slug>.
 */
export default function GuidesPage() {
  const [q, setQ] = useState('')
  const term = q.trim().toLowerCase()
  const matches = term
    ? GUIDES.filter((g) => `${g.title} ${g.summary} ${g.category}`.toLowerCase().includes(term))
    : null
  const byCat = guidesByCategory()

  return (
    <>
      <Seo
        title="Guias de operação — equipe"
        description="Como usar o balcão (PDV) e o painel da UtiLar: vender, dar desconto, publicar produto, subir fotos em lote, estoque, pedidos, devoluções, frete e financeiro."
        path="/ajuda/operacao"
      />
      <div className="container py-8">
        <Breadcrumb
          items={[
            { label: 'Início', href: '/' },
            { label: 'Ajuda', href: '/ajuda' },
            { label: 'Guias de operação' },
          ]}
          className="mb-6"
        />

        <header className="mb-6 max-w-3xl">
          <h1 className="font-display text-3xl font-black text-gray-900">Guias de operação</h1>
          <p className="mt-2 leading-relaxed text-gray-600">
            Passo a passo das ferramentas da equipe — balcão e painel. Para dúvidas de cliente
            (pedido, entrega, troca), veja a{' '}
            <Link to="/ajuda" className="text-brand-orange underline">
              central de ajuda
            </Link>
            .
          </p>
        </header>

        <div className="relative mb-8 max-w-md">
          <Search
            className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
            aria-hidden="true"
          />
          <input
            type="search"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Buscar um guia (ex.: desconto, foto, estoque)"
            aria-label="Buscar guia"
            className="w-full rounded-lg border border-gray-300 bg-white py-2 pl-9 pr-3 text-sm text-gray-900 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue"
          />
        </div>

        {matches ? (
          matches.length === 0 ? (
            <p className="text-gray-500">Nenhum guia encontrado. Tente outra palavra.</p>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {matches.map((g) => (
                <GuideCard key={g.slug} guide={g} />
              ))}
            </div>
          )
        ) : (
          <div className="space-y-8">
            {GUIDE_CATEGORIES.filter((c) => byCat[c]?.length).map((cat) => (
              <section key={cat}>
                <h2 className="mb-3 font-display text-lg font-bold text-gray-900">{cat}</h2>
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {byCat[cat].map((g) => (
                    <GuideCard key={g.slug} guide={g} />
                  ))}
                </div>
              </section>
            ))}
          </div>
        )}
      </div>
    </>
  )
}

function GuideCard({ guide }: { guide: Guide }) {
  return (
    <Link
      to={`/ajuda/operacao/${guide.slug}`}
      className="group flex items-start justify-between gap-2 rounded-xl border border-gray-200 bg-white p-4 transition-all hover:border-gray-300 hover:shadow-md"
    >
      <span className="min-w-0">
        <span className="block font-semibold text-gray-900">{guide.title}</span>
        <span className="mt-1 block text-sm text-gray-600">{guide.summary}</span>
      </span>
      <ChevronRight
        className="mt-0.5 h-4 w-4 shrink-0 text-gray-300 transition-colors group-hover:text-brand-orange"
        aria-hidden="true"
      />
    </Link>
  )
}
