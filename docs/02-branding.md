# 02 — Branding e Identidade Visual

Referência: [@utilar_ferragens no Instagram](https://www.instagram.com/utilar_ferragens/).

> **Status**: ponto de partida proposto. Substituir pelos assets finais de marca assim que o usuário fornecer logo, códigos hex exatos e licenças tipográficas.

## Personalidade da marca

| Dimensão | Utilar Ferragem | Por quê |
|----------|-----------------|---------|
| **Arquétipo** | O Homem Comum + O Artesão | Confiável, prático, sem rodeios; respeita a habilidade |
| **Voz** | Direta, segura, nativa em português | Sem brincadeiras, sem corporativismo — como um bom atendente de balcão |
| **Variação de tom** | Mais técnico para usuários pro, mais orientado para DIY | UX adaptada por persona (ver doc de visão) |
| **Evitar** | Linguagem "gamificada", exclamações, ironia | Comprar um saco de 25 kg de cimento não é um momento fofo |

## Paleta de cores (proposta)

Inspirada na sinalização clássica das ferragens brasileiras (laranja industrial quente + carvão escuro + acentos âmbar) e no tom profissional/utilitário do Instagram de referência.

| Token | Hex | Uso |
|-------|-----|-----|
| `--color-primary` | `#E85D04` | CTA principal, marca, nav ativo |
| `--color-primary-dark` | `#B84604` | Estados de hover, pressionado |
| `--color-primary-light` | `#FFB570` | Destaques, badges, anéis de foco |
| `--color-accent` | `#FFB703` | Promoções, tags de "oferta", avaliações |
| `--color-ink` | `#1F1F1F` | Texto do corpo, títulos |
| `--color-surface` | `#FAFAF7` | Fundo da página (branco suave e quente) |
| `--color-surface-alt` | `#F0EEE8` | Fundo de cards, barras laterais |
| `--color-border` | `#D8D4CC` | Divisores, bordas de input |
| `--color-muted` | `#6B6660` | Texto secundário, metadados |
| `--color-success` | `#2E7D32` | Em estoque, pedido confirmado |
| `--color-warning` | `#ED6C02` | Estoque baixo |
| `--color-danger` | `#C62828` | Erros, fora de estoque |

Esses tokens mapeiam 1:1 para variáveis CSS do Tailwind no tema da Utilar Ferragem (ver doc de arquitetura).

## Tipografia

| Papel | Família | Fallback | Pesos |
|-------|---------|----------|-------|
| **Display / títulos** | `Archivo` ou `Barlow Condensed` | `ui-sans-serif, system-ui` | 600, 700, 800 |
| **Corpo** | `Inter` | `ui-sans-serif, system-ui` | 400, 500, 600 |
| **Mono (SKUs, tabelas de especificação)** | `JetBrains Mono` | `ui-monospace` | 400, 600 |

Justificativa: sans condensado nos títulos remete ao visual "industrial / sinalização"; Inter é o cavalo de batalha pragmático para o corpo; mono torna as especificações de produto escaneáveis (dimensões, tensões, códigos SKU).

As três famílias são do Google Fonts, gratuitas para uso comercial e com bom desempenho nas velocidades de conexão médias do Brasil.

## Direção de logo

Até existir um logo oficial, usar uma marca tipográfica:

```
UTILAR FERRAGEM
```

- Caixa alta, `Archivo 800`
- `UTILAR` em `--color-ink`, `FERRAGEM` em `--color-primary`
- Espaçamento entre letras `0.02em`
- Opcional: um pequeno ícone de chave inglesa ou porca sextavada entre as palavras (ícone de linha, traço único)

O logo final deve funcionar com 24 px de altura (navbar) e 96 px de altura (splash / impressão). Solicitar quando o asset de marca for fornecido.

## Iconografia

- **Biblioteca**: [lucide-react](https://lucide.dev/) (já usada no gifthy-hub)
- **Espessura do traço**: 2 px, uniforme
- **Tamanhos**: apenas 16 / 20 / 24 / 32 px — sem tamanhos arbitrários
- **Cor**: herda do `currentColor`; nunca preenchido

## Imagens

| Caso de uso | Tratamento |
|-------------|------------|
| Fotos de produto | Fundo branco ou cinza claro, produto centralizado, adereços mínimos |
| Blocos de categoria | Ilustração flat OU fotografia com filtro de luz quente uniforme |
| Banners de hero | Fotografia real de contexto de oficina / obra, nunca "mulher sorrindo com furadeira" de banco de imagem |
| Estados vazios | Ícone lucide único em `--color-muted`, mensagem curta e útil |

## Voz: faça / não faça

| Faça | Não faça |
|------|---------|
| "Entrega em 3 dias úteis para 01310-100." | "Yay! Your stuff is on the way! 🎉" |
| "Sem estoque. Avisar quando voltar?" | "Oh no! We're all out! 😢" |
| "CNPJ necessário para nota fiscal." | "We'll need some info from you, hero!" |
| Use termos técnicos livremente (bitola, rosca BSP, tensão 127V/220V) | Explique em excesso termos do setor para usuários pro |

## Princípios de layout

1. **Densidade em vez de espaço em branco** nas páginas de categoria/busca — profissionais precisam comparar rápido.
2. **Espaçamento generoso** no detalhe do produto e no checkout — confiança e clareza importam mais aqui.
3. **CTA fixo** — botão principal "Adicionar ao carrinho" sempre visível no mobile via rodapé fixo na página de produto.
4. **Especificações como dados, não como prosa** — fichas técnicas são renderizadas como tabelas chave-valor, nunca em parágrafos.

## Piso de acessibilidade

- Contraste mínimo WCAG 2.1 AA em todos os textos
- Navegação por teclado em todos os fluxos (não apenas no checkout)
- Labels de screen reader em todos os botões somente com ícone
- Área de toque ≥ 44 × 44 px para todos os elementos clicáveis
- Respeitar `prefers-reduced-motion`

## Perguntas em aberto para o usuário

1. Existe algum logo da Utilar Ferragem que devemos usar? Se sim, por favor compartilhe o SVG.
2. Há restrições legais para usar o nome "Utilar Ferragem" — é uma parceria, aquisição ou marca nova sem relação com a referência do Instagram?
3. Existem diretrizes de trademark ou identidade visual da marca de referência que precisamos seguir ou evitar?
