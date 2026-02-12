-- Rollback migration 036: Extensibility features

DROP TABLE IF EXISTS key_backup_recovery_codes;
DROP TABLE IF EXISTS key_backups;
DROP TABLE IF EXISTS plugin_execution_log;
DROP TABLE IF EXISTS guild_plugins;
DROP TABLE IF EXISTS plugins;
DROP TABLE IF EXISTS widget_permissions;
DROP TABLE IF EXISTS channel_widgets;
DROP TABLE IF EXISTS guild_widgets;
