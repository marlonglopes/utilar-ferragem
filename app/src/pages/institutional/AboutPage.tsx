import { Link } from 'react-router-dom'
import { Seo } from '@/components/seo/Seo'
import { LegalLayout } from '@/components/layout/LegalLayout'
import { COMPANY } from '@/lib/company'

const NUMBERS = [
  { value: '8', label: 'categorias de produto' },
  { value: '65+', label: 'vendedores parceiros' },
  { value: '3', label: 'formas de pagamento' },
]

export default function AboutPage() {
  return (
    <>
      <Seo
        title="Sobre nós"
        description="Conheça a UtiLar Ferragem: um marketplace que conecta você a ferragens e distribuidores de materiais de construção com preço justo, nota fiscal e entrega rastreada."
        path="/sobre"
      />

      <LegalLayout
        title="Sobre a UtiLar Ferragem"
        subtitle="Somos um marketplace de ferragens, ferramentas e materiais de construção. Reunimos o sortimento de dezenas de distribuidores em um só lugar, com pagamento simples e entrega rastreada."
      >
        <h2>Por que existimos</h2>
        <p>
          Quem toca uma obra conhece a rotina: ligar para três ferragens para descobrir quem tem o
          material, anotar preço no papel, ir até a loja e descobrir que acabou o estoque. Do outro
          lado do balcão, ferragens de bairro com bom preço e atendimento de verdade perdem venda
          por não ter vitrine online.
        </p>
        <p>
          A UtiLar existe para resolver os dois lados. Para quem compra, um catálogo único com
          estoque e preço reais, pagamento em Pix, boleto ou cartão e acompanhamento do pedido do
          pagamento à entrega. Para quem vende, uma vitrine digital sem a complexidade de montar
          e operar uma loja própria.
        </p>

        <h2>Como trabalhamos</h2>
        <ul>
          <li>
            <strong>Vendedor verificado.</strong> Todo parceiro passa por checagem de CNPJ e situação
            cadastral antes de publicar. Toda venda sai com nota fiscal.
          </li>
          <li>
            <strong>Estoque honesto.</strong> A disponibilidade exibida vem do vendedor. Quando um
            item acaba, ele sai da vitrine — preferimos mostrar menos a prometer o que não temos.
          </li>
          <li>
            <strong>Preço sem pegadinha.</strong> O valor da tela é o valor que você paga. Frete e
            prazo aparecem antes de confirmar o pedido, nunca depois.
          </li>
          <li>
            <strong>Atendimento humano.</strong> Suporte em português, por gente que entende a
            diferença entre um parafuso autobrocante e um chipboard.
          </li>
        </ul>

        <h2>A plataforma hoje</h2>
        <div className="not-prose my-6 grid grid-cols-1 gap-4 sm:grid-cols-3">
          {NUMBERS.map(({ value, label }) => (
            <div key={label} className="rounded-xl border border-gray-200 bg-white p-5">
              <p className="font-display text-3xl font-black text-brand-orange">{value}</p>
              <p className="mt-1 text-sm text-gray-600">{label}</p>
            </div>
          ))}
        </div>
        <p>
          Estamos em fase de expansão do catálogo e da malha de entrega. Se o seu material ainda não
          está aqui, escreva para nós — a lista de pedidos dos clientes é o que orienta quais
          categorias abrimos primeiro.
        </p>

        <h2>Quem somos formalmente</h2>
        <p>
          A UtiLar Ferragem é operada por <strong>{COMPANY.legalName}</strong>, CNPJ{' '}
          <strong>{COMPANY.cnpj}</strong>. Os dados cadastrais completos estão nos{' '}
          <Link to="/termos">Termos de Uso</Link>.
        </p>

        <h2>Fale com a gente</h2>
        <p>
          Dúvidas, sugestões ou uma reclamação que precisa chegar a quem decide? Use a página de{' '}
          <Link to="/contato">contato</Link>. Se você é vendedor e quer anunciar,{' '}
          <Link to="/vender">cadastre sua ferragem</Link>.
        </p>
      </LegalLayout>
    </>
  )
}
