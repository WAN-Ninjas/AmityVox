-- Migration 034: Phase 7 Voice/Media Features
-- Adds soundboard sounds, voice broadcasts, voice preferences, and screen share config.

-- ============================================================
-- VOICE PREFERENCES (PTT, VAD, priority speaker settings)
-- ============================================================

CREATE TABLE IF NOT EXISTS voice_preferences (
    user_id             TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    input_mode          TEXT NOT NULL DEFAULT 'vad',         -- 'vad' or 'ptt'
    ptt_key             TEXT NOT NULL DEFAULT 'Space',       -- keybind for push-to-talk
    vad_threshold       REAL NOT NULL DEFAULT 0.3,           -- 0.0-1.0 sensitivity
    noise_suppression   BOOLEAN NOT NULL DEFAULT true,
    echo_cancellation   BOOLEAN NOT NULL DEFAULT true,
    auto_gain_control   BOOLEAN NOT NULL DEFAULT true,
    input_volume        REAL NOT NULL DEFAULT 1.0,           -- 0.0-2.0
    output_volume       REAL NOT NULL DEFAULT 1.0,           -- 0.0-2.0
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- SOUNDBOARD SOUNDS (per-guild sound clips)
-- ============================================================

CREATE TABLE IF NOT EXISTS soundboard_sounds (
    id              TEXT PRIMARY KEY,                        -- ULID
    guild_id        TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    file_url        TEXT NOT NULL,                           -- S3 key for audio file
    volume          REAL NOT NULL DEFAULT 1.0,               -- 0.0-2.0
    duration_ms     INTEGER NOT NULL,                        -- max 5000ms
    emoji           TEXT,                                    -- optional emoji icon
    creator_id      TEXT NOT NULL REFERENCES users(id),
    play_count      BIGINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_soundboard_sounds_guild ON soundboard_sounds(guild_id);
CREATE INDEX idx_soundboard_sounds_creator ON soundboard_sounds(creator_id);
CREATE UNIQUE INDEX idx_soundboard_sounds_name_guild ON soundboard_sounds(guild_id, name);

-- ============================================================
-- SOUNDBOARD CONFIG (per-guild settings)
-- ============================================================

CREATE TABLE IF NOT EXISTS soundboard_config (
    guild_id            TEXT PRIMARY KEY REFERENCES guilds(id) ON DELETE CASCADE,
    enabled             BOOLEAN NOT NULL DEFAULT true,
    max_sounds          INTEGER NOT NULL DEFAULT 8,          -- configurable by admin
    cooldown_seconds    INTEGER NOT NULL DEFAULT 5,          -- spam prevention
    allow_external      BOOLEAN NOT NULL DEFAULT false,      -- allow sounds from other guilds
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- SOUNDBOARD PLAY LOG (cooldown tracking + analytics)
-- ============================================================

CREATE TABLE IF NOT EXISTS soundboard_play_log (
    id          TEXT PRIMARY KEY,                            -- ULID
    sound_id    TEXT NOT NULL REFERENCES soundboard_sounds(id) ON DELETE CASCADE,
    guild_id    TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    channel_id  TEXT NOT NULL,
    user_id     TEXT NOT NULL REFERENCES users(id),
    played_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_soundboard_play_log_user_time ON soundboard_play_log(user_id, played_at DESC);
CREATE INDEX idx_soundboard_play_log_guild ON soundboard_play_log(guild_id, played_at DESC);

-- ============================================================
-- VOICE BROADCASTS (one-way live audio)
-- ============================================================

CREATE TABLE IF NOT EXISTS voice_broadcasts (
    id              TEXT PRIMARY KEY,                        -- ULID
    guild_id        TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    channel_id      TEXT NOT NULL,
    broadcaster_id  TEXT NOT NULL REFERENCES users(id),
    title           TEXT NOT NULL DEFAULT '',
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at        TIMESTAMPTZ,
    listener_count  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_voice_broadcasts_channel ON voice_broadcasts(channel_id);
CREATE INDEX idx_voice_broadcasts_active ON voice_broadcasts(channel_id) WHERE ended_at IS NULL;

-- ============================================================
-- SCREEN SHARE CONFIG (per-session settings)
-- ============================================================

CREATE TABLE IF NOT EXISTS screen_share_sessions (
    id              TEXT PRIMARY KEY,                        -- ULID
    channel_id      TEXT NOT NULL,
    user_id         TEXT NOT NULL REFERENCES users(id),
    share_type      TEXT NOT NULL DEFAULT 'screen',          -- 'screen' or 'window'
    resolution      TEXT NOT NULL DEFAULT '1080p',           -- '720p', '1080p', '4k'
    framerate       INTEGER NOT NULL DEFAULT 30,             -- 15, 30, or 60
    audio_enabled   BOOLEAN NOT NULL DEFAULT false,
    max_viewers     INTEGER NOT NULL DEFAULT 50,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at        TIMESTAMPTZ
);

CREATE INDEX idx_screen_share_sessions_channel ON screen_share_sessions(channel_id);
CREATE INDEX idx_screen_share_sessions_active ON screen_share_sessions(channel_id) WHERE ended_at IS NULL;
