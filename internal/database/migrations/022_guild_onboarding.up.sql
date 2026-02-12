-- Migration 022: Guild onboarding system.
-- Allows guild owners to configure a welcome experience for new members:
-- welcome message, rules requiring acceptance, default channels, and
-- an optional questionnaire with role assignment.

CREATE TABLE IF NOT EXISTS guild_onboarding (
    guild_id        TEXT PRIMARY KEY REFERENCES guilds(id) ON DELETE CASCADE,
    enabled         BOOLEAN NOT NULL DEFAULT false,
    welcome_message TEXT,                            -- Markdown-formatted welcome text
    rules           JSONB NOT NULL DEFAULT '[]',     -- Array of rule strings members must accept
    default_channel_ids TEXT[] NOT NULL DEFAULT '{}', -- Channels new members are auto-subscribed to
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS onboarding_prompts (
    id          TEXT PRIMARY KEY,                    -- ULID
    guild_id    TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,                       -- e.g. "What are you interested in?"
    required    BOOLEAN NOT NULL DEFAULT false,
    single_select BOOLEAN NOT NULL DEFAULT false,    -- false = multi-select
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_onboarding_prompts_guild ON onboarding_prompts(guild_id, position);

CREATE TABLE IF NOT EXISTS onboarding_options (
    id          TEXT PRIMARY KEY,                    -- ULID
    prompt_id   TEXT NOT NULL REFERENCES onboarding_prompts(id) ON DELETE CASCADE,
    guild_id    TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    label       TEXT NOT NULL,                       -- Display text, e.g. "Gaming"
    description TEXT,                                -- Optional extra info
    emoji       TEXT,                                -- Optional emoji
    role_ids    TEXT[] NOT NULL DEFAULT '{}',         -- Roles assigned when this option is chosen
    channel_ids TEXT[] NOT NULL DEFAULT '{}',         -- Channels revealed when chosen
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_onboarding_options_prompt ON onboarding_options(prompt_id);

-- Track which members have completed onboarding.
CREATE TABLE IF NOT EXISTS onboarding_completions (
    guild_id    TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, user_id)
);
