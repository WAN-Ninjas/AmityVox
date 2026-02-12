-- Cross-channel quotes: track when a message quotes a message from another channel.
CREATE TABLE IF NOT EXISTS cross_channel_quotes (
    id TEXT PRIMARY KEY,
    message_id TEXT NOT NULL,
    quoted_message_id TEXT NOT NULL,
    quoted_channel_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_cross_channel_quotes_message ON cross_channel_quotes(message_id);
CREATE INDEX IF NOT EXISTS idx_cross_channel_quotes_quoted ON cross_channel_quotes(quoted_message_id);

-- Translation cache: cache translated message content to avoid redundant API calls.
CREATE TABLE IF NOT EXISTS translation_cache (
    id TEXT PRIMARY KEY,
    message_id TEXT NOT NULL,
    source_lang TEXT NOT NULL DEFAULT '',
    target_lang TEXT NOT NULL,
    translated_text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(message_id, target_lang)
);

CREATE INDEX IF NOT EXISTS idx_translation_cache_message ON translation_cache(message_id);

-- Per-thread notification preferences.
CREATE TABLE IF NOT EXISTS thread_notification_preferences (
    user_id TEXT NOT NULL,
    thread_id TEXT NOT NULL,
    level TEXT NOT NULL DEFAULT 'all', -- 'all', 'mentions', 'none'
    PRIMARY KEY (user_id, thread_id)
);

-- Sticker pack sharing: add share_code and shared columns to sticker_packs.
ALTER TABLE sticker_packs ADD COLUMN IF NOT EXISTS share_code TEXT UNIQUE;
ALTER TABLE sticker_packs ADD COLUMN IF NOT EXISTS shared BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_sticker_packs_share_code ON sticker_packs(share_code) WHERE share_code IS NOT NULL;
