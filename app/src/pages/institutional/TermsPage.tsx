// ⚠️ ATENÇÃO — TEXTO PENDENTE DE REVISÃO JURÍDICA
//
// Estes Termos de Uso são uma MINUTA redigida para atender ao Código de Defesa
// do Consumidor (Lei 8.078/1990), ao Decreto 7.962/2013 (contratação
// eletrônica) e ao Código Civil. NÃO devem ir ao ar sem:
//   1. revisão por profissional habilitado;
//   2. preenchimento dos campos [PREENCHER: ...] em src/lib/company.ts;
//   3. conferência de que prazos de entrega, política de trocas e formas de
//      pagamento descritos aqui correspondem à operação real.
//
// Pontos que exigem decisão do negócio antes da aprovação:
//   - quem arca com o frete da devolução por arrependimento (item 8);
//   - prazo de reembolso praticado pelo PSP;
//   - se a Utilar atua como marketplace (intermediadora) ou vendedora direta —
//     o texto abaixo assume MARKETPLACE, o que muda a responsabilidade civil.

import { Seo } from '@/components/seo/Seo'
import { LegalLayout } from '@/components/layout/LegalLayout'
import { COMPANY, formattedAddress } from '@/lib/company'

export default function TermsPage() {
  return (
    <>
      <Seo
        title="Termos de Uso"
        description="Condições gerais de uso e de compra no marketplace UtiLar Ferragem: pedidos, pagamento, entrega, direito de arrependimento, trocas e devoluções."
        path="/termos"
      />

      <LegalLayout
        title="Termos de Uso e Condições de Compra"
        subtitle="Estas condições regem o uso do site e as compras realizadas no marketplace UtiLar Ferragem. Ao criar uma conta ou finalizar um pedido, você declara que leu e concorda com elas."
        updatedAt={COMPANY.legalDocsUpdatedAt}
        reviewNotice
      >
        <h2>1. Identificação do fornecedor</h2>
        <p>
          O site é operado por <strong>{COMPANY.legalName}</strong>, CNPJ{' '}
          <strong>{COMPANY.cnpj}</strong>, inscrição estadual{' '}
          <strong>{COMPANY.stateRegistration}</strong>, com sede em {formattedAddress()}.
        </p>
        <p>
          Canais oficiais de atendimento: <strong>{COMPANY.contact.email}</strong> e{' '}
          <strong>{COMPANY.contact.phone}</strong>, no horário de {COMPANY.contact.hours}.
        </p>

        <h2>2. O que é a UtiLar Ferragem</h2>
        <p>
          A UtiLar Ferragem é um <strong>marketplace</strong>: uma plataforma que aproxima você de
          vendedores parceiros de ferragens, ferramentas e materiais de construção. Em cada produto
          indicamos quem é o vendedor responsável pela venda e pela entrega.
        </p>
        <p>
          A UtiLar responde pela operação da plataforma, pelo processamento do pagamento e pelo
          atendimento ao cliente. O vendedor parceiro responde pela procedência, qualidade,
          embalagem, emissão da nota fiscal e envio do produto. Nos termos do Código de Defesa do
          Consumidor, atuamos solidariamente com o vendedor na solução de problemas de consumo
          decorrentes de compras feitas pela plataforma.
        </p>

        <h2>3. Cadastro e conta</h2>
        <ul>
          <li>Para comprar é necessário criar uma conta com dados verdadeiros, completos e atuais.</li>
          <li>É preciso ser maior de 18 anos e civilmente capaz.</li>
          <li>
            A senha é pessoal e intransferível. Você é responsável pelas operações realizadas com
            suas credenciais; avise-nos imediatamente se suspeitar de uso indevido.
          </li>
          <li>
            Podemos suspender ou encerrar contas que apresentem dados falsos, indícios de fraude ou
            violação destes Termos, sempre com comunicação ao titular.
          </li>
        </ul>

        <h2>4. Produtos, descrições e disponibilidade</h2>
        <p>
          Fazemos o possível para que fotos, descrições, especificações técnicas e medidas estejam
          corretas. Imagens são ilustrativas e podem apresentar variação de cor conforme a tela do
          seu dispositivo; itens de decoração ou acessórios que apareçam nas fotos não acompanham o
          produto, salvo indicação expressa.
        </p>
        <p>
          As ofertas valem enquanto houver estoque. Identificado erro evidente de cadastro — como
          preço manifestamente incompatível com o de mercado — poderemos cancelar o pedido antes da
          confirmação, comunicando você imediatamente e devolvendo integralmente qualquer valor já
          pago.
        </p>

        <h2>5. Preços e pagamento</h2>
        <ul>
          <li>
            Os preços são em reais (R$) e incluem os tributos aplicáveis. O frete é calculado e
            exibido separadamente antes da conclusão do pedido.
          </li>
          <li>
            Antes de confirmar a compra você visualiza o resumo com produtos, quantidades, frete,
            prazo e valor total, conforme o art. 4º do Decreto 7.962/2013.
          </li>
          <li>
            Aceitamos <strong>Pix</strong>, <strong>boleto bancário</strong> e{' '}
            <strong>cartão de crédito</strong>, com as condições de parcelamento exibidas no
            checkout.
          </li>
          <li>
            O pedido só é encaminhado para separação após a confirmação do pagamento pela
            instituição financeira. Boletos não pagos até o vencimento levam ao cancelamento
            automático.
          </li>
          <li>
            Preços e condições anunciados podem mudar a qualquer tempo, mas nunca alteram pedidos
            já confirmados.
          </li>
        </ul>

        <h2>6. Entrega</h2>
        <p>
          O prazo de entrega é informado no checkout e começa a contar da confirmação do pagamento,
          não da data do pedido. Ele soma o tempo de separação pelo vendedor e o tempo de transporte
          da transportadora escolhida.
        </p>
        <ul>
          <li>
            A entrega ocorre no endereço informado por você. Endereço incorreto ou incompleto pode
            gerar devolução da mercadoria e cobrança de novo frete.
          </li>
          <li>
            É necessário haver alguém no local para receber e conferir o produto. Confira a
            embalagem no ato: havendo avaria visível, recuse o recebimento e registre a ocorrência
            no comprovante de entrega.
          </li>
          <li>
            Atrasos causados por caso fortuito ou força maior — greves, bloqueios de via, eventos
            climáticos extremos — serão comunicados, com nova previsão.
          </li>
        </ul>

        <h2>7. Cancelamento antes do envio</h2>
        <p>
          Enquanto o pedido não tiver sido despachado, você pode cancelá-lo pela sua conta ou pelo
          atendimento, sem qualquer custo. O estorno segue os prazos do item 9.
        </p>

        <h2>8. Direito de arrependimento — 7 dias</h2>
        <p>
          Por se tratar de compra fora do estabelecimento comercial, o{' '}
          <strong>art. 49 do Código de Defesa do Consumidor</strong> garante a você o direito de
          desistir da compra em até <strong>7 (sete) dias corridos</strong> contados do{' '}
          <strong>recebimento do produto</strong>, sem precisar justificar o motivo e sem qualquer
          penalidade.
        </p>
        <h3>Como exercer</h3>
        <ol>
          <li>
            Comunique a desistência dentro do prazo pelo e-mail{' '}
            <strong>{COMPANY.contact.email}</strong> ou pela área &ldquo;Meus pedidos&rdquo;,
            informando o número do pedido.
          </li>
          <li>
            Devolva o produto com todos os acessórios, manuais e a embalagem original, sem sinais de
            uso além do necessário para simples avaliação do item.
          </li>
          <li>
            Enviaremos as instruções e o código de postagem. <strong>Os custos da devolução por
            arrependimento correm por nossa conta</strong>, conforme entendimento consolidado sobre
            o art. 49, parágrafo único, do CDC.
          </li>
        </ol>
        <p>
          Recebido e conferido o produto, devolvemos <strong>integralmente</strong> tudo o que foi
          pago, inclusive o frete de envio, nos prazos do item 9.
        </p>

        <h2>9. Trocas, devoluções e reembolso</h2>
        <h3>9.1. Produto com defeito</h3>
        <p>
          O art. 26 do CDC assegura prazo de reclamação de <strong>30 dias</strong> para produtos
          não duráveis e <strong>90 dias</strong> para produtos duráveis, contados do recebimento.
          Constatado vício, o fornecedor tem até 30 dias para saná-lo; não sanado, você pode
          escolher entre a substituição do produto, a devolução do valor pago corrigido ou o abatimento
          proporcional do preço.
        </p>
        <h3>9.2. Produto errado ou avariado no transporte</h3>
        <p>
          Recebeu item diferente do pedido ou danificado? Comunique em até 7 dias corridos, de
          preferência com fotos. A coleta e o reenvio são por nossa conta.
        </p>
        <h3>9.3. Itens que não aceitam devolução por arrependimento</h3>
        <ul>
          <li>produtos cortados, misturados ou fabricados sob medida a seu pedido;</li>
          <li>
            sacarias de cimento, argamassa e afins já abertas, por impossibilidade de recomposição;
          </li>
          <li>itens de uso pessoal cuja devolução seja vedada por razão de higiene ou segurança.</li>
        </ul>
        <p>Essas exceções não afetam a garantia legal por defeito descrita no item 9.1.</p>
        <h3>9.4. Prazos de reembolso</h3>
        <ul>
          <li>
            <strong>Pix e boleto:</strong> depósito na conta bancária de sua titularidade em até 10
            dias úteis após a conferência da devolução.
          </li>
          <li>
            <strong>Cartão de crédito:</strong> solicitamos o estorno à operadora em até 10 dias
            úteis; o crédito aparece na fatura seguinte ou na subsequente, conforme a data de
            fechamento definida pelo emissor — prazo que foge ao nosso controle.
          </li>
        </ul>

        <h2>10. Garantia</h2>
        <p>
          Além da garantia legal do CDC, alguns produtos contam com garantia adicional oferecida
          pelo fabricante, cujo prazo e cobertura constam na embalagem ou no manual. Guarde a nota
          fiscal: ela é indispensável para o acionamento de qualquer garantia.
        </p>

        <h2>11. Uso da plataforma</h2>
        <p>Ao usar o site, você se compromete a não:</p>
        <ul>
          <li>praticar atos que comprometam a segurança, a disponibilidade ou a integridade do serviço;</li>
          <li>usar robôs, scrapers ou automações para coletar dados ou fazer pedidos em massa;</li>
          <li>publicar conteúdo ilícito, ofensivo ou que viole direitos de terceiros;</li>
          <li>tentar acessar contas, dados ou áreas restritas sem autorização.</li>
        </ul>
        <p>
          Marcas, logotipos, textos, imagens e o código do site são protegidos por direitos de
          propriedade intelectual e não podem ser reproduzidos sem autorização prévia por escrito.
        </p>

        <h2>12. Proteção de dados</h2>
        <p>
          O tratamento dos seus dados pessoais é descrito na nossa{' '}
          <a href="/privacidade">Política de Privacidade</a>, que integra estes Termos.
        </p>

        <h2>13. Alterações destes Termos</h2>
        <p>
          Podemos alterar estes Termos a qualquer momento; a versão vigente é sempre a publicada
          nesta página. Alterações não retroagem para atingir pedidos já confirmados, que continuam
          regidos pelas condições em vigor na data da compra.
        </p>

        <h2>14. Atendimento e solução de conflitos</h2>
        <p>
          Procure primeiro o nosso atendimento em <strong>{COMPANY.contact.email}</strong> — a
          maioria das questões se resolve por lá. Você também pode registrar reclamação na
          plataforma consumidor.gov.br ou nos órgãos de defesa do consumidor (Procon).
        </p>

        <h2>15. Lei aplicável e foro</h2>
        <p>
          Estes Termos são regidos pelas leis brasileiras. Fica eleito o foro da comarca de{' '}
          <strong>{COMPANY.jurisdiction}</strong> para dirimir controvérsias, ressalvado o direito
          do consumidor de ajuizar a ação no foro de seu domicílio, nos termos do art. 101, I, do
          Código de Defesa do Consumidor.
        </p>
      </LegalLayout>
    </>
  )
}
