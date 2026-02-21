-- Rollback migration 063: Federation full parity rearchitecture

DROP INDEX IF EXISTS idx_fed_delivery_receipts_unique;

DROP TABLE IF EXISTS federation_events;

ALTER TABLE instances DROP CONSTRAINT IF EXISTS chk_instances_voice_mode;
ALTER TABLE instances DROP COLUMN IF EXISTS voice_mode;
ALTER TABLE instances DROP COLUMN IF EXISTS shorthand;
