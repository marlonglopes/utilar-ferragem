# Sprint 01 — Scaffold + tooling

**Fase**: 1 — Fundação. **Estimativa**: 3–5 dias.

## Escopo

Subir o projeto. Nada voltado ao usuário final ainda.

## Tarefas

1. `npm create vite@latest utilar-ferragem -- --template react-ts`
2. Adicionar Tailwind 3, configurado com variáveis CSS para o tema (ver [doc de branding](../02-branding.md))
3. Adicionar ESLint + Prettier + lint-staged + husky (seguir exatamente as configs de `gifthy-hub/`)
4. Adicionar Vitest 2 + happy-dom + @testing-library/react; escrever um smoke test
5. Adicionar React Router 6 com uma rota home de placeholder
6. Adicionar Zustand + TanStack Query + providers básicos em `App.tsx`
7. Adicionar `src/lib/api.ts` — wrapper de fetch, `isApiEnabled`, anexar JWT
8. Adicionar `.env.example` com `VITE_API_URL=`
9. Adicionar workflow de CI: `lint + build + test` no push (reaproveitar o do gifthy-hub, se disponível)
10. Adicionar README com os comandos de dev / build / test
11. Escolher o destino de deploy (S3+CloudFront conforme [ADR 001](../adr/001-placement-and-stack.md)) e criar um script de deploy mínimo

## Critérios de aceite

- [ ] `npm run dev` sobe em `http://localhost:5174`
- [ ] `npm run build` gera `dist/` com um único HTML + JS chunked
- [ ] `npm run lint` termina com zero erros
- [ ] `npm test` executa o smoke test e passa
- [ ] CI fica verde no push para uma feature branch
- [ ] Uma URL de staging está no ar e acessível

## Dependências

Nenhuma. Este é o primeiro sprint.

## Riscos

- Incompatibilidade de versão do Node — confirmar 20+ antes de começar (conforme CLAUDE.md do projeto pai)
- Sistema de tokens do tema Tailwind — fazer um spike de meio dia para decidir entre CSS vars e apenas config do Tailwind
