# Nota fiscal da Utilar — o que precisa ser decidido e contratado

**Para:** Thomazinho (dono da Utilar)
**De:** time de tecnologia
**Assunto:** o que a loja precisa para emitir nota fiscal — na venda pela internet
e na venda de balcão — e as decisões que dependem de você.

---

## Em uma frase

A **Appmax cuida só do pagamento** (Pix, boleto, cartão). Ela **não emite nota
fiscal**. Quem emite a nota é a Utilar, pelo CNPJ da loja. Para isso a gente
**não fala direto com a SEFAZ** — a gente contrata uma **empresa que emite a nota
por API** (tipo Focus NF-e), e o nosso sistema manda os dados da venda pra ela.

O sistema já está preparado para receber isso. Faltam **três decisões que são
suas** e uma parte de desenvolvimento que a gente toca.

---

## Como vai funcionar (em linguagem simples)

São **dois tipos de nota**, um para cada tipo de venda da Utilar:

| Venda | Nota | Quando sai |
|---|---|---|
| **Pela internet** (site, entrega) | **NF-e** | assim que o pagamento é confirmado; o documento (DANFE) vai junto com a mercadoria no pacote |
| **No balcão** (tablet, retirada na hora) | **NFC-e** (o "cupom fiscal" de hoje) | na hora da venda, com QR code, impresso ali mesmo |

No balcão tem um detalhe importante: se a SEFAZ ou o emissor ficarem fora do ar,
a venda **não pode parar**. O sistema emite em "contingência" e transmite a nota
depois — o cliente vai embora com o cupom do mesmo jeito.

---

## As 3 decisões que dependem de você

### 1. Falar com o contador sobre o regime e a classificação dos produtos
Antes de emitir qualquer nota, o contador precisa definir:
- O **regime tributário** da loja (provavelmente **Simples Nacional**).
- A **classificação fiscal de cada produto** — uns códigos chamados **NCM** e,
  para muitos itens de construção, o **CEST/substituição tributária**.

> **Por que isso importa:** material de construção é cheio de item com
> "substituição tributária" (o imposto já foi pago lá atrás pelo fabricante).
> Classificar errado gera nota errada e problema com o fisco. **É serviço do
> contador**, não da tecnologia. A gente só precisa que ele entregue essa lista.

### 2. Contratar a empresa que emite a nota (o "emissor")
É a empresa que conversa com a SEFAZ por nós. Sugestões (todas fazem NF-e e
NFC-e, têm ambiente de teste):

- **Focus NF-e** — recomendada para começar (simples, boa documentação)
- **PlugNotas** — boa para NFC-e/Simples
- **NFe.io** / **eNotas** — alternativas

**Custo típico:** por nota (centavos) ou plano de **~R$ 50 a R$ 200/mês** em
volume baixo.

### 3. Comprar o certificado digital da empresa (e-CNPJ A1)
É a "assinatura digital" do CNPJ da Utilar, exigida para emitir nota. Custa
**~R$ 150 a R$ 250 por ano**. O certificado fica guardado na conta do emissor —
a gente não precisa mexer nele no dia a dia.

---

## Custo estimado do fiscal (à parte da Appmax e da infra)

| Item | Valor aproximado |
|---|---|
| Emissor de nota (API) | R$ 50–200/mês |
| Certificado A1 e-CNPJ | R$ 150–250/ano |
| Honorário do contador (classificação) | conforme seu contador |

> Isso é **separado** do custo da Appmax (que é só a taxa do pagamento) e do
> custo de servidor/AWS.

---

## O que já está pronto e o que falta

**Já pronto:** o sistema tem os dois fluxos de venda (internet e balcão)
funcionando, com o controle financeiro interno certinho. Ele já foi desenhado
para "pendurar" a emissão da nota nos pontos certos.

**Falta (parte da tecnologia, a gente toca):**
- ligar o sistema no emissor escolhido;
- guardar os códigos fiscais em cada produto (depois que o contador entregar);
- emitir a NF-e na venda web e a NFC-e no balcão, com testes cobrindo tudo.

A gente **consegue adiantar quase toda a programação** usando o ambiente de teste
do emissor, **antes mesmo** de fechar o contrato — mas para emitir nota **de
verdade** (valendo), as 3 decisões acima precisam estar resolvidas.

---

## O que a gente precisa de você, em ordem

1. **Confirmar o contador** e pedir a ele: regime tributário + a lista de
   **NCM/CEST** dos produtos.
2. **Escolher o emissor** (a gente recomenda Focus NF-e) e autorizar a
   contratação.
3. **Comprar o certificado A1 e-CNPJ** (o contador ou a gente ajuda no processo).

Com esses três, a Utilar passa a emitir nota fiscal automática em toda venda —
site e balcão — sem trabalho manual.

---

*Detalhe técnico (para o time de desenvolvimento, não precisa ler): o plano de
implementação está em `docs/fiscal-implementacao.md` e a análise em
`docs/fiscal-nota-e-integracao.md`.*
