# Sprint 04 — Página de detalhe do produto

**Fase**: 2 — Catálogo. **Estimativa**: 5 dias.

## Escopo

A superfície de conversão mais importante. Construir com cuidado.

## Tarefas

1. Adicionar o hook `GET /api/v1/products/:id`
2. Construir a `ProductDetailPage` (`/produto/:id`)
   - Esquerda: galeria de imagens com rail de miniaturas + lightbox
   - Direita: título, nome do vendedor + link, preço, badge de estoque, seletor de quantidade, CTA principal ("Adicionar ao carrinho" — não funcional no Sprint 04), secundário ("Comprar agora" — oculto até a Fase 3)
   - Abaixo: seções em abas — Descrição / Especificações / Avaliações (avaliações exibem dados stub até a Fase 5)
3. Construir o componente `SpecSheet`: renderiza uma tabela chave-valor a partir de `product.specs` (JSONB); cai em descrição se specs estiver ausente
4. Construir `StockBadge`: "Em estoque", "Últimas unidades" (< 10), "Sem estoque" (= 0), "Sob consulta" (null)
5. Construir `SellerCard`: avatar, nome, stub de avaliação, "Ver loja"
6. Adicionar rail de "Produtos relacionados" na parte inferior (primeiros 4 produtos da mesma categoria, excluindo o atual)
7. Mobile: rodapé de CTA fixo com preço + adicionar ao carrinho
8. Galeria de imagens: acessível pelo teclado, deslizável no touch, pré-carrega a próxima imagem

## Critérios de aceite

- [ ] O lightbox da galeria abre + fecha + navega com ← / →
- [ ] A tabela de especificações renderiza corretamente para produtos com e sem `specs`
- [ ] O seletor de quantidade respeita o limite de estoque; desabilita "Adicionar ao carrinho" com zero unidades
- [ ] A página é navegável e utilizável com uma mão só no celular
- [ ] Lighthouse LCP ≤ 2,5s em 4G (a imagem principal é o candidato ao LCP — otimizá-la)

## Dependências

- Sprint 03 concluído
- Coluna `products.specs` adicionada (aditiva, nullable — segura para deploy independente)

## Riscos

- O schema de specs por categoria vai evoluir — ocultar linhas vazias; não retornar 404 para chaves de spec não reconhecidas
