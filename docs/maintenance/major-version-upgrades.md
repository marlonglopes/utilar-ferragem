# Plano de upgrade de versões principais

**Criado**: 2026-04-23. **Status**: Planejado — executar após Sprint 09.

Este documento registra os upgrades de versão **major** que foram deliberadamente adiados para não interromper o desenvolvimento de funcionalidades. Os upgrades de versão minor e patch (Node 20, Go 1.26, Gin 1.12, Postgres 17, Redpanda v26) já foram aplicados em 2026-04-23.

---

## Upgrades pendentes

| Pacote | Versão atual | Versão alvo | Risco | Breaking changes principais |
|--------|-------------|-------------|-------|----------------------------|
| React | 18.2 | 19.x | Alto | Novo compilador React, `use()` hook, server actions, mudanças em concurrent features |
| Vite | 5.x | 8.x | Alto | Rolldown bundler (Rust), mudanças de config, plugins podem não ser compatíveis |
| React Router | 6.22 | 7.x | Alto | Nova API de loaders/actions, `createBrowserRouter` permanece mas `<Route>` muda |
| Tailwind CSS | 3.4 | 4.x | Alto | Nova sintaxe CSS-first, `@apply` deprecado, requer Safari 16.4+/Chrome 111+ |
| Vitest | 2.1.9 | 4.x | Médio | API de coverage mudou, alguns matchers foram renomeados |
| TypeScript | 5.x | 6.x | Médio | Stricter checks, algumas APIs de tipo removidas |
| zustand | 4.5 | 5.x | Médio | `create` API mudou (sem `devtools` inline), persist middleware atualizado |

---

## Ordem recomendada

Executar em sequência, um pacote por vez, com suite de testes completa entre cada upgrade.

### Fase A — Ferramentas de build (baixo impacto em runtime)

1. **TypeScript 5 → 6**
   - Rodar `npx tsc --noEmit` antes e depois
   - Esperado: novos erros em tipos implícitos `any` e uso de `namespace`
   - Estimativa: 2–4h

2. **Vitest 2 → 4**
   - Atualizar `@vitest/coverage-istanbul` para v4 junto
   - Revisar matchers renomeados na [changelog do Vitest 4](https://vitest.dev/blog/)
   - Estimativa: 1–2h

### Fase B — Estado e roteamento

3. **zustand 4 → 5**
   - `create` passa a exigir `createStore` para uso fora de React
   - `persist` middleware: checar mudança de assinatura
   - Testar `authStore`, `cartStore`, `localeStore` com a nova API
   - Estimativa: 2–4h

4. **React Router 6 → 7**
   - Migrar `future` flags já presentes (`v7_startTransition`, `v7_relativeSplatPath`) — remove a necessidade delas
   - `useNavigate`, `useParams`, `<Link>` mantêm API; foco em `<Route>` e loaders
   - Atualizar testes que usam `MemoryRouter` com as novas flags
   - Estimativa: 3–5h

### Fase C — UI e estilos

5. **Tailwind CSS 3 → 4**
   - Maior esforço — nova sintaxe CSS-first (`@import "tailwindcss"` no lugar de `@tailwind base/components/utilities`)
   - `tailwind.config.ts` → migrar para CSS vars ou manter com `@config`
   - `@apply` pode exigir refatoração nos componentes de UI
   - Testar todas as páginas visualmente antes de considerar concluído
   - Estimativa: 1–2 dias
   - **Requisito**: verificar suporte de browser do público-alvo (Safari 16.4+)

### Fase D — Framework principal

6. **Vite 5 → 8**
   - Atualizar `@vitejs/plugin-react` junto
   - Revisar `vite.config.ts` — algumas opções foram movidas ou removidas
   - Rolldown pode ter comportamento diferente em imports dinâmicos e tree-shaking
   - Estimativa: 3–6h

7. **React 18 → 19**
   - Executar por último pois depende de Vite e TypeScript atualizados
   - Principais mudanças: `forwardRef` não é mais necessário, `use()` hook, `ref` como prop
   - Verificar dependências que ainda exigem React 18 (`@testing-library/react`, `react-hook-form`)
   - Atualizar `@types/react` e `@types/react-dom` junto
   - Estimativa: 4–8h

---

## Pré-requisitos antes de começar

- [ ] Suite de testes com ≥70% de cobertura nas libs críticas (atualmente ~39% geral)
- [ ] Build de produção (`npm run build`) passando sem warnings
- [ ] Sprint 09 concluído (nenhuma feature em desenvolvimento paralela)

## Como executar cada upgrade

```bash
# Exemplo para zustand
npm install zustand@5 --legacy-peer-deps
npx vitest run          # todos os testes devem passar
npm run build           # build de produção deve passar
git commit -m "chore: upgrade zustand 4 → 5"
```

Nunca fazer mais de um upgrade major no mesmo commit. Isso facilita o `git bisect` caso algo quebre em produção.

---

## Upgrades que NÃO faremos (por enquanto)

| Pacote | Motivo |
|--------|--------|
| React Router → Remix/TanStack Router | Reescrita completa; não há ganho imediato |
| Node.js 20 → 24 | Node 20 tem suporte LTS até abril 2026; upgrade em 2026-Q3 |
| Go 1.26 → 1.27+ | Upgrade incremental quando lançado; sem breaking changes esperados |
