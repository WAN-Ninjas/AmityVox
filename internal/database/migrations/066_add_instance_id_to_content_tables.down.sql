-- Remove instance_id columns from content tables.
DROP INDEX IF EXISTS idx_channels_instance_id;
ALTER TABLE channels DROP COLUMN IF EXISTS instance_id;

DROP INDEX IF EXISTS idx_roles_instance_id;
ALTER TABLE roles DROP COLUMN IF EXISTS instance_id;

DROP INDEX IF EXISTS idx_guild_members_instance_id;
ALTER TABLE guild_members DROP COLUMN IF EXISTS instance_id;

DROP INDEX IF EXISTS idx_messages_instance_id;
ALTER TABLE messages DROP COLUMN IF EXISTS instance_id;

DROP INDEX IF EXISTS idx_attachments_instance_id;
ALTER TABLE attachments DROP COLUMN IF EXISTS instance_id;

DROP INDEX IF EXISTS idx_embeds_instance_id;
ALTER TABLE embeds DROP COLUMN IF EXISTS instance_id;

DROP INDEX IF EXISTS idx_reactions_instance_id;
ALTER TABLE reactions DROP COLUMN IF EXISTS instance_id;

DROP INDEX IF EXISTS idx_pins_instance_id;
ALTER TABLE pins DROP COLUMN IF EXISTS instance_id;

DROP INDEX IF EXISTS idx_webhooks_instance_id;
ALTER TABLE webhooks DROP COLUMN IF EXISTS instance_id;
