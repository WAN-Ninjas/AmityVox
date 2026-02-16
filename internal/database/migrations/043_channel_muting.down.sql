-- Rollback migration 043: Remove channel-level notification preferences.

DROP TABLE IF EXISTS channel_notification_preferences;
