-- Sticker packs
CREATE TABLE IF NOT EXISTS sticker_packs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    cover_sticker_id TEXT,
    owner_type TEXT NOT NULL DEFAULT 'guild', -- 'guild', 'user', or 'system'
    owner_id TEXT NOT NULL, -- guild_id or user_id depending on owner_type
    public BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sticker_packs_owner ON sticker_packs(owner_type, owner_id);
CREATE INDEX IF NOT EXISTS idx_sticker_packs_public ON sticker_packs(public) WHERE public = true;

-- Individual stickers
CREATE TABLE IF NOT EXISTS stickers (
    id TEXT PRIMARY KEY,
    pack_id TEXT NOT NULL REFERENCES sticker_packs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    tags TEXT,
    file_id TEXT NOT NULL,
    format TEXT NOT NULL DEFAULT 'png', -- 'png', 'apng', 'gif', 'lottie'
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_stickers_pack ON stickers(pack_id);

-- Channel templates
CREATE TABLE IF NOT EXISTS channel_templates (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    channel_type TEXT NOT NULL DEFAULT 'text',
    topic TEXT,
    slowmode_seconds INTEGER NOT NULL DEFAULT 0,
    nsfw BOOLEAN NOT NULL DEFAULT false,
    default_permissions BIGINT,
    permission_overwrites JSONB,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_channel_templates_guild ON channel_templates(guild_id);

-- Message bookmarks with optional reminders
ALTER TABLE message_bookmarks ADD COLUMN IF NOT EXISTS reminder_at TIMESTAMPTZ;
ALTER TABLE message_bookmarks ADD COLUMN IF NOT EXISTS reminded BOOLEAN NOT NULL DEFAULT false;
