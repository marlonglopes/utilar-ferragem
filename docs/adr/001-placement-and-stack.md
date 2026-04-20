# ADR 001 — Posicionamento e stack

**Status**: Proposto. **Data**: 2026-04-20.

## Contexto

A Utilar Ferragem é um storefront vertical novo que precisa de um lar no repositório e de uma stack tecnológica. Três opções:

1. **Incorporar dentro do `gifthy-hub/`** como uma nova árvore de rotas (`/utilar/*`)
2. **Nova SPA irmã** em `utilar-ferragem/` com seu próprio build + deploy
3. **App standalone com backend próprio** em um repositório separado

## Decisão

**Opção 2 — Nova SPA irmã no mesmo monorepo.** A stack espelha o `gifthy-hub/`: Vite + React 18 + TS + Tailwind + Zustand + TanStack Query + i18next + Vitest.

## Consequências

### Positivas
- Deploy independente (`utilarferragem.com.br`) sem acoplamento ao ritmo de lançamento do console de vendedores
- Monorepo compartilhado significa padrões de CI compartilhados, configurações de ferramentas compartilhadas e portabilidade trivial de código entre os apps
- Paridade de stack reduz o custo de troca de contexto para qualquer engenheiro que transite entre os apps
- Não força uma divisão de repositório até que haja uma razão real (crescimento de equipe, cadência de lançamento diferente, modelo de autenticação diferente)

### Negativas
- Dois configs de Vite para manter (aceitável; são pequenos)
- Risco de divergência — se a Utilar começar a construir seu próprio design system, ou fazemos back-port para o gifthy-hub ou aceitamos evolução separada (decidir caso a caso)
- Duplicação de bundle se não extrairmos um pacote `@gifthy/ui` compartilhado mais tarde (aceitável por enquanto; extrair quando a dor superar ~3 componentes duplicados)

### Alternativas rejeitadas
- **Opção 1 (incorporar)**: o gifthy-hub é um console de vendedor/admin. Misturar um marketplace para consumidor nele confunde personas muito diferentes, requisitos de autenticação, expectativas de SEO e cadências de deploy. Não.
- **Opção 3 (repositório separado)**: prematuro. Um repositório separado faz sentido quando as equipes não conseguem se coordenar, não quando um único engenheiro está desenvolvendo os dois. Reavaliar se headcount ou pressão de releases exigir.

## Destino de deploy

**S3 + CloudFront** para o bundle estático da SPA.

Justificativa:
- Mais barato (hospedagem estática + CDN)
- CDN mais rápida no BR versus nginx atrás de ALB
- Zero acoplamento ao trabalho de ALB da Sprint 15 do projeto pai
- A API ainda flui pelo Go gateway existente em `api.utilarferragem.com.br` ou `api.gifthy.com.br/utilar` (subdomínio vs. prefixo de path — a definir)

Rejeitado: container nginx atrás de ALB (caminho da Sprint 14) — funciona, mas custa mais e não agrega nada para uma SPA estática.

## Plano de rollback

Se S3+CloudFront se mostrar problemático (invalidação de cache, complexidade de deploy), cair de volta para o caminho nginx-no-ALB da Sprint 15 do projeto pai. Nenhuma alteração de código na SPA é necessária — apenas o pipeline de deploy.
