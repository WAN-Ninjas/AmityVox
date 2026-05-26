-- Feature availability controls for instance and guild administrators.

CREATE TABLE IF NOT EXISTS instance_feature_flags (
    feature_key TEXT PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT true,
    hard_disabled BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS guild_feature_flags (
    guild_id TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    feature_key TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, feature_key)
);

CREATE INDEX IF NOT EXISTS idx_guild_feature_flags_guild
    ON guild_feature_flags(guild_id);

INSERT INTO instance_feature_flags (feature_key, enabled, hard_disabled)
VALUES
    ('polls', true, false),
    ('code_snippets', true, false),
    ('voice_transcription', true, false),
    ('video_recordings', true, false),
    ('whiteboards', true, false),
    ('kanban_boards', true, false),
    ('activities', true, false),
    ('voice_broadcasts', true, false),
    ('message_effects', true, false),
    ('super_reactions', true, false),
    ('location_sharing', true, false),
    ('message_summaries', true, false),
    ('sticker_packs', true, false),
    ('custom_emoji', true, false),
    ('gif_search', true, false),
    ('message_bookmarks', true, false),
    ('scheduled_messages', true, false),
    ('expiring_messages', true, false),
    ('threads_and_replies', true, false),
    ('pins', true, false),
    ('full_text_search', true, false),
    ('federated_messaging', true, false),
    ('federated_presence', true, false),
    ('federated_attachments', true, false),
    ('federation_admin_diagnostics', true, false),
    ('webhooks', true, false),
    ('translation', true, false),
    ('e2ee', true, false),
    ('guild_onboarding', true, false),
    ('channel_groups', true, false),
    ('announcement_channels', true, false),
    ('automod', true, false),
    ('moderation_reports', true, false),
    ('audit_logs', true, false),
    ('gallery_media', true, false),
    ('admin_backups', true, false),
    ('theme_editor', true, false),
    ('widgets', true, false),
    ('pwa_push', true, false)
ON CONFLICT (feature_key) DO NOTHING;
