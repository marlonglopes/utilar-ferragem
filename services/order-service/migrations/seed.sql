-- ============================================================================
-- Seed do order-service
-- ----------------------------------------------------------------------------
-- 60 pedidos distribuídos entre 20 usuários (UUIDs sintéticos) nos vários
-- status do ciclo. Item IDs e seller IDs são strings opacas sem FK cross-DB.
-- ============================================================================

BEGIN;

TRUNCATE TABLE tracking_events, shipping_addresses, order_items, orders RESTART IDENTITY CASCADE;

WITH
user_pool AS (
    SELECT
        'user-' || lpad(n::text, 3, '0') AS user_id,    -- TEXT opaco; auth-service no futuro fornecerá
        n AS idx
    FROM generate_series(1, 20) AS n
),
order_seed AS (
    SELECT
        n,
        (SELECT user_id FROM user_pool WHERE idx = ((n - 1) % 20) + 1) AS user_id,
        CASE (n % 6)
            WHEN 0 THEN 'delivered'::order_status
            WHEN 1 THEN 'shipped'::order_status
            WHEN 2 THEN 'picking'::order_status
            WHEN 3 THEN 'paid'::order_status
            WHEN 4 THEN 'pending_payment'::order_status
            ELSE       'cancelled'::order_status
        END AS status,
        (ARRAY['pix','boleto','card']::payment_method[])[((n - 1) % 3) + 1] AS method,
        round((89 + (n * 47) % 4500)::numeric, 2) AS subtotal_val,
        round(((n % 5) * 9.90)::numeric, 2) AS shipping_val,
        n AS seq
    FROM generate_series(1, 60) AS n
)
INSERT INTO orders (
    id, number, user_id, status, payment_method, payment_id, payment_info,
    subtotal, shipping_cost, total, tracking_code,
    created_at, paid_at, picked_at, shipped_at, delivered_at, cancelled_at
)
SELECT
    ('00000000-0000-4000-b000-' || lpad(to_hex(os.seq), 12, '0'))::uuid,
    '2026-' || lpad(os.seq::text, 4, '0'),
    os.user_id,
    os.status,
    os.method,
    CASE WHEN os.status IN ('paid','picking','shipped','delivered')
         THEN ('00000000-0000-4000-a000-' || lpad(to_hex(os.seq), 12, '0'))::uuid
         ELSE NULL END,
    CASE os.method
        WHEN 'pix'    THEN 'Pix · pago em 10/04 14:32'
        WHEN 'boleto' THEN 'Boleto · vencimento 15/04'
        WHEN 'card'   THEN 'Cartão · 3x sem juros'
    END,
    os.subtotal_val,
    os.shipping_val,
    os.subtotal_val + os.shipping_val,
    CASE WHEN os.status IN ('shipped','delivered') THEN 'BR' || lpad(os.seq::text, 9, '0') || 'BR' ELSE NULL END,
    now() - ((os.seq * 2) || ' days')::interval,
    CASE WHEN os.status IN ('paid','picking','shipped','delivered') THEN now() - (((os.seq * 2) - 1) || ' days')::interval ELSE NULL END,
    CASE WHEN os.status IN ('picking','shipped','delivered') THEN now() - (((os.seq * 2) - 2) || ' days')::interval ELSE NULL END,
    CASE WHEN os.status IN ('shipped','delivered') THEN now() - (((os.seq * 2) - 3) || ' days')::interval ELSE NULL END,
    CASE WHEN os.status = 'delivered' THEN now() - (((os.seq * 2) - 5) || ' days')::interval ELSE NULL END,
    CASE WHEN os.status = 'cancelled' THEN now() - ((os.seq) || ' days')::interval ELSE NULL END
FROM order_seed os;

-- -- order items: 1 a 3 items por pedido --
INSERT INTO order_items (order_id, product_id, name, icon, seller_id, seller_name, quantity, unit_price)
SELECT
    o.id,
    gen_random_uuid(),
    CASE (row_number() OVER (PARTITION BY o.id ORDER BY item_seq)) % 6
        WHEN 0 THEN 'Furadeira Bosch GSB 13 RE'
        WHEN 1 THEN 'Tinta Acrílica Suvinil 18L'
        WHEN 2 THEN 'Cimento CP II-E-32 50kg'
        WHEN 3 THEN 'Parafusos Sortidos 500pç'
        WHEN 4 THEN 'Martelo Tramontina'
        ELSE       'Capacete de Segurança CA 31469'
    END,
    (ARRAY['⚒','▥','◫','▣','⚒','⚠'])[((row_number() OVER (PARTITION BY o.id ORDER BY item_seq))::int % 6) + 1],
    (ARRAY['ferragem-silva','tintas-rio','casa-obra','parafusos-sp','pro-tools-br','epi-pro'])[((row_number() OVER (PARTITION BY o.id ORDER BY item_seq))::int % 6) + 1],
    (ARRAY['Ferragem Silva','Tintas Rio','Casa & Obra','Parafusos SP','Pro Tools BR','EPI Pro'])[((row_number() OVER (PARTITION BY o.id ORDER BY item_seq))::int % 6) + 1],
    1 + (item_seq % 3),
    round((19.90 + (item_seq * 37) % 500)::numeric, 2)
FROM orders o
CROSS JOIN generate_series(1, 2) AS item_seq;

-- shipping addresses
INSERT INTO shipping_addresses (order_id, street, number, complement, neighborhood, city, state, cep)
SELECT
    o.id,
    'Rua das Ferragens',
    (100 + (row_number() OVER (ORDER BY o.created_at)))::text,
    CASE WHEN (row_number() OVER (ORDER BY o.created_at)) % 2 = 0 THEN 'Apto 42' ELSE NULL END,
    'Centro',
    'São Paulo',
    'SP',
    lpad((1000 + (row_number() OVER (ORDER BY o.created_at)))::text, 5, '0') || '-000'
FROM orders o;

-- tracking events: uma linha por transição até o status atual
INSERT INTO tracking_events (order_id, status, description, occurred_at)
SELECT o.id, 'pending_payment', 'Pedido criado. Aguardando pagamento.', o.created_at
FROM orders o;

INSERT INTO tracking_events (order_id, status, description, occurred_at)
SELECT o.id, 'paid', 'Pagamento confirmado.', o.paid_at
FROM orders o WHERE o.paid_at IS NOT NULL;

INSERT INTO tracking_events (order_id, status, location, description, occurred_at)
SELECT o.id, 'picking', 'CD São Paulo', 'Pedido em separação.', o.picked_at
FROM orders o WHERE o.picked_at IS NOT NULL;

INSERT INTO tracking_events (order_id, status, location, description, occurred_at)
SELECT o.id, 'shipped', 'Em trânsito', 'Pedido enviado via Correios.', o.shipped_at
FROM orders o WHERE o.shipped_at IS NOT NULL;

INSERT INTO tracking_events (order_id, status, description, occurred_at)
SELECT o.id, 'delivered', 'Pedido entregue com sucesso.', o.delivered_at
FROM orders o WHERE o.delivered_at IS NOT NULL;

INSERT INTO tracking_events (order_id, status, description, occurred_at)
SELECT o.id, 'cancelled', 'Pedido cancelado.', o.cancelled_at
FROM orders o WHERE o.cancelled_at IS NOT NULL;

COMMIT;

SELECT 'orders'             AS table_name, count(*) AS rows FROM orders
UNION ALL
SELECT 'order_items',         count(*) FROM order_items
UNION ALL
SELECT 'shipping_addresses',  count(*) FROM shipping_addresses
UNION ALL
SELECT 'tracking_events',     count(*) FROM tracking_events;
