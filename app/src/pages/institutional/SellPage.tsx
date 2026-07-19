import { Link } from 'react-router-dom'
import { Store, TrendingUp, FileCheck, Headphones, ArrowRight } from 'lucide-react'
import { Seo } from '@/components/seo/Seo'
import { COMPANY } from '@/lib/company'

const BENEFITS = [
  {
    icon: Store,
    title: 'Vitrine pronta',
    text: 'Sua ferragem online sem contratar desenvolvedor, sem mensalidade de plataforma e sem cuidar de servidor.',
  },
  {
    icon: TrendingUp,
    title: 'Clientes que já estão comprando',
    text: 'Você entra num catálogo que recebe tráfego de busca. Não precisa construir audiência do zero.',
  },
  {
    icon: FileCheck,
    title: 'Pagamento e repasse organizados',
    text: 'Cuidamos de Pix, boleto e cartão, do antifraude e da conciliação. Você recebe o repasse consolidado.',
  },
  {
    icon: Headphones,
    title: 'Atendimento compartilhado',
    text: 'Nosso time responde a dúvida do cliente. Você foca em separar e despachar o pedido.',
  },
]

const STEPS = [
  {
    title: 'Envie seus dados',
    text: 'CNPJ, inscrição estadual e contato do responsável. Conferimos a situação cadastral na Receita.',
  },
  {
    title: 'Monte seu catálogo',
    text: 'Envie sua lista de produtos em planilha ou cadastre item a item. Ajudamos na primeira carga.',
  },
  {
    title: 'Comece a vender',
    text: 'Seus produtos entram na vitrine. Você recebe o pedido, separa, emite a nota e despacha.',
  },
]

export default function SellPage() {
  return (
    <>
      <Seo
        title="Venda na UtiLar"
        description="Cadastre sua ferragem ou distribuidora na UtiLar Ferragem e venda online sem montar uma loja própria. Vitrine pronta, pagamento integrado e atendimento compartilhado."
        path="/vender"
      />

      {/* Hero */}
      <section className="bg-brand-blue text-white py-16">
        <div className="container max-w-3xl">
          <span className="mb-5 inline-flex items-center gap-1.5 rounded-full bg-white/10 px-3 py-1 text-xs font-semibold text-brand-orange-light">
            Para ferragens e distribuidoras
          </span>
          <h1 className="font-display font-black text-3xl sm:text-4xl leading-tight mb-4">
            Sua ferragem vendendo online,{' '}
            <span className="text-brand-orange">sem virar empresa de tecnologia.</span>
          </h1>
          <p className="text-blue-200 leading-relaxed mb-7 max-w-xl">
            Você entende de material de construção. A gente entende de plataforma. Coloque seu
            estoque na vitrine da UtiLar e receba pedidos de clientes que já estão procurando o que
            você vende.
          </p>
          <a
            href={`mailto:${COMPANY.contact.email}?subject=Quero%20vender%20na%20UtiLar`}
            className="inline-flex items-center gap-2 rounded-lg bg-brand-orange px-5 py-3 font-semibold text-white transition-colors hover:bg-brand-orange-dark"
          >
            Quero cadastrar minha loja
            <ArrowRight className="h-4 w-4" aria-hidden />
          </a>
        </div>
      </section>

      {/* Benefícios */}
      <section className="py-12">
        <div className="container">
          <h2 className="font-display font-black text-2xl text-gray-900 mb-6">
            O que você ganha
          </h2>
          <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-4">
            {BENEFITS.map(({ icon: Icon, title, text }) => (
              <div key={title} className="rounded-xl border border-gray-200 bg-white p-5">
                <span className="mb-3 flex h-10 w-10 items-center justify-center rounded-xl bg-brand-orange-light">
                  <Icon className="h-5 w-5 text-brand-orange" aria-hidden />
                </span>
                <h3 className="font-display font-bold text-sm text-gray-900">{title}</h3>
                <p className="mt-1.5 text-sm leading-relaxed text-gray-500">{text}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Como funciona */}
      <section className="bg-gray-50 py-12">
        <div className="container">
          <h2 className="font-display font-black text-2xl text-gray-900 mb-6">Como funciona</h2>
          <ol className="grid grid-cols-1 gap-5 sm:grid-cols-3">
            {STEPS.map(({ title, text }, i) => (
              <li key={title} className="rounded-xl border border-gray-200 bg-white p-5">
                <span className="mb-3 flex h-8 w-8 items-center justify-center rounded-lg bg-brand-blue font-display font-black text-sm text-white">
                  {i + 1}
                </span>
                <h3 className="font-display font-bold text-sm text-gray-900">{title}</h3>
                <p className="mt-1.5 text-sm leading-relaxed text-gray-500">{text}</p>
              </li>
            ))}
          </ol>
        </div>
      </section>

      {/* Requisitos + CTA */}
      <section className="py-12">
        <div className="container max-w-3xl">
          <h2 className="font-display font-black text-2xl text-gray-900 mb-4">
            O que pedimos de você
          </h2>
          <ul className="mb-8 flex flex-col gap-2.5 text-[15px] text-gray-700">
            {[
              'CNPJ ativo e inscrição estadual regular.',
              'Emissão de nota fiscal eletrônica em toda venda.',
              'Estoque atualizado — anunciar o que não tem gera cancelamento e prejudica sua reputação.',
              'Despacho do pedido dentro do prazo combinado.',
            ].map((req) => (
              <li key={req} className="flex gap-2.5">
                <span className="mt-2 h-1.5 w-1.5 flex-shrink-0 rounded-full bg-brand-orange" />
                {req}
              </li>
            ))}
          </ul>

          <div className="rounded-2xl border border-gray-200 bg-white p-6">
            <h3 className="font-display font-bold text-lg text-gray-900">
              Pronto para começar?
            </h3>
            <p className="mt-2 text-sm leading-relaxed text-gray-600">
              Mande um e-mail com o CNPJ e um resumo do seu sortimento. Retornamos em até 2 dias
              úteis com as condições comerciais e o passo a passo do cadastro.
            </p>
            <div className="mt-5 flex flex-wrap gap-3">
              <a
                href={`mailto:${COMPANY.contact.email}?subject=Quero%20vender%20na%20UtiLar`}
                className="inline-flex items-center gap-2 rounded-lg bg-brand-orange px-5 py-2.5 font-semibold text-white transition-colors hover:bg-brand-orange-dark"
              >
                Falar com o time comercial
              </a>
              <Link
                to="/contato"
                className="inline-flex items-center gap-2 rounded-lg border border-gray-300 px-5 py-2.5 font-semibold text-gray-700 transition-colors hover:bg-gray-50"
              >
                Outros canais
              </Link>
            </div>
            <p className="mt-4 text-xs text-gray-400">
              As condições comerciais e a taxa de intermediação são apresentadas antes de qualquer
              assinatura de contrato.
            </p>
          </div>
        </div>
      </section>
    </>
  )
}
