# Nota fiscal, SEFAZ e impostos — como o Utilar emite

> **Resumo de uma linha:** a **Appmax processa o dinheiro, não emite nota**. A
> emissão fiscal e a conversa com a SEFAZ ficam com um **emissor de NF-e como
> serviço** (Focus NF-e, PlugNotas, NFe.io, eNotas). São responsabilidades
> ortogonais: gateway ≠ fiscal. Hoje **não há nada disso no código** — este doc
> é o plano.

## 1. Quem emite a nota? (não é a Appmax)

A Appmax é **PSP / gateway de pagamento**: Pix, boleto, cartão, Payment Split,
antecipação. Ela move dinheiro. **Não emite documento fiscal** e não fala com a
SEFAZ pela loja.

Quem emite é o **contribuinte** — a Utilar, pelo CNPJ dela. É obrigação legal do
vendedor, indelegável ao meio de pagamento. O que existe hoje no payment-service
é o **livro contábil** (ledger em partidas dobradas): controle **interno**, não
tem valor fiscal e não substitui a nota.

## 2. São DUAS notas — uma por fluxo de venda

Os dois fluxos de venda do Utilar mapeiam exatamente nos dois modelos fiscais:

| Fluxo | Modelo | Quando | Característica |
|---|---|---|---|
| **Loja online** (`/checkout`) — mercadoria enviada | **NF-e (mod. 55)** | após pagamento, antes/junto do envio | acompanha a mercadoria (DANFE no pacote); pode ser **assíncrona** |
| **Balcão / PDV** (`/balcao`) — venda presencial | **NFC-e (mod. 65)** | no ato da venda | cliente esperando; imprime o **DANFE NFC-e com QR code** na hora — precisa ser **~síncrona** |

Essa diferença de tempo é o ponto arquitetural mais importante: **NF-e web pode
ser processada em background** (emitir quando o pedido vira `paid`, anexar ao
envio); **NFC-e de balcão trava o operador** até a SEFAZ autorizar, então precisa
de resposta rápida e de um **modo de contingência offline** (a SEFAZ cai, e o
vendedor não pode parar de vender).

## 3. Como funciona a integração com a SEFAZ

NF-e/NFC-e não é "gerar um PDF". É:

1. Montar um **XML** no layout oficial (4.00) com **CFOP, NCM, CST/CSOSN,
   origem, ICMS/PIS/COFINS, CEST** etc. calculados corretamente.
2. **Assinar** com **certificado digital e-CNPJ A1** (ICP-Brasil).
3. Enviar ao **webservice da SEFAZ do estado**, que valida e devolve um
   **protocolo de autorização de uso**.
4. Guardar o **XML autorizado** (obrigatório por 5 anos) e gerar o **DANFE**.
5. Tratar **eventos**: cancelamento (dentro do prazo legal), carta de correção,
   inutilização de numeração, e **contingência** (SVC-AN/SVC-RS quando a SEFAZ
   do estado está fora).

Fazer isso **direto** significa manter certificado, assinatura XML, os webservices
de **cada estado** (com suas manias), contingência, eventos e as mudanças de
layout — para sempre. Não compensa para uma loja.

**Decisão recomendada: usar um emissor como serviço (API REST).** Você faz um
`POST` com os dados da venda; o provedor monta+assina o XML, fala com a SEFAZ,
devolve o XML autorizado + DANFE (PDF/URL) e cuida de contingência/eventos/layout.

### Provedores (todos com sandbox e cobrem NF-e **e** NFC-e)

| Provedor | Observação |
|---|---|
| **Focus NF-e** | API simples, forte em NF-e/NFC-e, boa doc — recomendado p/ começar |
| **PlugNotas** | idem, bom suporte a NFC-e e MEI/Simples |
| **NFe.io** | boa DX, foco em quem constrói produto |
| **eNotas** | voltado a e-commerce/SaaS |

Custo típico: por nota (~R$ 0,10–0,50) ou planos de ~R$ 50–200/mês em baixo
volume. **Certificado A1 e-CNPJ**: ~R$ 150–250/ano (compra separada, obrigatória
em qualquer cenário).

## 4. O que falta no código do Utilar

### 4.1 Campos fiscais no produto (catalog-service) — **bloqueador de dados**
O produto **não tem** os campos fiscais. Precisa (migration no catalog):

- **NCM** (8 díg.) — classificação da mercadoria. Obrigatório.
- **CEST** — para itens em **ICMS-ST** (substituição tributária). **Muito
  material de construção está em ST** — é a parte mais traiçoeira.
- **origem** (0–8) — nacional/importado.
- **CSOSN** (Simples) ou **CST** (Lucro Presumido/Real).
- **unidade tributável** (UN, PC, KG, M, M², …) — casa com a venda fracionada.

⚠️ Definir NCM/CEST/CSOSN por produto é **decisão do contador**, não de
engenharia. É trabalho de curadoria, igual foi o de foto/preço. Mesma leva da
migration de **peso/dimensões** que o frete via transportadora vai precisar.

### 4.2 Integração fiscal (order-service)
O order-service já é dono do fulfillment e do balcão — conhece itens, preços,
cliente e frete. A emissão nasce ao lado dele (módulo `fiscal` no order, ou
serviço pequeno dedicado):

- **Web**: no `payment.confirmed` (pedido `paid`) → emite **NF-e** em background
  → guarda XML+DANFE → anexa ao envio.
- **Balcão**: no fechamento da venda → emite **NFC-e** de forma síncrona →
  imprime o DANFE com QR na hora → **contingência offline** se a SEFAZ cair.
- **Cancelamento**: dentro da janela legal, casado com estorno/devolução (CDC).

### 4.3 Invariante que se estende
"**O cliente nunca dita valor**" vira também "**o cliente nunca dita imposto**":
CFOP, base de cálculo e alíquota são resolvidos **no servidor** a partir da
classificação do produto e do destino — nunca vêm do corpo da requisição.

## 5. Impostos — o que a nota carrega (e o regime tributário)

- **Regime**: uma ferragem provavelmente começa no **Simples Nacional** — ICMS/
  PIS/COFINS recolhidos juntos via **DAS**, e a nota usa **CSOSN**. Em Lucro
  Presumido/Real usa **CST** e destaca os tributos. O contador define.
- **ICMS-ST**: em muitos itens de construção o ICMS já foi recolhido lá atrás
  (fabricante/distribuidor) — a loja **não destaca de novo**, mas referencia
  certo (ex.: CSOSN 500 no Simples). Errar ST é o modo de falha fiscal mais caro.
- **DIFAL** (venda interestadual a consumidor): vender para outro estado dispara
  o diferencial de alíquota do ICMS — o emissor calcula, mas exige cadastro/
  tratamento. Importa numa loja que envia pro Brasil todo.

Um bom emissor + NCM/CEST corretos resolvem a maior parte do cálculo; a
**classificação fiscal por produto** é o que precisa ser curado com o contador.

## 6. Passos práticos (em ordem)

1. **Contador define**: regime tributário + NCM/CEST/CSOSN/origem por produto.
   *(bloqueador do dono — decisão de negócio/contábil)*
2. Comprar **certificado A1 e-CNPJ**.
3. Escolher e integrar um **emissor** (recomendo Focus NF-e ou PlugNotas) — usar
   o **sandbox** primeiro.
4. Migration no catalog: **campos fiscais** no produto (junto de peso/dimensões).
5. Módulo `fiscal` no order-service: NF-e no `paid` (web) + NFC-e síncrona
   (balcão) + contingência + cancelamento.
6. **Testes**: contrato contra o sandbox do emissor; regressão do mapeamento
   pedido→NF-e; a invariante "cliente nunca dita imposto". Adicionar a camada
   `fiscal` na skill `test-utilar`.

## 7. Resposta direta às perguntas

- **Isso fica tudo com a Appmax?** Não. A Appmax é só pagamento.
- **Nós integramos com a SEFAZ ou com outros sistemas?** Com a SEFAZ **via um
  emissor de NF-e** (Focus/PlugNotas/NFe.io/eNotas). Integração direta com a
  SEFAZ é possível, mas não vale o custo de manutenção para uma loja.
- **Como a nota é emitida?** Loja online → **NF-e (55)** ao confirmar pagamento;
  balcão → **NFC-e (65)** no ato, com QR e contingência. Ambas assinadas com
  **certificado A1** e autorizadas pela SEFAZ do estado — trabalho que o emissor
  faz pela API.
