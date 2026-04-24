-- ============================================================================
-- Seed data for payment-service
-- ----------------------------------------------------------------------------
-- Populates payments, webhook_events e payments_outbox com volume suficiente
-- para smoke tests, paginação, filtros e carga leve.
--
-- Executado por:  make db-seed   (ver Makefile raiz)
--
-- Nota: users/orders *não* existem nesse DB — vivem nos mocks do frontend.
-- Aqui apenas geramos 100 UUIDs de usuários fictícios e os reutilizamos
-- através dos pagamentos para dar verossimilhança aos dados.
-- ============================================================================

BEGIN;

-- Limpa antes de popular (idempotente)
TRUNCATE TABLE payments_outbox, webhook_events, payments RESTART IDENTITY CASCADE;

-- ---------------------------------------------------------------------------
-- 1) Pool de 100 users e 150 orders fictícios (CTE reaproveitada adiante)
-- ---------------------------------------------------------------------------
WITH
user_pool AS (
    SELECT
        ('00000000-0000-4000-8000-' || lpad(to_hex(n), 12, '0'))::uuid AS user_id,
        n AS idx
    FROM generate_series(1, 100) AS n
),
order_pool AS (
    SELECT
        ('00000000-0000-4000-9000-' || lpad(to_hex(n), 12, '0'))::uuid AS order_id,
        -- distribui orders entre os 100 users ciclicamente
        (SELECT user_id FROM user_pool WHERE idx = ((n - 1) % 100) + 1) AS user_id,
        n AS idx
    FROM generate_series(1, 150) AS n
)

-- ---------------------------------------------------------------------------
-- 2) Payments — 150 linhas, distribuídas entre métodos e status
-- ---------------------------------------------------------------------------
INSERT INTO payments (
    id, order_id, user_id, method, status, amount, currency,
    psp_payment_id, psp_metadata, psp_payload,
    confirmed_at, expires_at, created_at, updated_at
)
SELECT
    ('00000000-0000-4000-a000-' || lpad(to_hex(o.idx), 12, '0'))::uuid,
    o.order_id,
    o.user_id,
    -- method rotaciona: pix, boleto, card, pix, boleto, card...
    (ARRAY['pix','boleto','card']::payment_method[])[((o.idx - 1) % 3) + 1],
    -- status distribuído: 60% confirmed, 20% pending, 10% expired, 5% failed, 5% cancelled
    CASE
        WHEN o.idx % 20 = 0 THEN 'cancelled'::payment_status
        WHEN o.idx % 20 = 1 THEN 'failed'::payment_status
        WHEN o.idx % 10 = 2 THEN 'expired'::payment_status
        WHEN o.idx % 5  = 3 THEN 'pending'::payment_status
        ELSE                      'confirmed'::payment_status
    END,
    -- amount: 49,90 — 4.999,90 (pseudo-aleatório determinístico)
    round((49.90 + (o.idx * 37) % 4950)::numeric, 2),
    'BRL',
    'MP-' || lpad(o.idx::text, 10, '0'),
    jsonb_build_object(
        'seed', true,
        'idx', o.idx,
        'installments', (o.idx % 12) + 1
    ),
    jsonb_build_object(
        'qr_code', 'PIX-QR-' || o.idx,
        'copy_paste', '00020126' || lpad(o.idx::text, 10, '0')
    ),
    -- confirmed_at apenas em pagamentos confirmed
    CASE
        WHEN o.idx % 20 = 0 OR o.idx % 20 = 1 OR o.idx % 10 = 2 OR o.idx % 5 = 3
        THEN NULL
        ELSE now() - ((o.idx % 30) || ' days')::interval
    END,
    -- expires_at: 30min para pix/card, 3 dias para boleto
    CASE ((o.idx - 1) % 3) + 1
        WHEN 2 THEN now() + interval '3 days'
        ELSE         now() + interval '30 minutes'
    END,
    now() - ((o.idx % 90) || ' days')::interval,
    now() - ((o.idx % 90) || ' days')::interval
FROM order_pool o;

-- ---------------------------------------------------------------------------
-- 3) Webhook events — 1 evento 'payment.created' + 1 'payment.confirmed'
--    para cada pagamento confirmed; 'payment.failed' para failed.
--    Unique: (psp_id, psp_payment_id, event_type)
-- ---------------------------------------------------------------------------
INSERT INTO webhook_events (psp_id, psp_payment_id, event_type, raw_payload, received_at)
SELECT
    'mercadopago',
    p.psp_payment_id,
    'payment.created',
    jsonb_build_object(
        'id', p.psp_payment_id,
        'status', 'pending',
        'amount', p.amount,
        'seed', true
    ),
    p.created_at
FROM payments p;

INSERT INTO webhook_events (psp_id, psp_payment_id, event_type, raw_payload, received_at)
SELECT
    'mercadopago',
    p.psp_payment_id,
    CASE p.status
        WHEN 'confirmed' THEN 'payment.confirmed'
        WHEN 'failed'    THEN 'payment.failed'
        WHEN 'expired'   THEN 'payment.expired'
        WHEN 'cancelled' THEN 'payment.cancelled'
    END,
    jsonb_build_object(
        'id', p.psp_payment_id,
        'status', p.status,
        'amount', p.amount,
        'confirmed_at', p.confirmed_at,
        'seed', true
    ),
    COALESCE(p.confirmed_at, p.created_at + interval '5 minutes')
FROM payments p
WHERE p.status IN ('confirmed', 'failed', 'expired', 'cancelled');

-- ---------------------------------------------------------------------------
-- 4) Payments outbox — 1 entry por pagamento confirmed (published) +
--    20 entries pendentes (não publicadas ainda) para simular fila
-- ---------------------------------------------------------------------------
INSERT INTO payments_outbox (event_type, payload_json, attempts, published_at, next_attempt_at, created_at)
SELECT
    'payment.confirmed',
    jsonb_build_object(
        'payment_id', p.id,
        'order_id', p.order_id,
        'user_id', p.user_id,
        'amount', p.amount,
        'seed', true
    ),
    1,
    p.confirmed_at + interval '2 seconds',
    p.confirmed_at,
    p.confirmed_at
FROM payments p
WHERE p.status = 'confirmed';

-- 20 entries pendentes com attempts variados (simula retry backoff)
INSERT INTO payments_outbox (event_type, payload_json, attempts, published_at, next_attempt_at, created_at)
SELECT
    'payment.confirmed',
    jsonb_build_object(
        'payment_id', gen_random_uuid(),
        'order_id', gen_random_uuid(),
        'user_id', gen_random_uuid(),
        'amount', round((100 + n * 13)::numeric, 2),
        'seed', true,
        'retry_test', true
    ),
    (n % 5),                                   -- attempts: 0..4
    NULL,                                      -- não publicado
    now() + ((n % 10) || ' minutes')::interval,
    now() - ((n % 60) || ' minutes')::interval
FROM generate_series(1, 20) AS n;

COMMIT;

-- ---------------------------------------------------------------------------
-- Relatório final
-- ---------------------------------------------------------------------------
SELECT 'payments'         AS table_name, count(*) AS rows FROM payments
UNION ALL
SELECT 'webhook_events',   count(*) FROM webhook_events
UNION ALL
SELECT 'payments_outbox',  count(*) FROM payments_outbox;
