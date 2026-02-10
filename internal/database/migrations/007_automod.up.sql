-- Migration 007: AutoMod — per-guild configurable content moderation rules.
-- AutoMod processes messages server-side before delivery. Encrypted channels
-- are exempt since the server cannot read their content.

CREATE TABLE automod_rules (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    guild_id    TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    enabled     BOOLEAN DEFAULT true,

    -- Rule type: word_filter, regex_filter, invite_filter, mention_spam,
    --            caps_filter, spam_filter, link_filter
    rule_type   TEXT NOT NULL CHECK (rule_type IN (
        'word_filter', 'regex_filter', 'invite_filter',
        'mention_spam', 'caps_filter', 'spam_filter', 'link_filter'
    )),

    -- JSON configuration blob. Contents depend on rule_type:
    --   word_filter:    {"words": ["bad","words"], "match_whole_word": true}
    --   regex_filter:   {"patterns": ["regex1","regex2"]}
    --   invite_filter:  {"allow_own_guild": true}
    --   mention_spam:   {"max_mentions": 5}
    --   caps_filter:    {"max_caps_percent": 70, "min_length": 10}
    --   spam_filter:    {"max_messages": 5, "window_seconds": 5, "max_duplicates": 3}
    --   link_filter:    {"allowed_domains": [], "blocked_domains": []}
    config      JSONB NOT NULL DEFAULT '{}',

    -- Action: delete, warn, timeout, log
    action      TEXT NOT NULL DEFAULT 'delete' CHECK (action IN ('delete','warn','timeout','log')),

    -- Duration for timeout action (seconds). Ignored for other actions.
    timeout_duration_seconds INTEGER DEFAULT 60,

    -- Channels/roles exempt from this rule.
    exempt_channel_ids TEXT[] DEFAULT '{}',
    exempt_role_ids    TEXT[] DEFAULT '{}',

    created_by  TEXT REFERENCES users(id),
    created_at  TIMESTAMPTZ DEFAULT now(),
    updated_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_automod_rules_guild ON automod_rules(guild_id);
CREATE INDEX idx_automod_rules_enabled ON automod_rules(guild_id, enabled) WHERE enabled = true;

-- Audit log for automod actions taken.
CREATE TABLE automod_actions (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    guild_id    TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    rule_id     TEXT NOT NULL REFERENCES automod_rules(id) ON DELETE CASCADE,
    channel_id  TEXT NOT NULL,
    message_id  TEXT,
    user_id     TEXT NOT NULL,
    action      TEXT NOT NULL,
    reason      TEXT,
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_automod_actions_guild ON automod_actions(guild_id);
CREATE INDEX idx_automod_actions_user ON automod_actions(user_id);
