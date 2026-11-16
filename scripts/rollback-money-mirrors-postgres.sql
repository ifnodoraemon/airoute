-- Offline rollback for migration 55. Stop ALL gateway writers first and take a
-- database backup. Restore the mirrors from units before starting an older binary.
BEGIN;
LOCK TABLE users, api_keys, usage_logs IN ACCESS EXCLUSIVE MODE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS balance DOUBLE PRECISION DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS budget DOUBLE PRECISION DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS used_cost DOUBLE PRECISION DEFAULT 0;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS cost DOUBLE PRECISION DEFAULT 0;
UPDATE users SET balance = COALESCE(balance_units, 0) / 1000000000.0;
UPDATE api_keys SET budget = COALESCE(budget_units, 0) / 1000000000.0,
    used_cost = COALESCE(used_cost_units, 0) / 1000000000.0;
UPDATE usage_logs SET cost = COALESCE(cost_units, 0) / 1000000000.0;
DELETE FROM schema_migrations WHERE version = 55 AND name = 'drop_float_money_mirrors';
COMMIT;
