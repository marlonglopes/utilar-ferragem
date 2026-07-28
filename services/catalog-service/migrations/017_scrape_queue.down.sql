DROP TRIGGER IF EXISTS trg_scrape_queue_updated ON scrape_queue;
DROP INDEX IF EXISTS idx_scrape_queue_pending;
DROP TABLE IF EXISTS scrape_queue;
