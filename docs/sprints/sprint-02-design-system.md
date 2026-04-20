# Sprint 02 — Design system + i18n + shells de layout

**Fase**: 1 — Fundação. **Estimativa**: 5–7 dias.

## Escopo

Construir os primitivos de UI reutilizáveis e os shells de layout. Carregar ambos os idiomas. Nada específico de produto ainda.

## Tarefas

1. Configurar i18next + react-i18next exatamente como em `gifthy-hub/src/i18n/` (namespaces: `common`, `catalog`, `checkout`, `account`)
2. Copiar `format.ts` (BRL, CEP, CPF, CNPJ, telefone, datas) do gifthy-hub e estender onde necessário
3. Criar `LocaleSwitcher`, `localeStore` (padrão pt-BR)
4. Construir o shell de layout: `PublicLayout` = `Navbar` + `CategoryRail` + `<Outlet />` + `Footer`
5. Construir a `Navbar`: logomarca, barra de busca (não funcional), ícone de conta, ícone de carrinho, seletor de idioma
6. Construir a `CategoryRail`: tira de pills com scroll horizontal com 8 nós de taxonomia de nível superior (lida de `src/lib/taxonomy.ts`)
7. Construir o `Footer`: links, ícones de formas de pagamento (Pix/boleto/cartões/BRL), aviso de LGPD, redes sociais
8. Construir os primitivos de UI: `Button` (primary/secondary/ghost/danger, todos os tamanhos), `Input`, `Select`, `Checkbox`, `Radio`, `Card`, `Badge`, `Tag`, `Modal`, `Drawer`, `Toast`, `Skeleton`, `Pagination`, `Breadcrumb`
9. Criar a página de referência `/_dev/ui` que renderiza cada primitivo em cada variante
10. Configurar o suporte a dark-mode no Tailwind (desligado por padrão, mas preparado para o futuro)
11. Adicionar `Archivo` + `Inter` + `JetBrains Mono` via Google Fonts + preconnect

## Critérios de aceite

- [ ] `/_dev/ui` renderiza todos os primitivos; usuário aprova os visuais
- [ ] O seletor de idioma alterna visivelmente todas as strings do shell
- [ ] O Footer exibe as logos de Pix/boleto/principais cartões
- [ ] Mobile: navbar recolhe para hambúrguer + busca expande para largura total; category rail tem scroll horizontal
- [ ] Lighthouse a11y ≥ 95 na página de referência
- [ ] Nenhuma string em inglês ou português hardcoded em nenhum componente do shell

## Dependências

- Sprint 01 concluído
- Logo final do usuário (ou acordo para lançar com placeholder em texto)

## Riscos

- Expansão descontrolada do escopo do design system — limitar aos primitivos listados; adiar qualquer novidade para um sprint separado
