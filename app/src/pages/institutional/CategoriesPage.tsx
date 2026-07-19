import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Seo } from '@/components/seo/Seo'
import { Breadcrumb } from '@/components/ui'
import { TOP_LEVEL_CATEGORIES } from '@/lib/taxonomy'
import { breadcrumbListSchema } from '@/lib/seo'

// Descrições curtas por categoria — servem tanto ao usuário quanto ao SEO
// (texto indexável numa página que, sem isso, seria só um grid de ícones).
const DESCRIPTIONS: Record<string, string> = {
  ferramentas:
    'Furadeiras, parafusadeiras, serras, esmerilhadeiras e ferramentas manuais das principais marcas.',
  construcao:
    'Cimento, argamassa, areia, blocos, telhas e tudo que a obra consome do alicerce ao acabamento.',
  eletrica:
    'Cabos, disjuntores, quadros, tomadas, interruptores e material de instalação elétrica.',
  hidraulica:
    'Tubos e conexões de PVC e CPVC, registros, torneiras, caixas d’água e material de esgoto.',
  pintura:
    'Tintas acrílicas e esmaltes, massa corrida, texturas, pincéis, rolos, lixas e fitas.',
  jardim:
    'Mangueiras, aspersores, podadores, carrinhos de mão, terra adubada e ferramentas de jardinagem.',
  seguranca:
    'Capacetes, luvas, óculos, botinas, protetores auriculares e sinalização de obra.',
  fixacao:
    'Parafusos, buchas, pregos, arruelas, abraçadeiras, âncoras químicas e fitas de alta fixação.',
}

export default function CategoriesPage() {
  const { t } = useTranslation()

  return (
    <>
      <Seo
        title="Todas as categorias"
        description="Navegue por todas as categorias da UtiLar Ferragem: ferramentas, construção, elétrica, hidráulica, pintura, jardim, segurança e fixação."
        path="/categorias"
        jsonLd={breadcrumbListSchema([
          { name: 'Início', path: '/' },
          { name: 'Categorias' },
        ])}
      />

      <div className="container py-8">
        <Breadcrumb
          items={[{ label: 'Início', href: '/' }, { label: 'Categorias' }]}
          className="mb-6"
        />

        <header className="mb-8 max-w-2xl">
          <h1 className="font-display font-black text-3xl text-gray-900">Todas as categorias</h1>
          <p className="mt-3 text-gray-600 leading-relaxed">
            Todo o catálogo da UtiLar organizado por tipo de material. Escolha uma categoria para
            ver os produtos disponíveis, filtrar por marca e faixa de preço.
          </p>
        </header>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {TOP_LEVEL_CATEGORIES.map((cat) => (
            <Link
              key={cat.slug}
              to={`/categoria/${cat.slug}`}
              className="group flex gap-4 rounded-xl border border-gray-200 bg-white p-5 transition-all hover:border-brand-orange hover:shadow-sm"
            >
              <span className="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl bg-brand-orange-light text-2xl select-none">
                {cat.icon}
              </span>
              <div className="min-w-0">
                <h2 className="font-display font-bold text-base text-gray-900 transition-colors group-hover:text-brand-orange">
                  {t(cat.labelKey)}
                </h2>
                <p className="mt-1 text-sm leading-relaxed text-gray-500">
                  {DESCRIPTIONS[cat.slug]}
                </p>
              </div>
            </Link>
          ))}
        </div>
      </div>
    </>
  )
}
