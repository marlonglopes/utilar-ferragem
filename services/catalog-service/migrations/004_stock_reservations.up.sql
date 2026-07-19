-- ============================================================================
-- Reserva de estoque
-- ----------------------------------------------------------------------------
-- PORQUÊ: até aqui `products.stock` só era escrito por CRUD admin e import CSV.
-- Dava pra fechar um pedido de 10.000 unidades de um produto com estoque 1.
--
-- MODELO ESCOLHIDO — "decrementa na reserva":
-- Ao reservar, `products.stock` é decrementado na hora, dentro da MESMA
-- transação que insere a linha de reserva. `stock` passa a significar
-- "disponível para venda", não "físico em prateleira".
--
-- A alternativa (manter stock físico e calcular disponível = stock - SUM(reservas
-- ativas)) exige agregação a cada checagem e um lock de range pra ser correta
-- sob concorrência. O decremento direto resolve a corrida com um único
-- `UPDATE ... WHERE stock >= n` — atômico por definição no Postgres, porque o
-- UPDATE pega row lock e reavalia o predicado na versão mais recente da linha.
--
-- Consequência aceita: durante a janela de reserva a vitrine mostra estoque
-- menor. Para uma loja de ferragem isso é o comportamento desejado (não vender
-- o que já está separado pra outro cliente).
-- ============================================================================

CREATE TABLE IF NOT EXISTS stock_reservations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    TEXT NOT NULL,                    -- opaco, vem do order-service (sem FK cross-DB)
    product_id  UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    quantity    INT  NOT NULL CHECK (quantity > 0),
    status      TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'committed', 'released')),
    expires_at  TIMESTAMPTZ NOT NULL,             -- reserva não confirmada morre aqui
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Idempotência da reserva: um pedido só pode ter UMA reserva ativa por produto.
-- Retry do order-service (timeout de rede, redelivery do consumer) bate nesse
-- índice e vira no-op em vez de decrementar o estoque duas vezes.
CREATE UNIQUE INDEX IF NOT EXISTS idx_stock_reservations_active
    ON stock_reservations(order_id, product_id)
    WHERE status = 'active';

-- Sweeper de expiração varre por (status, expires_at).
CREATE INDEX IF NOT EXISTS idx_stock_reservations_expiry
    ON stock_reservations(expires_at)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_stock_reservations_order
    ON stock_reservations(order_id);

CREATE INDEX IF NOT EXISTS idx_stock_reservations_product
    ON stock_reservations(product_id);

-- Estoque nunca pode ficar negativo. Se algum caminho de código errar a conta,
-- queremos falhar alto na hora em vez de vender ar.
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_stock_non_negative;
ALTER TABLE products ADD CONSTRAINT products_stock_non_negative CHECK (stock >= 0);

CREATE TRIGGER trg_stock_reservations_updated
    BEFORE UPDATE ON stock_reservations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
