-- Migration 037: Experimental features and Activities/Games framework

-- ============================================================
-- Location Sharing
-- ============================================================
CREATE TABLE IF NOT EXISTS location_shares (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    message_id TEXT REFERENCES messages(id) ON DELETE SET NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    accuracy DOUBLE PRECISION,             -- meters
    altitude DOUBLE PRECISION,
    label TEXT,                             -- optional place name
    live BOOLEAN NOT NULL DEFAULT false,    -- true = continuously updating
    expires_at TIMESTAMPTZ,                -- for live sharing: when to stop
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_location_shares_channel ON location_shares(channel_id);
CREATE INDEX IF NOT EXISTS idx_location_shares_user ON location_shares(user_id);
CREATE INDEX IF NOT EXISTS idx_location_shares_live ON location_shares(user_id, channel_id) WHERE live = true;

-- ============================================================
-- Message Effects (confetti, fireworks, super reactions, etc.)
-- ============================================================
CREATE TABLE IF NOT EXISTS message_effects (
    id TEXT PRIMARY KEY,
    message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    effect_type TEXT NOT NULL,   -- 'confetti', 'fireworks', 'hearts', 'snow', 'spotlight'
    config JSONB NOT NULL DEFAULT '{}',  -- intensity, color, duration etc.
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_message_effects_message ON message_effects(message_id);
CREATE INDEX IF NOT EXISTS idx_message_effects_channel ON message_effects(channel_id);

-- Super reactions (animated reactions with particle effects)
CREATE TABLE IF NOT EXISTS super_reactions (
    id TEXT PRIMARY KEY,
    message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji TEXT NOT NULL,
    intensity INT NOT NULL DEFAULT 1,   -- 1-5 scale, higher = more particles
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(message_id, user_id, emoji)
);

CREATE INDEX IF NOT EXISTS idx_super_reactions_message ON super_reactions(message_id);

-- ============================================================
-- Message Summaries (AI-powered channel summarization)
-- ============================================================
CREATE TABLE IF NOT EXISTS message_summaries (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    requested_by TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    summary TEXT NOT NULL,
    message_count INT NOT NULL,
    from_message_id TEXT,            -- oldest message in range
    to_message_id TEXT,              -- newest message in range
    model TEXT NOT NULL DEFAULT 'local',  -- AI model used
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_message_summaries_channel ON message_summaries(channel_id);

-- ============================================================
-- Voice Transcriptions (speech-to-text)
-- ============================================================
CREATE TABLE IF NOT EXISTS voice_transcriptions (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    confidence DOUBLE PRECISION,     -- 0.0-1.0 transcription confidence
    language TEXT DEFAULT 'en',
    duration_ms INT NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_voice_transcriptions_channel ON voice_transcriptions(channel_id);
CREATE INDEX IF NOT EXISTS idx_voice_transcriptions_user ON voice_transcriptions(user_id);

-- Voice channel transcription opt-in settings
CREATE TABLE IF NOT EXISTS voice_transcription_settings (
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT false,
    language TEXT NOT NULL DEFAULT 'en',
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (channel_id, user_id)
);

-- ============================================================
-- Collaborative Whiteboards
-- ============================================================
CREATE TABLE IF NOT EXISTS whiteboards (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    guild_id TEXT REFERENCES guilds(id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT 'Untitled Whiteboard',
    creator_id TEXT NOT NULL REFERENCES users(id),
    state JSONB NOT NULL DEFAULT '{"objects": [], "version": 0}',
    width INT NOT NULL DEFAULT 1920,
    height INT NOT NULL DEFAULT 1080,
    background_color TEXT DEFAULT '#ffffff',
    locked BOOLEAN NOT NULL DEFAULT false,
    max_collaborators INT NOT NULL DEFAULT 20,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_whiteboards_channel ON whiteboards(channel_id);

-- Whiteboard collaborator tracking
CREATE TABLE IF NOT EXISTS whiteboard_collaborators (
    whiteboard_id TEXT NOT NULL REFERENCES whiteboards(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'editor',  -- 'viewer', 'editor', 'admin'
    cursor_x DOUBLE PRECISION DEFAULT 0,
    cursor_y DOUBLE PRECISION DEFAULT 0,
    last_active_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (whiteboard_id, user_id)
);

-- ============================================================
-- Code Snippets (syntax-highlighted, runnable code sharing)
-- ============================================================
CREATE TABLE IF NOT EXISTS code_snippets (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    message_id TEXT REFERENCES messages(id) ON DELETE SET NULL,
    author_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT,
    language TEXT NOT NULL DEFAULT 'plaintext',
    code TEXT NOT NULL,
    stdin TEXT,                                -- optional input for execution
    output TEXT,                               -- cached execution output
    output_error TEXT,                         -- stderr output
    exit_code INT,
    runtime_ms INT,                            -- execution duration
    runnable BOOLEAN NOT NULL DEFAULT false,   -- whether server-side execution is available
    public BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_code_snippets_channel ON code_snippets(channel_id);
CREATE INDEX IF NOT EXISTS idx_code_snippets_author ON code_snippets(author_id);

-- ============================================================
-- Video Recordings (in-app screen/camera recording)
-- ============================================================
CREATE TABLE IF NOT EXISTS video_recordings (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id TEXT REFERENCES messages(id) ON DELETE SET NULL,
    title TEXT,
    s3_key TEXT NOT NULL,
    s3_bucket TEXT NOT NULL,
    duration_ms INT NOT NULL DEFAULT 0,
    file_size_bytes BIGINT NOT NULL DEFAULT 0,
    width INT,
    height INT,
    thumbnail_s3_key TEXT,
    status TEXT NOT NULL DEFAULT 'processing',  -- 'processing', 'ready', 'failed'
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_recordings_channel ON video_recordings(channel_id);
CREATE INDEX IF NOT EXISTS idx_video_recordings_user ON video_recordings(user_id);

-- ============================================================
-- Kanban Boards (project management channel type)
-- ============================================================
CREATE TABLE IF NOT EXISTS kanban_boards (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT 'Project Board',
    description TEXT,
    creator_id TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kanban_boards_channel ON kanban_boards(channel_id);

CREATE TABLE IF NOT EXISTS kanban_columns (
    id TEXT PRIMARY KEY,
    board_id TEXT NOT NULL REFERENCES kanban_boards(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    color TEXT DEFAULT '#6366f1',
    position INT NOT NULL DEFAULT 0,
    wip_limit INT,     -- work-in-progress limit (null = unlimited)
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kanban_columns_board ON kanban_columns(board_id);

CREATE TABLE IF NOT EXISTS kanban_cards (
    id TEXT PRIMARY KEY,
    column_id TEXT NOT NULL REFERENCES kanban_columns(id) ON DELETE CASCADE,
    board_id TEXT NOT NULL REFERENCES kanban_boards(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT,
    color TEXT,
    position INT NOT NULL DEFAULT 0,
    assignee_ids TEXT[] DEFAULT '{}',
    label_ids TEXT[] DEFAULT '{}',
    due_date TIMESTAMPTZ,
    completed BOOLEAN NOT NULL DEFAULT false,
    creator_id TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kanban_cards_column ON kanban_cards(column_id);
CREATE INDEX IF NOT EXISTS idx_kanban_cards_board ON kanban_cards(board_id);

CREATE TABLE IF NOT EXISTS kanban_labels (
    id TEXT PRIMARY KEY,
    board_id TEXT NOT NULL REFERENCES kanban_boards(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    color TEXT NOT NULL DEFAULT '#6366f1',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kanban_labels_board ON kanban_labels(board_id);

-- ============================================================
-- Activities Framework (embedded iframe activities)
-- ============================================================
CREATE TABLE IF NOT EXISTS activities (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    activity_type TEXT NOT NULL,   -- 'watch_together', 'music_party', 'game', 'custom'
    icon_url TEXT,
    url TEXT NOT NULL,             -- iframe src URL
    developer_id TEXT REFERENCES users(id),
    sdk_version TEXT DEFAULT '1.0',
    max_participants INT DEFAULT 0,    -- 0 = unlimited
    min_participants INT DEFAULT 1,
    category TEXT NOT NULL DEFAULT 'other',  -- 'entertainment', 'games', 'productivity', 'other'
    public BOOLEAN NOT NULL DEFAULT true,
    verified BOOLEAN NOT NULL DEFAULT false,
    supported_channel_types TEXT[] DEFAULT '{voice,text}',
    config_schema JSONB DEFAULT '{}',     -- JSON schema for activity config
    permissions_required TEXT[] DEFAULT '{}',
    install_count INT NOT NULL DEFAULT 0,
    rating_sum INT NOT NULL DEFAULT 0,
    rating_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_activities_type ON activities(activity_type);
CREATE INDEX IF NOT EXISTS idx_activities_category ON activities(category);
CREATE INDEX IF NOT EXISTS idx_activities_public ON activities(public) WHERE public = true;

-- Active activity sessions in channels
CREATE TABLE IF NOT EXISTS activity_sessions (
    id TEXT PRIMARY KEY,
    activity_id TEXT NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    guild_id TEXT REFERENCES guilds(id) ON DELETE CASCADE,
    host_user_id TEXT NOT NULL REFERENCES users(id),
    state JSONB NOT NULL DEFAULT '{}',          -- shared state synced to all participants
    config JSONB NOT NULL DEFAULT '{}',         -- session config (e.g. video URL, game settings)
    status TEXT NOT NULL DEFAULT 'active',       -- 'active', 'paused', 'ended'
    max_participants INT DEFAULT 0,
    started_at TIMESTAMPTZ DEFAULT NOW(),
    ended_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_activity_sessions_channel ON activity_sessions(channel_id);
CREATE INDEX IF NOT EXISTS idx_activity_sessions_active ON activity_sessions(status) WHERE status = 'active';

-- Participants in an activity session
CREATE TABLE IF NOT EXISTS activity_participants (
    session_id TEXT NOT NULL REFERENCES activity_sessions(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'participant',  -- 'host', 'participant', 'spectator'
    joined_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (session_id, user_id)
);

-- Activity ratings/reviews
CREATE TABLE IF NOT EXISTS activity_ratings (
    activity_id TEXT NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating INT NOT NULL CHECK (rating >= 1 AND rating <= 5),
    review TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (activity_id, user_id)
);

-- ============================================================
-- Mini-Games State (Trivia, TicTacToe, Chess, Drawing)
-- ============================================================
CREATE TABLE IF NOT EXISTS game_sessions (
    id TEXT PRIMARY KEY,
    activity_session_id TEXT REFERENCES activity_sessions(id) ON DELETE CASCADE,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    game_type TEXT NOT NULL,   -- 'trivia', 'tictactoe', 'chess', 'drawing'
    state JSONB NOT NULL DEFAULT '{}',    -- game-specific state
    config JSONB NOT NULL DEFAULT '{}',   -- game settings (difficulty, time limit, etc.)
    status TEXT NOT NULL DEFAULT 'waiting',  -- 'waiting', 'playing', 'finished'
    winner_user_id TEXT REFERENCES users(id),
    turn_user_id TEXT REFERENCES users(id),
    started_at TIMESTAMPTZ DEFAULT NOW(),
    ended_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_game_sessions_channel ON game_sessions(channel_id);
CREATE INDEX IF NOT EXISTS idx_game_sessions_type ON game_sessions(game_type);

CREATE TABLE IF NOT EXISTS game_players (
    session_id TEXT NOT NULL REFERENCES game_sessions(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    player_index INT NOT NULL DEFAULT 0,     -- player order/seat number
    score INT NOT NULL DEFAULT 0,
    player_state JSONB NOT NULL DEFAULT '{}',  -- per-player data (hand, pieces, etc.)
    joined_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (session_id, user_id)
);

-- Game leaderboard (aggregated stats)
CREATE TABLE IF NOT EXISTS game_leaderboard (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    game_type TEXT NOT NULL,
    wins INT NOT NULL DEFAULT 0,
    losses INT NOT NULL DEFAULT 0,
    draws INT NOT NULL DEFAULT 0,
    total_score BIGINT NOT NULL DEFAULT 0,
    games_played INT NOT NULL DEFAULT 0,
    last_played_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, game_type)
);

CREATE INDEX IF NOT EXISTS idx_game_leaderboard_type ON game_leaderboard(game_type, total_score DESC);

-- ============================================================
-- Built-in Activities: Watch Together state
-- ============================================================
CREATE TABLE IF NOT EXISTS watch_together_sessions (
    id TEXT PRIMARY KEY,
    activity_session_id TEXT NOT NULL REFERENCES activity_sessions(id) ON DELETE CASCADE,
    video_url TEXT NOT NULL,
    video_title TEXT,
    video_thumbnail TEXT,
    current_time_ms BIGINT NOT NULL DEFAULT 0,
    playing BOOLEAN NOT NULL DEFAULT true,
    playback_rate DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    queue JSONB NOT NULL DEFAULT '[]',       -- [{url, title, thumbnail, added_by}]
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================
-- Built-in Activities: Music Listening Party state
-- ============================================================
CREATE TABLE IF NOT EXISTS music_party_sessions (
    id TEXT PRIMARY KEY,
    activity_session_id TEXT NOT NULL REFERENCES activity_sessions(id) ON DELETE CASCADE,
    current_track_url TEXT,
    current_track_title TEXT,
    current_track_artist TEXT,
    current_track_artwork TEXT,
    current_time_ms BIGINT NOT NULL DEFAULT 0,
    playing BOOLEAN NOT NULL DEFAULT true,
    queue JSONB NOT NULL DEFAULT '[]',      -- [{url, title, artist, artwork, added_by}]
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
