-- Migration 059: Convert per-user channel groups to guild-wide channel groups.
-- Channel groups are now managed by guild admins and visible to all members.

-- Drop old per-user tables.
DROP TABLE IF EXISTS user_channel_group_items;
DROP TABLE IF EXISTS user_channel_groups;

-- Guild-wide channel groups (admin-managed).
CREATE TABLE guild_channel_groups (
    id          TEXT PRIMARY KEY,
    guild_id    TEXT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    position    INTEGER NOT NULL DEFAULT 0,
    color       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_guild_channel_groups_guild ON guild_channel_groups(guild_id, position);

-- Channels assigned to guild channel groups.
CREATE TABLE guild_channel_group_items (
    group_id    TEXT NOT NULL REFERENCES guild_channel_groups(id) ON DELETE CASCADE,
    channel_id  TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    position    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (group_id, channel_id)
);
CREATE INDEX idx_guild_channel_group_items_channel ON guild_channel_group_items(channel_id);
