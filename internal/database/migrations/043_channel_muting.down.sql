-- Rollback migration 041: Remove channel-level notification preferences.

DROP TABLE IF EXISTS channel_notification_preferences;
