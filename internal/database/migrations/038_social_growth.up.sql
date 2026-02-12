-- Migration 038: Social & Growth features
-- Server insights, boosts, vanity URLs, achievements, leveling, starboard,
-- welcome messages, and auto-role assignment.

-- ============================================================
-- Server Insights / Analytics (aggregated stats snapshots)
-- ============================================================
CREATE TABLE IF NOT EXISTS guild_insights_daily (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    member_count INT NOT NULL DEFAULT 0,
    members_joined INT NOT NULL DEFAULT 0,
    members_left INT NOT NULL DEFAULT 0,
    messages_sent INT NOT NULL DEFAULT 0,
    reactions_added INT NOT NULL DEFAULT 0,
    voice_minutes INT NOT NULL DEFAULT 0,
    active_members INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(guild_id, date)
);

CREATE INDEX IF NOT EXISTS idx_guild_insights_guild_date ON guild_insights_daily(guild_id, date DESC);

-- Hourly message activity for peak hours chart
CREATE TABLE IF NOT EXISTS guild_insights_hourly (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    hour INT NOT NULL CHECK (hour >= 0 AND hour <= 23),
    messages INT NOT NULL DEFAULT 0,
    UNIQUE(guild_id, date, hour)
);

CREATE INDEX IF NOT EXISTS idx_guild_insights_hourly_guild ON guild_insights_hourly(guild_id, date DESC);

-- ============================================================
-- Server Boosts
-- ============================================================
CREATE TABLE IF NOT EXISTS guild_boosts (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tier INT NOT NULL DEFAULT 1,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    active BOOLEAN NOT NULL DEFAULT true,
    UNIQUE(guild_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_guild_boosts_guild ON guild_boosts(guild_id) WHERE active = true;
CREATE INDEX IF NOT EXISTS idx_guild_boosts_user ON guild_boosts(user_id);

-- Guild boost summary cached on the guild
ALTER TABLE guilds ADD COLUMN IF NOT EXISTS boost_count INT NOT NULL DEFAULT 0;
ALTER TABLE guilds ADD COLUMN IF NOT EXISTS boost_tier INT NOT NULL DEFAULT 0;

-- ============================================================
-- Vanity URL Marketplace (guild claims a vanity code)
-- Already have vanity_url on guilds, add a reservation table
-- ============================================================
CREATE TABLE IF NOT EXISTS vanity_url_claims (
    code TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    claimed_by TEXT NOT NULL REFERENCES users(id),
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_vanity_claims_guild ON vanity_url_claims(guild_id);

-- ============================================================
-- User Achievements / Badges
-- ============================================================
CREATE TABLE IF NOT EXISTS achievement_definitions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    icon TEXT NOT NULL DEFAULT 'star',
    category TEXT NOT NULL DEFAULT 'general',
    criteria_type TEXT NOT NULL, -- 'message_count', 'reaction_count', 'guild_join_count', 'voice_minutes', 'account_age_days', 'boost'
    criteria_threshold INT NOT NULL DEFAULT 1,
    rarity TEXT NOT NULL DEFAULT 'common', -- 'common', 'uncommon', 'rare', 'epic', 'legendary'
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_achievements (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    achievement_id TEXT NOT NULL REFERENCES achievement_definitions(id) ON DELETE CASCADE,
    earned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, achievement_id)
);

CREATE INDEX IF NOT EXISTS idx_user_achievements_user ON user_achievements(user_id);

-- Seed default achievements
INSERT INTO achievement_definitions (id, name, description, icon, category, criteria_type, criteria_threshold, rarity) VALUES
    ('achv_first_message', 'First Words', 'Send your first message', 'message-circle', 'messaging', 'message_count', 1, 'common'),
    ('achv_100_messages', 'Chatty', 'Send 100 messages', 'messages-square', 'messaging', 'message_count', 100, 'common'),
    ('achv_1000_messages', 'Talkative', 'Send 1,000 messages', 'message-circle-heart', 'messaging', 'message_count', 1000, 'uncommon'),
    ('achv_10000_messages', 'Prolific', 'Send 10,000 messages', 'book-open', 'messaging', 'message_count', 10000, 'rare'),
    ('achv_50000_messages', 'Legendary Chatter', 'Send 50,000 messages', 'crown', 'messaging', 'message_count', 50000, 'legendary'),
    ('achv_first_reaction', 'Reactor', 'Add your first reaction', 'smile', 'social', 'reaction_count', 1, 'common'),
    ('achv_500_reactions', 'Emotionally Invested', 'Add 500 reactions', 'heart', 'social', 'reaction_count', 500, 'uncommon'),
    ('achv_join_3_guilds', 'Social Butterfly', 'Join 3 servers', 'users', 'social', 'guild_join_count', 3, 'common'),
    ('achv_join_10_guilds', 'Community Leader', 'Join 10 servers', 'building', 'social', 'guild_join_count', 10, 'uncommon'),
    ('achv_voice_60', 'Voice Actor', 'Spend 60 minutes in voice', 'mic', 'voice', 'voice_minutes', 60, 'common'),
    ('achv_voice_600', 'Podcaster', 'Spend 10 hours in voice', 'headphones', 'voice', 'voice_minutes', 600, 'uncommon'),
    ('achv_voice_6000', 'Radio Star', 'Spend 100 hours in voice', 'radio', 'voice', 'voice_minutes', 6000, 'rare'),
    ('achv_account_30d', 'Veteran (30 days)', 'Account is 30 days old', 'calendar', 'tenure', 'account_age_days', 30, 'common'),
    ('achv_account_365d', 'Veteran (1 year)', 'Account is 1 year old', 'award', 'tenure', 'account_age_days', 365, 'rare'),
    ('achv_booster', 'Supporter', 'Boost a server', 'zap', 'support', 'boost', 1, 'epic')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- Leveling / XP System (per-guild)
-- ============================================================
CREATE TABLE IF NOT EXISTS guild_leveling_config (
    guild_id TEXT PRIMARY KEY REFERENCES guilds(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT false,
    xp_per_message INT NOT NULL DEFAULT 15,
    xp_cooldown_seconds INT NOT NULL DEFAULT 60,
    level_up_channel_id TEXT REFERENCES channels(id) ON DELETE SET NULL,
    level_up_message TEXT NOT NULL DEFAULT 'Congratulations {user}, you reached level {level}!',
    stack_roles BOOLEAN NOT NULL DEFAULT true,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS guild_level_roles (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    level INT NOT NULL,
    role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    UNIQUE(guild_id, level, role_id)
);

CREATE INDEX IF NOT EXISTS idx_guild_level_roles_guild ON guild_level_roles(guild_id);

CREATE TABLE IF NOT EXISTS member_xp (
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    xp BIGINT NOT NULL DEFAULT 0,
    level INT NOT NULL DEFAULT 0,
    messages_counted INT NOT NULL DEFAULT 0,
    last_xp_at TIMESTAMPTZ,
    PRIMARY KEY (guild_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_member_xp_guild_level ON member_xp(guild_id, xp DESC);

-- ============================================================
-- Starboard
-- ============================================================
CREATE TABLE IF NOT EXISTS guild_starboard_config (
    guild_id TEXT PRIMARY KEY REFERENCES guilds(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT false,
    channel_id TEXT REFERENCES channels(id) ON DELETE SET NULL,
    emoji TEXT NOT NULL DEFAULT 'star',
    threshold INT NOT NULL DEFAULT 3,
    self_star BOOLEAN NOT NULL DEFAULT false,
    nsfw_allowed BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS starboard_entries (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    source_message_id TEXT NOT NULL,
    source_channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    starboard_message_id TEXT,
    star_count INT NOT NULL DEFAULT 0,
    author_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(guild_id, source_message_id)
);

CREATE INDEX IF NOT EXISTS idx_starboard_entries_guild ON starboard_entries(guild_id, star_count DESC);

-- ============================================================
-- Welcome Message Automation
-- ============================================================
CREATE TABLE IF NOT EXISTS guild_welcome_config (
    guild_id TEXT PRIMARY KEY REFERENCES guilds(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT false,
    channel_id TEXT REFERENCES channels(id) ON DELETE SET NULL,
    message TEXT NOT NULL DEFAULT 'Welcome to the server, {user}!',
    dm_enabled BOOLEAN NOT NULL DEFAULT false,
    dm_message TEXT NOT NULL DEFAULT 'Welcome to {guild}! Please read the rules.',
    embed_enabled BOOLEAN NOT NULL DEFAULT false,
    embed_color TEXT DEFAULT '#5865F2',
    embed_title TEXT DEFAULT 'Welcome!',
    embed_image_url TEXT,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================
-- Auto-Role Assignment
-- ============================================================
CREATE TABLE IF NOT EXISTS guild_auto_roles (
    id TEXT PRIMARY KEY,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    rule_type TEXT NOT NULL DEFAULT 'on_join', -- 'on_join', 'after_delay', 'on_verify'
    delay_seconds INT NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(guild_id, role_id)
);

CREATE INDEX IF NOT EXISTS idx_guild_auto_roles_guild ON guild_auto_roles(guild_id) WHERE enabled = true;
