-- Rollback of the initial AmityVox schema.
-- Drops all tables and indexes in reverse dependency order.

DROP INDEX IF EXISTS idx_users_instance;
DROP INDEX IF EXISTS idx_webhooks_channel;
DROP INDEX IF EXISTS idx_custom_emoji_guild;
DROP INDEX IF EXISTS idx_invites_guild;
DROP INDEX IF EXISTS idx_read_state_user;
DROP INDEX IF EXISTS idx_user_sessions_expiry;
DROP INDEX IF EXISTS idx_user_sessions_user;
DROP INDEX IF EXISTS idx_audit_log_actor;
DROP INDEX IF EXISTS idx_audit_log_guild;
DROP INDEX IF EXISTS idx_embeds_message;
DROP INDEX IF EXISTS idx_reactions_message;
DROP INDEX IF EXISTS idx_attachments_message;
DROP INDEX IF EXISTS idx_member_roles_role;
DROP INDEX IF EXISTS idx_guild_members_user;
DROP INDEX IF EXISTS idx_channels_category;
DROP INDEX IF EXISTS idx_channels_guild;
DROP INDEX IF EXISTS idx_messages_nonce;
DROP INDEX IF EXISTS idx_messages_author;
DROP INDEX IF EXISTS idx_messages_channel;

DROP TABLE IF EXISTS read_state;
DROP TABLE IF EXISTS federation_peers;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS webhooks;
DROP TABLE IF EXISTS custom_emoji;
DROP TABLE IF EXISTS guild_bans;
DROP TABLE IF EXISTS invites;
DROP TABLE IF EXISTS pins;
DROP TABLE IF EXISTS reactions;
DROP TABLE IF EXISTS embeds;
DROP TABLE IF EXISTS attachments;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS channel_permission_overrides;
DROP TABLE IF EXISTS member_roles;
DROP TABLE IF EXISTS guild_members;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS channel_recipients;
DROP TABLE IF EXISTS channels;
DROP TABLE IF EXISTS guild_categories;
DROP TABLE IF EXISTS guilds;
DROP TABLE IF EXISTS webauthn_credentials;
DROP TABLE IF EXISTS user_relationships;
DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS instances;
