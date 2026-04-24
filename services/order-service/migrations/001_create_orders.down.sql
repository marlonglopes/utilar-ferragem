DROP TRIGGER IF EXISTS trg_orders_updated ON orders;
DROP FUNCTION IF EXISTS set_updated_at();

DROP TABLE IF EXISTS tracking_events;
DROP TABLE IF EXISTS shipping_addresses;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TYPE  IF EXISTS payment_method;
DROP TYPE  IF EXISTS order_status;
