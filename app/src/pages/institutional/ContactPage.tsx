import { Mail, Phone, MapPin, Clock, MessageCircle, ShieldCheck } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Seo } from '@/components/seo/Seo'
import { Breadcrumb } from '@/components/ui'
import { COMPANY, formattedAddress } from '@/lib/company'

const CHANNELS = [
  {
    icon: Mail,
    title: 'E-mail',
    value: COMPANY.contact.email,
    detail: 'Respondemos em até 1 dia útil. Melhor canal para assuntos com número de pedido.',
  },
  {
    icon: MessageCircle,
    title: 'WhatsApp',
    value: COMPANY.contact.whatsapp,
    detail: 'Para dúvidas rápidas sobre produto, prazo ou status de entrega.',
  },
  {
    icon: Phone,
    title: 'Telefone',
    value: COMPANY.contact.phone,
    detail: COMPANY.contact.hours,
  },
  {
    icon: ShieldCheck,
    title: 'Privacidade e dados (DPO)',
    value: COMPANY.contact.dpoEmail,
    detail: 'Canal exclusivo para exercer seus direitos de titular previstos na LGPD.',
  },
]

export default function ContactPage() {
  return (
    <>
      <Seo
        title="Contato"
        description="Fale com o atendimento da UtiLar Ferragem por e-mail, WhatsApp ou telefone. Suporte a pedidos, trocas, devoluções e privacidade de dados."
        path="/contato"
      />

      <div className="container py-8">
        <Breadcrumb items={[{ label: 'Início', href: '/' }, { label: 'Contato' }]} className="mb-6" />

        <header className="mb-8 max-w-3xl">
          <h1 className="font-display font-black text-3xl text-gray-900">Fale com a gente</h1>
          <p className="mt-3 text-gray-600 leading-relaxed">
            Escolha o canal que fizer mais sentido. Se o assunto for um pedido específico, tenha o
            número dele em mãos — resolvemos bem mais rápido.
          </p>
        </header>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 max-w-3xl">
          {CHANNELS.map(({ icon: Icon, title, value, detail }) => (
            <div key={title} className="rounded-xl border border-gray-200 bg-white p-5">
              <div className="flex items-center gap-2.5 mb-2">
                <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-brand-orange-light">
                  <Icon className="h-4 w-4 text-brand-orange" aria-hidden />
                </span>
                <h2 className="font-display font-bold text-sm text-gray-900">{title}</h2>
              </div>
              <p className="text-sm font-semibold text-gray-900 break-words">{value}</p>
              <p className="mt-1.5 text-sm text-gray-500 leading-relaxed">{detail}</p>
            </div>
          ))}
        </div>

        <div className="mt-6 max-w-3xl rounded-xl border border-gray-200 bg-white p-5">
          <div className="flex items-center gap-2.5 mb-3">
            <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-brand-orange-light">
              <MapPin className="h-4 w-4 text-brand-orange" aria-hidden />
            </span>
            <h2 className="font-display font-bold text-sm text-gray-900">Endereço</h2>
          </div>
          <p className="text-sm text-gray-700">{formattedAddress()}</p>
          <p className="mt-2 text-sm text-gray-700">
            <strong className="text-gray-900">{COMPANY.legalName}</strong> — CNPJ {COMPANY.cnpj}
          </p>
          <p className="mt-4 flex items-center gap-2 text-sm text-gray-500">
            <Clock className="h-4 w-4 flex-shrink-0" aria-hidden />
            {COMPANY.contact.hours}
          </p>
        </div>

        <div className="mt-8 max-w-3xl rounded-xl bg-gray-50 border border-gray-200 p-5">
          <h2 className="font-display font-bold text-sm text-gray-900 mb-2">
            Antes de escrever, veja se já respondemos
          </h2>
          <p className="text-sm text-gray-600 leading-relaxed">
            Prazo de entrega, troca, devolução, formas de pagamento e status do pedido estão
            explicados na <Link to="/ajuda" className="text-brand-orange underline">Central de
            ajuda</Link>. Condições de compra e o direito de arrependimento de 7 dias estão nos{' '}
            <Link to="/termos" className="text-brand-orange underline">Termos de Uso</Link>.
          </p>
        </div>
      </div>
    </>
  )
}
