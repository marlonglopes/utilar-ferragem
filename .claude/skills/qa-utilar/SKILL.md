---
name: qa-utilar
description: "QA PROFUNDA do Utilar Ferragem (entrada de pré-deploy). Roda a pirâmide inteira via test-utilar — backend Go -race, frontend, E2E, a11y, segurança/SAST/CVE/invariantes, pentest, quality, ingestão, Appmax, integrações. A test-utilar já isola os bancos do catalog E do order (efêmeros, dev intocado), então a QA é confiável e imune ao estado do banco. Esta skill só para os serviços Go rodando (higiene) e delega. Use quando o usuário pedir uma QA completa/profunda, 'testar tudo como um QA faria', ter certeza de que está tudo funcionando, ou validar antes de deploy/go-live."
---

# QA profunda do Utilar — `qa-utilar`

Entrada de **pré-deploy**: roda toda a pirâmide de testes num ambiente limpo e
reporta se está tudo verde. É a skill pra "tem CERTEZA que está tudo funcionando?".

## O que ela faz

1. **Para** os serviços Go nas portas 8090–8094 (mantém as DBs). Higiene — os
   testes já rodam isolados, mas evita ruído de porta/log. Lista o que parou.
2. **Delega** a pirâmide pra `.claude/skills/test-utilar/run-tests.sh`.
3. Explica como ler o resultado.

## Por que é confiável (a diferença que importa)

A `test-utilar` roda o **catalog** e o **order** contra bancos **EFÊMEROS**
(clone + normalização; o dev fica intocado):

- **order** — clona `order_service`, `TRUNCATE orders` (os testes criam os seus).
- **catalog** — clona `catalog_service` e normaliza a fixture no clone: arquiva
  os produtos reais (sem foto), publica os curados (`CUR-`/`UTL-`, com capa) e os
  põe no topo da lista. Assim busca/listagem/related/capa passam sempre, sem
  depender de a loja estar "suja" pra produção.

Resultado: **uma falha não é mais poluição de estado do banco — é regressão de
verdade.** (O antigo caveat do `TestList_DevolveCapaDoProduto` foi resolvido na
raiz por essa isolação.)

## Como usar

```bash
.claude/skills/qa-utilar/run-qa.sh            # tudo
.claude/skills/qa-utilar/run-qa.sh backend    # uma camada (repassada pra test-utilar)
```
Ou, comigo, é só mandar **`/qa-utilar`**.

## Como interpretar o resultado

- **Verde em tudo** → software são; pode seguir.
- **Qualquer falha** → investigue de verdade (build, rode o teste, olhe a
  lógica). Como os bancos são efêmeros, não dá mais pra atribuir a "estado do
  dev". Distinga regressão de flake conhecido (o runner já tolera 1: o async de
  upload do catalog sob -race).
- **Débito informado** (prettier/gofmt/CVE de stdlib) **não trava** — é
  informativo no resumo.

## Limites honestos

- **Não religa** os serviços — você religa (o script avisa quais parou).
- **Não roda DAST** (varredura ativa contra os serviços no ar) nem **Appmax
  live** (só com creds no ambiente).

## Ao concluir

Reporte o resumo por camada (✅/❌). Se houver falha, **verifique você mesmo** a
causa antes de chamar de regressão.
