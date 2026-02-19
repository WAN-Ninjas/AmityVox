-- Rollback migration 059: Restore per-user channel groups.

DROP TABLE IF EXISTS guild_channel_group_items;
DROP TABLE IF EXISTS guild_channel_groups;

-- Restore original per-user tables.
CREATE TABLE user_channel_groups (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    position    INTEGER NOT NULL DEFAULT 0,
    color       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_user_channel_groups_user ON user_channel_groups(user_id, position);

CREATE TABLE user_channel_group_items (
    group_id    TEXT NOT NULL REFERENCES user_channel_groups(id) ON DELETE CASCADE,
    channel_id  TEXT NOT NULL,
    PRIMARY KEY (group_id, channel_id)
);
CREATE INDEX idx_user_channel_group_items_channel ON user_channel_group_items(channel_id);
