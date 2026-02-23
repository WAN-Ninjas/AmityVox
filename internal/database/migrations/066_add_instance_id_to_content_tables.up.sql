-- Add instance_id to content tables for federation full parity.
-- NULL = local data, populated = federated data from that instance.
-- Enables cascade delete via DELETE FROM instances and scoped queries.

ALTER TABLE channels ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_channels_instance_id ON channels(instance_id) WHERE instance_id IS NOT NULL;

ALTER TABLE roles ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_roles_instance_id ON roles(instance_id) WHERE instance_id IS NOT NULL;

ALTER TABLE guild_members ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_guild_members_instance_id ON guild_members(instance_id) WHERE instance_id IS NOT NULL;

ALTER TABLE messages ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_messages_instance_id ON messages(instance_id) WHERE instance_id IS NOT NULL;

ALTER TABLE attachments ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_attachments_instance_id ON attachments(instance_id) WHERE instance_id IS NOT NULL;

ALTER TABLE embeds ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_embeds_instance_id ON embeds(instance_id) WHERE instance_id IS NOT NULL;

ALTER TABLE reactions ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_reactions_instance_id ON reactions(instance_id) WHERE instance_id IS NOT NULL;

ALTER TABLE pins ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_pins_instance_id ON pins(instance_id) WHERE instance_id IS NOT NULL;

ALTER TABLE webhooks ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_webhooks_instance_id ON webhooks(instance_id) WHERE instance_id IS NOT NULL;

-- Backfill instance_id from parent guild for existing federated data.
-- Channels inherit from their guild.
UPDATE channels c SET instance_id = g.instance_id
FROM guilds g WHERE c.guild_id = g.id AND g.instance_id IS NOT NULL AND c.instance_id IS NULL;

-- Roles inherit from their guild.
UPDATE roles r SET instance_id = g.instance_id
FROM guilds g WHERE r.guild_id = g.id AND g.instance_id IS NOT NULL AND r.instance_id IS NULL;

-- Guild members inherit from their guild.
UPDATE guild_members gm SET instance_id = g.instance_id
FROM guilds g WHERE gm.guild_id = g.id AND g.instance_id IS NOT NULL AND gm.instance_id IS NULL;

-- Messages inherit from their channel's guild.
UPDATE messages m SET instance_id = g.instance_id
FROM channels c JOIN guilds g ON c.guild_id = g.id
WHERE m.channel_id = c.id AND g.instance_id IS NOT NULL AND m.instance_id IS NULL;

-- Attachments inherit from their message.
UPDATE attachments a SET instance_id = m.instance_id
FROM messages m WHERE a.message_id = m.id AND m.instance_id IS NOT NULL AND a.instance_id IS NULL;

-- Embeds inherit from their message.
UPDATE embeds e SET instance_id = m.instance_id
FROM messages m WHERE e.message_id = m.id AND m.instance_id IS NOT NULL AND e.instance_id IS NULL;

-- Reactions inherit from their message.
UPDATE reactions r SET instance_id = m.instance_id
FROM messages m WHERE r.message_id = m.id AND m.instance_id IS NOT NULL AND r.instance_id IS NULL;

-- Pins inherit from their channel's guild.
UPDATE pins p SET instance_id = g.instance_id
FROM channels c JOIN guilds g ON c.guild_id = g.id
WHERE p.channel_id = c.id AND g.instance_id IS NOT NULL AND p.instance_id IS NULL;

-- Webhooks inherit from their guild.
UPDATE webhooks w SET instance_id = g.instance_id
FROM guilds g WHERE w.guild_id = g.id AND g.instance_id IS NOT NULL AND w.instance_id IS NULL;
