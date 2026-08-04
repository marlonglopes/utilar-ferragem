---
name: qa-utilar
description: "QA PROFUNDA e confiável do Utilar Ferragem — roda a pirâmide inteira (test-utilar: backend Go -race, frontend, E2E, a11y, segurança/SAST/CVE/invariantes, pentest, quality, ingestão, Appmax, integrações) mas dentro de um ambiente PREPARADO, imune ao estado do banco de dev. Antes de rodar: para os serviços Go que atrapalham os testes de concorrência e PUBLICA a fixture do catalog (que a limpeza pra produção deixa arquivada), restaurando tudo no fim. Use quando o usuário pedir uma QA completa/profunda, 'testar tudo como um QA faria', ter certeza de que está tudo funcionando, validar antes de deploy/go-live, ou quando a test-utilar der falha no catalog por causa de estado do banco. É a versão robusta da test-utilar."
---

# QA profunda do Utilar — `qa-utilar`

Roda **toda** a pirâmide de testes (a mesma da `test-utilar`), mas dentro de um
ambiente preparado pra o veredito **não mentir por causa do estado do banco de
dev**. É a skill pra "tem CERTEZA que está tudo funcionando?".

## Por que ela existe (o que a `test-utilar` sozinha não resolve)

A `test-utilar` roda a pirâmide e isola o banco do **order** (efêmero). Mas dois
fatores do ambiente ainda fazem o **catalog** falhar sem ser bug:

1. **Serviços Go rodando** — o sweeper de reservas do catalog (a cada 60s) e o
   pool de conexões brigam com os testes de integração/concorrência.
2. **Fixture do catalog arquivada** — os testes de busca/listagem/capa dependem
   dos ~400 produtos curados (`CUR-`/`UTL-`) **publicados**. Quando a loja é
   limpa pra produção (arquivar mocks, publicar os reais), eles ficam
   `archived` e a busca não acha nada → falhas que **não são regressão**.

A `qa-utilar` conserta os dois antes de rodar e **restaura tudo no fim**.

## Como usar

```bash
# tudo (recomendado)
.claude/skills/qa-utilar/run-qa.sh

# uma camada só (repassada pra test-utilar)
.claude/skills/qa-utilar/run-qa.sh backend
.claude/skills/qa-utilar/run-qa.sh frontend
.claude/skills/qa-utilar/run-qa.sh security
```

O script:
1. **Para** os serviços Go nas portas 8090–8094 (mantém as DBs). Lista o que
   parou — religue com `make dev-full` depois.
2. **Publica** a fixture do catalog (`CUR-`/`UTL-` que estavam `archived`),
   guardando os ids pra **re-arquivar no fim** (trap EXIT, mesmo se der erro).
3. **Delega** a pirâmide pra `.claude/skills/test-utilar/run-tests.sh`.
4. **Restaura** a fixture e explica como ler o resultado.

## Como interpretar o resultado

- **Verde em tudo** → software são; pode seguir.
- **Única falha = `TestList_DevolveCapaDoProduto`** ("0 de N produtos com capa"):
  é o **caveat conhecido**, não bug. Esse teste verifica que a *página 1* da
  lista admin tem capa; com o catálogo real (milhares de produtos ainda **sem
  foto**) dominando a página 1, ele falha mesmo com a fixture publicada. É
  acoplamento do teste à composição do catálogo.
- **Qualquer outra falha do catalog** (com a fixture publicada) → investigar de
  verdade: é regressão.

## O que ela NÃO faz (limites honestos)

- **Não sobe** os serviços de volta — você religa (o script avisa quais parou).
- **Não cria banco efêmero pro catalog** (só publica a fixture in-place com
  restore). O fix de raiz do caveat do `TestList` — um banco só-fixture pro
  catalog, como o order já tem, OU tornar o teste fixture-scoped — fica como
  melhoria futura. Enquanto isso, "verde exceto TestList" = software são.
- **Não roda DAST** (varredura ativa contra os serviços no ar) nem **Appmax
  live** (só com creds no ambiente) — mesmos limites da `test-utilar`.

## Ao concluir

Reporte o resumo por camada (✅/❌). Se houver falha, **verifique você mesmo** a
causa (build, rode o teste, olhe a lógica) antes de chamar de regressão —
vários bugs sérios já saíram de confiar em relatório sem conferir. Distinga
sempre **regressão de código** de **poluição de estado do banco**.
