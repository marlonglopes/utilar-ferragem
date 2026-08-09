ALTER TABLE cashback_config
    DROP COLUMN IF EXISTS min_earn_subtotal,
    DROP COLUMN IF EXISTS min_redeem_subtotal,
    DROP COLUMN IF EXISTS campaign_rate_pct,
    DROP COLUMN IF EXISTS campaign_starts_at,
    DROP COLUMN IF EXISTS campaign_ends_at;
