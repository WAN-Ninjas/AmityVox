-- Migration 039 rollback: Drop interoperability tables

DROP TABLE IF EXISTS integration_message_log;
DROP TABLE IF EXISTS bridge_connections;
DROP TABLE IF EXISTS activitypub_follows;
DROP TABLE IF EXISTS guild_integrations;
