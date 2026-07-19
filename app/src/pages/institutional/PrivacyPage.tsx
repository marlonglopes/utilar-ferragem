// ⚠️ ATENÇÃO — TEXTO PENDENTE DE REVISÃO JURÍDICA
//
// Esta Política de Privacidade é uma MINUTA redigida para atender à Lei
// 13.709/2018 (LGPD) e ao Decreto 7.962/2013. Ela NÃO substitui a análise de
// um advogado e NÃO deve ir ao ar sem:
//   1. revisão por profissional habilitado;
//   2. preenchimento de todos os campos [PREENCHER: ...] em src/lib/company.ts;
//   3. conferência de que as práticas descritas refletem o que o sistema faz
//      de fato (retenção, operadores, transferência internacional).
//
// Em especial, a lista de operadores/subcontratados (item 5) precisa bater com
// os fornecedores realmente contratados (PSP, hospedagem, e-mail, analytics).

import { Seo } from '@/components/seo/Seo'
import { LegalLayout } from '@/components/layout/LegalLayout'
import { COMPANY, formattedAddress } from '@/lib/company'

export default function PrivacyPage() {
  return (
    <>
      <Seo
        title="Política de Privacidade"
        description="Como a UtiLar Ferragem coleta, usa, compartilha e protege seus dados pessoais, conforme a Lei Geral de Proteção de Dados (LGPD)."
        path="/privacidade"
      />

      <LegalLayout
        title="Política de Privacidade"
        subtitle="Esta política explica quais dados pessoais coletamos, por que coletamos, com quem compartilhamos e como você pode exercer seus direitos, nos termos da Lei nº 13.709/2018 (LGPD)."
        updatedAt={COMPANY.legalDocsUpdatedAt}
        reviewNotice
      >
        <h2>1. Quem é o controlador dos seus dados</h2>
        <p>
          O controlador dos dados pessoais tratados neste site é{' '}
          <strong>{COMPANY.legalName}</strong>, inscrita no CNPJ sob o nº{' '}
          <strong>{COMPANY.cnpj}</strong>, com sede em {formattedAddress()}, que opera o
          marketplace <strong>{COMPANY.tradeName}</strong>.
        </p>
        <p>
          Nosso Encarregado pelo Tratamento de Dados Pessoais (DPO), previsto no art. 41 da LGPD,
          é <strong>{COMPANY.contact.dpoName}</strong>, que pode ser contatado pelo e-mail{' '}
          <strong>{COMPANY.contact.dpoEmail}</strong>.
        </p>

        <h2>2. Quais dados coletamos</h2>
        <h3>2.1. Dados que você nos fornece</h3>
        <ul>
          <li>
            <strong>Cadastro:</strong> nome completo, e-mail, CPF, telefone e senha (armazenada
            apenas em formato criptografado, nunca em texto legível).
          </li>
          <li>
            <strong>Entrega:</strong> CEP, logradouro, número, complemento, bairro, cidade e
            estado dos endereços que você cadastra.
          </li>
          <li>
            <strong>Pagamento:</strong> os dados do cartão são digitados diretamente no ambiente
            do nosso provedor de pagamento e <strong>não trafegam nem são armazenados</strong> nos
            nossos servidores. Guardamos apenas o meio de pagamento escolhido, os quatro últimos
            dígitos, a bandeira e o identificador da transação.
          </li>
          <li>
            <strong>Atendimento:</strong> o conteúdo das mensagens que você envia ao suporte ou ao
            nosso assistente virtual.
          </li>
        </ul>

        <h3>2.2. Dados coletados automaticamente</h3>
        <ul>
          <li>Endereço IP, data e hora de acesso e registros de navegação, cuja guarda por 6 meses é
            exigida pelo art. 15 do Marco Civil da Internet (Lei 12.965/2014).</li>
          <li>Tipo de dispositivo, navegador, sistema operacional e idioma.</li>
          <li>Páginas visitadas, produtos consultados e itens adicionados ao carrinho.</li>
          <li>Cookies e identificadores similares, conforme o item 7 desta política.</li>
        </ul>

        <h2>3. Para que usamos seus dados e com qual base legal</h2>
        <p>
          Todo tratamento que realizamos está apoiado em uma das hipóteses legais do art. 7º da
          LGPD:
        </p>
        <ul>
          <li>
            <strong>Processar seus pedidos, pagamentos e entregas</strong> — base legal: execução
            de contrato (art. 7º, V). Sem esses dados não conseguimos concluir a compra.
          </li>
          <li>
            <strong>Criar e manter sua conta e autenticar seu acesso</strong> — base legal:
            execução de contrato (art. 7º, V).
          </li>
          <li>
            <strong>Emitir nota fiscal e cumprir obrigações fiscais e contábeis</strong> — base
            legal: cumprimento de obrigação legal (art. 7º, II).
          </li>
          <li>
            <strong>Prevenir fraudes e garantir a segurança das transações</strong> — base legal:
            legítimo interesse (art. 7º, IX) e proteção ao crédito (art. 7º, X).
          </li>
          <li>
            <strong>Prestar atendimento e responder solicitações</strong> — base legal: execução de
            contrato e legítimo interesse (art. 7º, V e IX).
          </li>
          <li>
            <strong>Enviar comunicações de marketing e ofertas</strong> — base legal:
            consentimento (art. 7º, I). Você pode retirar esse consentimento a qualquer momento,
            sem prejuízo às compras já realizadas.
          </li>
          <li>
            <strong>Melhorar o site, medir audiência e recomendar produtos</strong> — base legal:
            legítimo interesse (art. 7º, IX), sempre com dados agregados quando possível.
          </li>
        </ul>

        <h2>4. Dados de crianças e adolescentes</h2>
        <p>
          O site não se destina a menores de 18 anos e não coletamos intencionalmente dados de
          crianças e adolescentes. Caso identifiquemos um cadastro nessa situação, a conta será
          encerrada e os dados eliminados, salvo obrigação legal de guarda.
        </p>

        <h2>5. Com quem compartilhamos seus dados</h2>
        <p>
          Nós <strong>não vendemos</strong> seus dados pessoais. O compartilhamento acontece apenas
          na medida necessária para operar o serviço, com:
        </p>
        <ul>
          <li>
            <strong>Vendedores parceiros do marketplace:</strong> recebem seu nome, endereço de
            entrega e os itens comprados, exclusivamente para separar e enviar o pedido.
          </li>
          <li>
            <strong>Provedores de pagamento:</strong> processam a cobrança em Pix, boleto ou
            cartão e realizam análise antifraude.
          </li>
          <li>
            <strong>Transportadoras e Correios:</strong> recebem os dados de destinatário e
            endereço para a entrega.
          </li>
          <li>
            <strong>Prestadores de tecnologia:</strong> hospedagem em nuvem, envio de e-mails
            transacionais, monitoramento de erros e análise de audiência, sempre vinculados por
            contrato a tratar os dados apenas conforme nossas instruções.
          </li>
          <li>
            <strong>Autoridades públicas:</strong> quando houver ordem judicial, requisição legal
            ou para defesa de nossos direitos em processo.
          </li>
        </ul>
        <p>
          Parte desses prestadores pode processar dados fora do Brasil. Nesses casos, exigimos
          garantias de proteção compatíveis com a LGPD, nos termos dos arts. 33 a 36.
        </p>

        <h2>6. Por quanto tempo guardamos seus dados</h2>
        <ul>
          <li>
            <strong>Dados de cadastro:</strong> enquanto sua conta estiver ativa e por até 5 anos
            após o encerramento, prazo prescricional do art. 27 do Código de Defesa do Consumidor.
          </li>
          <li>
            <strong>Dados de pedidos e notas fiscais:</strong> 5 anos, para cumprir a legislação
            fiscal e permitir a defesa em eventual reclamação.
          </li>
          <li>
            <strong>Registros de acesso (logs):</strong> 6 meses, conforme o Marco Civil da
            Internet.
          </li>
          <li>
            <strong>Dados tratados com base em consentimento:</strong> até que você revogue o
            consentimento.
          </li>
        </ul>
        <p>
          Encerrados esses prazos, os dados são eliminados ou anonimizados de forma irreversível.
        </p>

        <h2>7. Cookies</h2>
        <p>Utilizamos três categorias de cookies:</p>
        <ul>
          <li>
            <strong>Necessários:</strong> mantêm sua sessão autenticada, o conteúdo do carrinho e
            o idioma escolhido. Sem eles o site não funciona, por isso não dependem de
            consentimento.
          </li>
          <li>
            <strong>De desempenho:</strong> medem como o site é usado, para corrigir erros e
            melhorar a navegação.
          </li>
          <li>
            <strong>De publicidade:</strong> permitem exibir ofertas mais relevantes. Dependem do
            seu consentimento.
          </li>
        </ul>
        <p>
          Você pode bloquear ou apagar cookies nas configurações do seu navegador, ciente de que
          isso pode limitar funcionalidades como manter-se conectado.
        </p>

        <h2>8. Seus direitos como titular</h2>
        <p>
          O art. 18 da LGPD garante a você, a qualquer momento e gratuitamente, o direito de:
        </p>
        <ol>
          <li>confirmar se tratamos dados seus e acessar esses dados;</li>
          <li>corrigir dados incompletos, inexatos ou desatualizados;</li>
          <li>
            solicitar a anonimização, o bloqueio ou a eliminação de dados desnecessários ou
            tratados em desconformidade com a lei;
          </li>
          <li>solicitar a portabilidade dos dados a outro fornecedor;</li>
          <li>
            solicitar a eliminação dos dados tratados com base em consentimento, ressalvadas as
            hipóteses de guarda obrigatória previstas no item 6;
          </li>
          <li>ser informado sobre com quem compartilhamos seus dados;</li>
          <li>
            ser informado sobre a possibilidade de não fornecer consentimento e sobre as
            consequências da recusa;
          </li>
          <li>revogar o consentimento a qualquer momento;</li>
          <li>opor-se a tratamento feito com base em legítimo interesse.</li>
        </ol>
        <p>
          Para exercer qualquer um desses direitos, escreva para{' '}
          <strong>{COMPANY.contact.dpoEmail}</strong>. Responderemos em até 15 dias. Podemos pedir
          informações adicionais para confirmar sua identidade antes de atender ao pedido — é uma
          medida de segurança para impedir que terceiros acessem seus dados.
        </p>
        <p>
          Se você entender que sua solicitação não foi atendida adequadamente, pode apresentar
          reclamação à Autoridade Nacional de Proteção de Dados (ANPD).
        </p>

        <h2>9. Como protegemos seus dados</h2>
        <p>
          Adotamos medidas técnicas e administrativas para proteger seus dados, entre elas: tráfego
          criptografado por HTTPS/TLS, senhas armazenadas com algoritmo de hash, controle de acesso
          restrito por função, registro de auditoria das operações sensíveis e segregação dos dados
          de pagamento, que ficam sob responsabilidade de provedor certificado PCI-DSS.
        </p>
        <p>
          Nenhum sistema é totalmente imune. Caso ocorra um incidente de segurança com risco
          relevante aos seus direitos, comunicaremos você e a ANPD nos prazos do art. 48 da LGPD.
        </p>

        <h2>10. Alterações desta política</h2>
        <p>
          Podemos atualizar esta política para refletir mudanças legais ou no serviço. A versão
          vigente é sempre a publicada nesta página, com a data de atualização no topo. Se a
          mudança for significativa, avisaremos por e-mail ou por aviso em destaque no site antes
          de ela passar a valer.
        </p>

        <h2>11. Fale com a gente</h2>
        <p>
          Dúvidas sobre esta política ou sobre o tratamento dos seus dados podem ser enviadas para{' '}
          <strong>{COMPANY.contact.dpoEmail}</strong> ou para o endereço {formattedAddress()}.
        </p>
      </LegalLayout>
    </>
  )
}
