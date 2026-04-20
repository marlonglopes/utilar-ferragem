# Fase 1 — Fundação

**Objetivo**: um SPA estruturado, implantável e com sistema de design pronto, que apenas renderiza páginas-shell. Zero lógica de produto. Esta fase trata de eliminar fricção para todos os sprints futuros.

## Sprints

- [Sprint 01 — Scaffold + ferramentas](../sprints/sprint-01-scaffold.md)
- [Sprint 02 — Design system + i18n + shells de layout](../sprints/sprint-02-design-system.md)

## Definição de pronto

- Vite + React + TS + Tailwind + ESLint + Prettier + Vitest todos configurados
- pt-BR é o locale padrão; en é selecionável
- `LocaleSwitcher`, `Navbar`, `Footer`, `CategoryRail` renderizam com conteúdo real
- Página de referência do design system em `/_dev/ui` renderiza todos os primitivos (botões, inputs, cards, badges, tags, modais, toasts)
- CI executa lint + build + test em todo push
- Preview implantado acessível (Vercel/Netlify/CloudFront staging)
- Integração com `VITE_API_URL` funciona; acessar retorna 200 do gateway

## Explicitamente fora do escopo

- Qualquer exibição de produto
- Qualquer chamada de API além de um health check do gateway
- Qualquer texto de marketing voltado ao usuário

## Saída para a Fase 2 quando

Todos os itens da definição de pronto estiverem marcados **e** o usuário tiver revisado a página de referência do design system e aprovado a direção visual.
