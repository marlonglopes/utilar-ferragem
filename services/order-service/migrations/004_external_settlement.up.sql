-- ============================================================================
-- Liquidação externa: venda de balcão paga na MAQUININHA DA LOJA
-- ----------------------------------------------------------------------------
-- A maquininha é de um adquirente próprio, FORA da Appmax. O dinheiro entra por
-- fora do nosso PSP: não há cobrança, não há webhook, não há prova externa.
--
-- Até aqui o PDV gravava essas vendas como `card`. Valor e desconto certos,
-- meio de pagamento errado — e o erro custava caro:
--   * o livro contábil registrava uma transação de PSP que nunca existiu;
--   * a conciliação com a Appmax acusaria divergência para sempre;
--   * o relatório por método de pagamento somava maquininha com cartão online.
--
-- Esta migration abre o método `external` e as colunas do comprovante.
-- ============================================================================

-- ---------------------------------------------------------------------------
-- 1. O enum `payment_method` ganha 'external'
-- ---------------------------------------------------------------------------
-- PORQUÊ recriar o tipo em vez de `ALTER TYPE ... ADD VALUE`:
-- ADD VALUE é IRREVERSÍVEL (Postgres não remove valor de enum) e, até o PG12,
-- não roda dentro de transação — o golang-migrate envolve cada migration numa,
-- então o down desta migration não teria como existir. Recriar o tipo e trocar
-- a coluna é transacional e reversível, que é o requisito.
ALTER TYPE payment_method RENAME TO payment_method_old;

CREATE TYPE payment_method AS ENUM ('pix', 'boleto', 'card', 'external');

ALTER TABLE orders
    ALTER COLUMN payment_method TYPE payment_method
    USING payment_method::text::payment_method;

DROP TYPE payment_method_old;

-- ---------------------------------------------------------------------------
-- 2. O comprovante da maquininha
-- ---------------------------------------------------------------------------
-- NSU (Número Sequencial Único) é o campo do comprovante que aparece também no
-- extrato do adquirente. É o que permite ao financeiro casar "esta venda" com
-- "esta linha do extrato". Sem ele, liquidação externa é palavra do operador.
ALTER TABLE orders ADD COLUMN external_nsu            TEXT;
ALTER TABLE orders ADD COLUMN external_brand          TEXT;
ALTER TABLE orders ADD COLUMN external_auth_code      TEXT;
-- Quem liquidou e quando. Duplicado em balcao_audit_events de propósito: a
-- trilha é o histórico (e pode ser arquivada), estas colunas são o ESTADO atual
-- do pedido, consultável sem varrer a trilha inteira.
ALTER TABLE orders ADD COLUMN external_settled_by     TEXT;
ALTER TABLE orders ADD COLUMN external_settled_at     TIMESTAMPTZ;

-- ---------------------------------------------------------------------------
-- 3. Invariantes no BANCO, não só no handler
-- ---------------------------------------------------------------------------
-- Estas são as barreiras que sobrevivem a um bug no handler, a um script de
-- manutenção e a um endpoint futuro que ninguém lembrou de proteger.

-- Só venda de BALCÃO se liquida por fora. Um pedido web marcado como pago sem
-- PSP é mercadoria saindo com base em nada.
ALTER TABLE orders ADD CONSTRAINT orders_external_only_balcao
    CHECK (payment_method <> 'external' OR channel = 'balcao');

-- Liquidação sem rastro até a PESSOA e sem NSU é exatamente o registro que não
-- pode existir: é o caminho natural de fraude interna. Ou os três campos
-- existem juntos, ou não existe liquidação.
ALTER TABLE orders ADD CONSTRAINT orders_external_settlement_complete
    CHECK (
        (external_nsu IS NULL AND external_settled_by IS NULL AND external_settled_at IS NULL)
        OR
        (external_nsu IS NOT NULL AND external_settled_by IS NOT NULL AND external_settled_at IS NOT NULL)
    );

-- Um NSU liquida UM pedido DENTRO DA MESMA LOJA. O mesmo comprovante em dois
-- pedidos significa uma venda cobrada uma vez e baixada duas — a metade que
-- "sobra" sai como mercadoria sem contrapartida.
--
-- PORQUÊ escopado por loja e não global: o NSU é sequencial POR TERMINAL do
-- adquirente, então duas lojas colidem naturalmente no mesmo número. Um índice
-- global recusaria vendas legítimas da segunda loja que chegasse ao número —
-- e recusar venda de verdade é pior que o risco que se está tentando cobrir.
--
-- LIMITAÇÃO CONHECIDA: uma loja com mais de um terminal ainda pode ter dois
-- comprovantes legítimos com o mesmo NSU. Não coletamos o número do terminal
-- hoje; quando coletarmos, a chave certa é (loja, terminal, NSU, data).
-- Enquanto isso, a colisão vira 409 e o operador chama o financeiro — falso
-- positivo raro, e barulhento, é o lado certo para errar aqui.
--
-- Índice parcial: pedidos não liquidados (NULL) não competem entre si.
CREATE UNIQUE INDEX idx_orders_external_nsu_unico
    ON orders(store_id, external_nsu) WHERE external_nsu IS NOT NULL;

-- Consulta do financeiro: "o que foi liquidado na maquininha desta loja hoje".
CREATE INDEX idx_orders_external_settled
    ON orders(store_id, external_settled_at DESC) WHERE external_nsu IS NOT NULL;
