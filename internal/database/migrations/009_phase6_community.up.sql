-- Phase 6: Community & Social Features
-- Adds polls, bookmarks, scheduled events, and custom status enhancements.

-- ============================================================
-- POLLS
-- ============================================================

CREATE TABLE polls (
    id              TEXT PRIMARY KEY,            -- ULID
    channel_id      TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    message_id      TEXT REFERENCES messages(id) ON DELETE CASCADE,
    author_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    question        TEXT NOT NULL,
    multi_vote      BOOLEAN DEFAULT false,       -- Allow multiple selections
    anonymous       BOOLEAN DEFAULT false,       -- Hide voter identities
    expires_at      TIMESTAMPTZ,                 -- NULL = no expiry
    closed          BOOLEAN DEFAULT false,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_polls_channel_id ON polls(channel_id);
CREATE INDEX idx_polls_message_id ON polls(message_id);

CREATE TABLE poll_options (
    id              TEXT PRIMARY KEY,            -- ULID
    poll_id         TEXT NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    text            TEXT NOT NULL,
    position        INTEGER NOT NULL DEFAULT 0,
    vote_count      INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_poll_options_poll_id ON poll_options(poll_id);

CREATE TABLE poll_votes (
    poll_id         TEXT NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    option_id       TEXT NOT NULL REFERENCES poll_options(id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (poll_id, option_id, user_id)
);

CREATE INDEX idx_poll_votes_user ON poll_votes(poll_id, user_id);

-- ============================================================
-- MESSAGE BOOKMARKS (Saved Messages)
-- ============================================================

CREATE TABLE message_bookmarks (
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id      TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    note            TEXT,                        -- Optional note about the bookmark
    created_at      TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (user_id, message_id)
);

CREATE INDEX idx_bookmarks_user ON message_bookmarks(user_id, created_at DESC);

-- ============================================================
-- SCHEDULED EVENTS
-- ============================================================

CREATE TABLE guild_events (
    id              TEXT PRIMARY KEY,            -- ULID
    guild_id        TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    creator_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT,
    location        TEXT,                        -- External location or channel reference
    channel_id      TEXT REFERENCES channels(id) ON DELETE SET NULL,
    image_id        TEXT,                        -- Attachment ID for cover image
    scheduled_start TIMESTAMPTZ NOT NULL,
    scheduled_end   TIMESTAMPTZ,
    status          TEXT DEFAULT 'scheduled'
        CHECK (status IN ('scheduled', 'active', 'completed', 'cancelled')),
    interested_count INTEGER DEFAULT 0,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_guild_events_guild ON guild_events(guild_id, scheduled_start);
CREATE INDEX idx_guild_events_status ON guild_events(status, scheduled_start);

CREATE TABLE event_rsvps (
    event_id        TEXT NOT NULL REFERENCES guild_events(id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status          TEXT DEFAULT 'interested'
        CHECK (status IN ('interested', 'going')),
    created_at      TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (event_id, user_id)
);

CREATE INDEX idx_event_rsvps_user ON event_rsvps(user_id);

-- ============================================================
-- CUSTOM STATUS ENHANCEMENTS
-- ============================================================

-- Add status_emoji and status_expires_at to users.
ALTER TABLE users ADD COLUMN IF NOT EXISTS status_emoji TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS status_expires_at TIMESTAMPTZ;

-- Add profile customization fields.
ALTER TABLE users ADD COLUMN IF NOT EXISTS banner_id TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS accent_color TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS pronouns TEXT;
