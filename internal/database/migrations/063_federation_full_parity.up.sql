-- Migration 063: Federation full parity rearchitecture
-- Adds instance shorthand/voice_mode columns, federation_events table
-- for backfill support, and fixes duplicate delivery receipt bug.

-- ============================================================
-- INSTANCES: shorthand badge + voice relay mode
-- ============================================================

ALTER TABLE instances ADD COLUMN IF NOT EXISTS shorthand VARCHAR(5);
ALTER TABLE instances ADD COLUMN IF NOT EXISTS voice_mode VARCHAR(10) NOT NULL DEFAULT 'direct';

-- Add CHECK constraint separately for compatibility
DO $$ BEGIN
    ALTER TABLE instances ADD CONSTRAINT chk_instances_voice_mode
        CHECK (voice_mode IN ('direct', 'relay'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- ============================================================
-- FEDERATION EVENTS (ordered event log for backfill & replay)
-- ============================================================

CREATE TABLE IF NOT EXISTS federation_events (
    id              TEXT PRIMARY KEY,
    instance_id     TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    event_type      TEXT NOT NULL,
    guild_id        TEXT,
    channel_id      TEXT,
    hlc_wall_ms     BIGINT NOT NULL,
    hlc_counter     INTEGER NOT NULL DEFAULT 0,
    payload         JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_federation_events_instance_hlc
    ON federation_events(instance_id, hlc_wall_ms, hlc_counter);

CREATE INDEX IF NOT EXISTS idx_federation_events_guild
    ON federation_events(guild_id);

-- ============================================================
-- DELIVERY RECEIPTS: unique constraint to prevent duplicates
-- ============================================================

-- Deduplicate existing rows before creating the unique index. Keep the row
-- with the lowest ctid (i.e. the earliest-inserted physical tuple) and
-- delete all later duplicates with the same (message_id, source_instance,
-- target_instance) triple. This prevents the CREATE UNIQUE INDEX from
-- failing on databases that already contain duplicate delivery receipts.
DELETE FROM federation_delivery_receipts a
USING federation_delivery_receipts b
WHERE a.ctid < b.ctid
  AND a.message_id = b.message_id
  AND a.source_instance = b.source_instance
  AND a.target_instance = b.target_instance;

CREATE UNIQUE INDEX IF NOT EXISTS idx_fed_delivery_receipts_unique
    ON federation_delivery_receipts(message_id, source_instance, target_instance);
