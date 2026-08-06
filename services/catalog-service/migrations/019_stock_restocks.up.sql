-- ============================================================================
-- Reposição de estoque por DEVOLUÇÃO (restock) — guarda de idempotência
-- ----------------------------------------------------------------------------
-- POR QUÊ: quando uma devolução é RECEBIDA, o order-service chama
-- POST /api/v1/internal/restock para a mercadoria voltar ao saldo vendável. O
-- cliente HTTP (order-service/internal/catalogclient/restock.go) já existe e
-- CHAMA essa rota — mas a rota não existia aqui, então o estoque devolvido nunca
-- voltava (o recebimento era registrado, `stock_returned` ficava false e uma
-- linha `return.stock_restore_failed` entrava na trilha). Esta migration + o
-- handler fecham esse buraco.
--
-- Repor estoque é uma operação cujo efeito DUPLICADO é "vender o que não existe".
-- Por isso precisa ser idempotente, e a chave é a DEVOLUÇÃO (returnId), NÃO o
-- pedido: um pedido pode ter várias devoluções parciais, e chavear pelo pedido
-- faria a segunda ser descartada como duplicata da primeira — o estoque dela
-- nunca voltaria.
--
-- Esta tabela é o guarda: a 1ª reposição de um returnId insere a linha e executa
-- os incrementos na MESMA transação; uma 2ª chamada (retry de rede) colide no PK
-- e vira no-op. Ver docs/devolucao-e-troca.md.
--
-- Reversível.
-- ============================================================================
CREATE TABLE stock_restocks (
    return_id   TEXT PRIMARY KEY,
    reason      TEXT NOT NULL CHECK (btrim(reason) <> ''),
    item_count  INTEGER NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
