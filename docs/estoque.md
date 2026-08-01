# Estoque — ajuste com motivo, histórico e alerta de baixo

**O que é:** a tela do **almoxarife** (`/admin/estoque`). Antes só dava para
**sobrescrever** o número absoluto do estoque (pelo PATCH do produto). Agora o
almoxarife confere a prateleira, dá **entrada** de recebimento (+) e **baixa**
de avaria (−), cada movimento com **motivo** e rastro de quem/quando.

## Regras que importam

- **Ajuste é RELATIVO (delta), não sobrescrita.** Lança-se a diferença, não o
  total. Motivo é **obrigatório** — "estoque mudou sem motivo" obriga a caçar o
  porquê depois.
- **Nunca mostra custo.** O almoxarife não vê custo (persona). A proteção é a
  **projeção**: o endpoint de estoque não seleciona a coluna `cost` — não é
  filtro depois, é a coluna nunca sair do banco.
- **Atômico.** O ajuste trava a linha do produto (`FOR UPDATE`), aplica, grava o
  movimento e a trilha (`stock.adjust`), tudo na mesma transação. Recusa deixar
  o estoque **negativo** (409 — respeita o `CHECK stock >= 0`).
- **Alerta de baixo.** `stock <= low_stock_threshold` (limite por produto,
  default 5). Os baixos sobem ao topo da lista.
- **Toda mudança vira trilha.** O `stock.adjust` aparece na **atividade
  unificada** com o de→para e o motivo.

## API (catalog-service)

| Método | Rota | Papéis | O quê |
|---|---|---|---|
| GET | `/api/v1/admin/stock?q=&low=1` | admin, almoxarife, vendas | Lista (sem custo), baixos no topo |
| GET | `/api/v1/admin/stock/:id/movements` | admin, almoxarife, vendas | Histórico do produto |
| POST | `/api/v1/admin/stock/:id/adjust` | admin, almoxarife | `{delta, reason}` → ajuste |

Conjuntos de papéis: `handler.StockReadRoles` / `StockWriteRoles`.

## Dados

- migration `018_stock_movements`: coluna `products.low_stock_threshold`
  (`NUMERIC(14,3)`, default 5) + tabela `stock_movements` (delta, motivo, estoque
  resultante, ator, request_id, created_at).
- `resulting_stock` é redundante de propósito: permite reconciliar a série sem
  recomputar e detecta se alguém mexeu no `stock` por fora deste caminho.

## Débito conhecido

O **PATCH do produto** (`/admin/products/:id`) ainda seta o `stock` de forma
absoluta **sem** gerar um `stock_movements` — é o caminho legado que o `vendas`
usa pela tela de Produtos. Unificar tudo sob movimento (todo write de estoque
gera uma linha) é um follow-up: mantém a série de histórico completa.
