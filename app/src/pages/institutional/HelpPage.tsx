import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronDown } from 'lucide-react'
import { Seo } from '@/components/seo/Seo'
import { Breadcrumb } from '@/components/ui'
import { cn } from '@/lib/cn'
import { COMPANY } from '@/lib/company'

interface Faq {
  q: string
  a: string
}

interface FaqSection {
  title: string
  items: Faq[]
}

// As respostas espelham o que os Termos de Uso dizem. Ao alterar prazos aqui,
// altere também em TermsPage.tsx — divergência entre os dois vira risco de Procon.
const SECTIONS: FaqSection[] = [
  {
    title: 'Pedidos',
    items: [
      {
        q: 'Como acompanho meu pedido?',
        a: 'Entre na sua conta e abra "Meus pedidos". Lá aparece a situação atual — aguardando pagamento, em separação, enviado ou entregue — e o código de rastreamento assim que a transportadora coleta a mercadoria.',
      },
      {
        q: 'Posso cancelar um pedido?',
        a: 'Sim, sem custo nenhum enquanto o pedido não tiver sido despachado. Cancele pela sua conta ou fale com o atendimento. Se o pedido já saiu para entrega, o caminho é o direito de arrependimento de 7 dias.',
      },
      {
        q: 'Recebi um produto errado ou danificado. E agora?',
        a: 'Avise em até 7 dias corridos do recebimento, de preferência com fotos do item e da embalagem. A coleta e o reenvio são por nossa conta. Se a caixa chegou visivelmente avariada, recuse a entrega e anote a ocorrência no comprovante do entregador.',
      },
      {
        q: 'Vem nota fiscal?',
        a: 'Sempre. Todo vendedor parceiro tem CNPJ verificado e emite nota fiscal eletrônica. Guarde-a: ela é indispensável para acionar garantia do fabricante.',
      },
    ],
  },
  {
    title: 'Pagamento',
    items: [
      {
        q: 'Quais formas de pagamento vocês aceitam?',
        a: 'Pix, boleto bancário e cartão de crédito. As condições de parcelamento disponíveis aparecem no checkout antes de você confirmar a compra.',
      },
      {
        q: 'Quanto tempo leva para o pagamento ser confirmado?',
        a: 'O Pix é confirmado em poucos minutos. O boleto costuma levar de 1 a 3 dias úteis, conforme a compensação bancária. O cartão é analisado na hora pela operadora. O pedido só vai para separação depois da confirmação.',
      },
      {
        q: 'É seguro digitar meu cartão aqui?',
        a: 'Sim. Os dados do cartão são digitados diretamente no ambiente do nosso provedor de pagamento, certificado PCI-DSS, e não trafegam nem ficam armazenados nos nossos servidores. Guardamos apenas bandeira, quatro últimos dígitos e o identificador da transação.',
      },
      {
        q: 'Meu boleto venceu. O que acontece?',
        a: 'O pedido é cancelado automaticamente e o estoque volta para a vitrine. Nenhum valor é cobrado. Se ainda quiser o produto, é só refazer o pedido.',
      },
    ],
  },
  {
    title: 'Entrega',
    items: [
      {
        q: 'Qual o prazo de entrega?',
        a: 'O prazo aparece no checkout, calculado para o seu CEP. Ele começa a contar da confirmação do pagamento — não da data do pedido — e soma o tempo de separação do vendedor com o tempo de transporte.',
      },
      {
        q: 'Quanto custa o frete?',
        a: 'Depende do CEP, do peso e das dimensões dos itens. O valor é calculado e exibido antes de você confirmar o pedido, nunca depois.',
      },
      {
        q: 'Preciso estar em casa para receber?',
        a: 'Sim, alguém precisa receber e conferir a mercadoria. Se ninguém estiver no local, a transportadora costuma tentar novamente; após as tentativas previstas, o produto volta para o remetente.',
      },
      {
        q: 'Vocês entregam em todo o Brasil?',
        a: 'A cobertura varia conforme o vendedor e o produto. Informe seu CEP na página do produto ou no carrinho para ver se há entrega disponível e qual o prazo.',
      },
    ],
  },
  {
    title: 'Trocas e devoluções',
    items: [
      {
        q: 'Mudei de ideia. Posso devolver?',
        a: 'Pode. O art. 49 do Código de Defesa do Consumidor garante o direito de arrependimento em até 7 dias corridos do recebimento, sem precisar justificar. O produto deve voltar com acessórios, manuais e embalagem original, e os custos da devolução são por nossa conta.',
      },
      {
        q: 'Existe algum produto que não posso devolver por arrependimento?',
        a: 'Itens cortados, misturados ou feitos sob medida a seu pedido, sacarias de cimento e argamassa já abertas e itens de uso pessoal vedados por higiene ou segurança. Isso não afeta a garantia legal em caso de defeito.',
      },
      {
        q: 'O produto apresentou defeito depois de um tempo. Tenho garantia?',
        a: 'Sim. A garantia legal é de 30 dias para produtos não duráveis e 90 dias para duráveis, contados do recebimento. Muitos itens ainda têm garantia adicional do fabricante, indicada na embalagem ou no manual.',
      },
      {
        q: 'Em quanto tempo recebo meu dinheiro de volta?',
        a: 'Pix e boleto são devolvidos por depósito em conta de sua titularidade em até 10 dias úteis após a conferência da devolução. No cartão, solicitamos o estorno à operadora em até 10 dias úteis e o crédito aparece na fatura seguinte ou na subsequente, conforme a data de fechamento definida pelo emissor.',
      },
    ],
  },
  {
    title: 'Conta e dados',
    items: [
      {
        q: 'Esqueci minha senha.',
        a: 'Use o link "Esqueci minha senha" na tela de entrada. Enviamos um link de redefinição para o e-mail cadastrado.',
      },
      {
        q: 'Como excluo minha conta e meus dados?',
        a: `Escreva para ${COMPANY.contact.dpoEmail}. Respondemos em até 15 dias. Alguns dados precisam ser mantidos por prazo legal — notas fiscais por 5 anos e registros de acesso por 6 meses —, e explicamos exatamente o que fica retido e por quê.`,
      },
      {
        q: 'Vocês vendem meus dados?',
        a: 'Não. Compartilhamos apenas o necessário para a compra funcionar: o vendedor recebe seu endereço para enviar, a transportadora para entregar e o provedor de pagamento para cobrar. Os detalhes estão na Política de Privacidade.',
      },
    ],
  },
]

function FaqItem({ item }: { item: Faq }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="border-b border-gray-200 last:border-b-0">
      <h3>
        <button
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          className="flex w-full items-center justify-between gap-4 py-4 text-left text-sm font-semibold text-gray-900 hover:text-brand-orange transition-colors"
        >
          {item.q}
          <ChevronDown
            className={cn(
              'h-4 w-4 flex-shrink-0 text-gray-400 transition-transform',
              open && 'rotate-180'
            )}
            aria-hidden
          />
        </button>
      </h3>
      {open && <p className="pb-4 pr-8 text-sm leading-relaxed text-gray-600">{item.a}</p>}
    </div>
  )
}

export default function HelpPage() {
  // FAQPage structured data — habilita o rich result de perguntas no Google.
  const faqSchema = {
    '@context': 'https://schema.org',
    '@type': 'FAQPage',
    mainEntity: SECTIONS.flatMap((s) =>
      s.items.map((item) => ({
        '@type': 'Question',
        name: item.q,
        acceptedAnswer: { '@type': 'Answer', text: item.a },
      }))
    ),
  }

  return (
    <>
      <Seo
        title="Central de ajuda"
        description="Respostas sobre pedidos, pagamento, prazo de entrega, trocas, devoluções e o direito de arrependimento de 7 dias na UtiLar Ferragem."
        path="/ajuda"
        jsonLd={faqSchema}
      />

      <div className="container py-8">
        <Breadcrumb
          items={[{ label: 'Início', href: '/' }, { label: 'Central de ajuda' }]}
          className="mb-6"
        />

        <header className="mb-8 max-w-3xl">
          <h1 className="font-display font-black text-3xl text-gray-900">Central de ajuda</h1>
          <p className="mt-3 text-gray-600 leading-relaxed">
            As dúvidas que mais chegam ao nosso atendimento, respondidas direto. Não achou a sua?{' '}
            <Link to="/contato" className="text-brand-orange underline">
              Fale com a gente
            </Link>
            .
          </p>
        </header>

        <div className="max-w-3xl flex flex-col gap-6">
          {SECTIONS.map((section) => (
            <section key={section.title} className="rounded-xl border border-gray-200 bg-white px-5">
              <h2 className="font-display font-bold text-lg text-gray-900 pt-5 pb-1">
                {section.title}
              </h2>
              {section.items.map((item) => (
                <FaqItem key={item.q} item={item} />
              ))}
            </section>
          ))}
        </div>
      </div>
    </>
  )
}
