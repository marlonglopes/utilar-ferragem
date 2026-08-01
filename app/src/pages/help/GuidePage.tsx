import { Link, useParams } from 'react-router-dom'
import { ArrowLeft, Lightbulb } from 'lucide-react'
import { Seo } from '@/components/seo/Seo'
import { Breadcrumb } from '@/components/ui'
import { findGuide, GUIDES, type Block } from '@/lib/helpGuides'

/** Um guia de operação. Renderiza os blocos (título/parágrafo/passos/dica). */
export default function GuidePage() {
  const { slug = '' } = useParams()
  const guide = findGuide(slug)

  if (!guide) {
    return (
      <div className="container py-12 text-center">
        <Seo title="Guia não encontrado" path={`/ajuda/operacao/${slug}`} noIndex />
        <p className="text-gray-600">Guia não encontrado.</p>
        <Link to="/ajuda/operacao" className="mt-3 inline-block text-brand-orange underline">
          Ver todos os guias
        </Link>
      </div>
    )
  }

  const related = GUIDES.filter((g) => g.category === guide.category && g.slug !== guide.slug)

  return (
    <>
      <Seo
        title={`${guide.title} — guias de operação`}
        description={guide.summary}
        path={`/ajuda/operacao/${guide.slug}`}
      />
      <div className="container py-8">
        <Breadcrumb
          items={[
            { label: 'Início', href: '/' },
            { label: 'Ajuda', href: '/ajuda' },
            { label: 'Guias', href: '/ajuda/operacao' },
            { label: guide.title },
          ]}
          className="mb-6"
        />

        <article className="max-w-2xl">
          <Link
            to="/ajuda/operacao"
            className="mb-4 inline-flex items-center gap-1 text-sm text-gray-500 hover:text-gray-800"
          >
            <ArrowLeft className="h-4 w-4" aria-hidden="true" /> Todos os guias
          </Link>
          <p className="text-xs font-semibold uppercase tracking-wide text-brand-orange">
            {guide.category}
          </p>
          <h1 className="mt-1 font-display text-3xl font-black text-gray-900">{guide.title}</h1>
          <p className="mt-2 text-lg text-gray-600">{guide.summary}</p>

          <div className="mt-6 space-y-4">
            {guide.blocks.map((b, i) => (
              <BlockView key={i} block={b} />
            ))}
          </div>
        </article>

        {related.length > 0 && (
          <div className="mt-10 max-w-2xl border-t border-gray-200 pt-6">
            <h2 className="mb-3 font-display font-bold text-gray-900">
              Também em {guide.category}
            </h2>
            <ul className="space-y-1.5">
              {related.map((g) => (
                <li key={g.slug}>
                  <Link
                    to={`/ajuda/operacao/${g.slug}`}
                    className="text-sm text-brand-orange hover:underline"
                  >
                    {g.title}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </>
  )
}

function BlockView({ block }: { block: Block }) {
  if ('h' in block) {
    return <h2 className="pt-2 font-display text-lg font-bold text-gray-900">{block.h}</h2>
  }
  if ('p' in block) {
    return <p className="leading-relaxed text-gray-700">{block.p}</p>
  }
  if ('steps' in block) {
    return (
      <ol className="list-decimal space-y-1.5 pl-5 text-gray-700">
        {block.steps.map((s, i) => (
          <li key={i} className="leading-relaxed">
            {s}
          </li>
        ))}
      </ol>
    )
  }
  return (
    <div className="flex items-start gap-2 rounded-lg border border-brand-blue-light bg-brand-blue-light/40 p-3">
      <Lightbulb className="mt-0.5 h-4 w-4 shrink-0 text-brand-blue" aria-hidden="true" />
      <p className="text-sm leading-relaxed text-gray-700">{block.tip}</p>
    </div>
  )
}
