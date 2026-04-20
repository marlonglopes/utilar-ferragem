# ADR 003 — Marca e abordagem de UI compartilhada

**Status**: Proposto. **Data**: 2026-04-20.

## Contexto

A Utilar Ferragem lança com sua própria marca (paleta de cores, tipografia, voz) mas quer compartilhar o máximo de código possível com o gifthy-hub. Como evitar tanto (a) deriva visual entre os dois apps quanto (b) abstrair demais em um pacote de design system antes que haja demanda real?

## Decisão

### Tematização
- Tailwind 3 + **variáveis CSS** para todos os tokens de tema (cores, famílias de fonte, radii).
- Cada app tem seu próprio bloco `:root { --color-primary: ... }`.
- O código dos componentes referencia `var(--color-primary)` através de um pequeno conjunto de utilitários Tailwind (`bg-primary`, `text-primary`, etc.) que mapeiam para as variáveis CSS.
- Trocar temas = trocar o bloco de variáveis CSS. Nenhuma mudança nos componentes é necessária.

### Componentes compartilhados
- **Nenhum pacote compartilhado nas Phases 1–3.** Em vez disso, copiar primitivos de `gifthy-hub/src/components/ui/` para `utilar-ferragem/src/components/ui/` durante a Sprint 02.
- **Extrair para um pacote compartilhado somente quando** tivermos ≥ 3 primitivos que divergem e ≥ 2 apps que consumiriam a versão compartilhada.
- Quando extrair: criar `packages/ui/` na raiz do monorepo, publicar como `@gifthy/ui` (workspace interno), migrar os dois apps.

### Voz & texto
- Cada app tem seus próprios arquivos JSON de i18n. Nenhuma biblioteca de copy compartilhada.
- Diretrizes de voz documentadas por app (ver o [doc de branding](../02-branding.md) da Utilar e o tom implícito do gifthy-hub).

### Logo & ativos de marca
- A Utilar Ferragem lança com uma marca tipográfica de placeholder até que o usuário forneça os ativos finais (logo SVG, wordmark, ícone, favicon).
- Source-of-truth dos ativos: `utilar-ferragem/public/brand/`.

## Consequências

### Positivas
- Zero investimento antecipado em design system; aprendemos o que é realmente compartilhado antes de nos comprometer com abstrações
- Tematização é genuinamente barata (um bloco de variáveis CSS por app)
- Cada app pode iterar sua UI sem coordenação entre equipes

### Negativas
- Alguma duplicação no início — aceita; é mais barata do que a abstração errada
- Potencial deriva — mitigar com uma revisão trimestral de "sincronizar primitivos"
- O passo de "copiar do gifthy-hub" na Sprint 02 é manual — documentar claramente nas tarefas daquela sprint

### Alternativas rejeitadas
- **Extrair pacote compartilhado no dia 1**: viola a regra de "três linhas similares antes de uma abstração"; ainda não sabemos o que é realmente compartilhado
- **Styled-components / Emotion**: adiciona custo de runtime e uma nova dependência; Tailwind + variáveis CSS cobrem todos os casos de uso reais aqui
- **Theme provider CSS-in-JS (ex: MUI)**: over-engineered; nenhum requisito nosso exige um runtime de tema completo

## Gatilho de revisão

Quando pelo menos dois dos seguintes forem verdadeiros, extrair para `@gifthy/ui`:
1. Um terceiro app frontend entra no monorepo
2. ≥ 5 primitivos precisam de atualizações em mais de um app por trimestre
3. Deriva visual é reportada como uma reclamação visível ao usuário
