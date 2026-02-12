-- Add tags column to guilds for discovery categorization.
ALTER TABLE guilds ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

-- Index for efficient tag-based discovery filtering.
CREATE INDEX IF NOT EXISTS idx_guilds_tags ON guilds USING GIN (tags);
