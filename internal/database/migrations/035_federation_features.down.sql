-- Migration 035 rollback: Remove Phase 10 Federation, Bridges, and Multi-Instance tables.

DROP TABLE IF EXISTS instance_connection_profiles;
DROP TABLE IF EXISTS bridge_virtual_users;
DROP TABLE IF EXISTS bridge_channel_mappings;
DROP TABLE IF EXISTS bridge_configs;
DROP TABLE IF EXISTS federation_search_config;
DROP TABLE IF EXISTS federation_delivery_receipts;
DROP TABLE IF EXISTS federation_peer_controls;
DROP TABLE IF EXISTS federation_peer_status;

ALTER TABLE messages DROP COLUMN IF EXISTS bridge_source;
ALTER TABLE messages DROP COLUMN IF EXISTS bridge_remote_id;
ALTER TABLE messages DROP COLUMN IF EXISTS bridge_author_name;
ALTER TABLE messages DROP COLUMN IF EXISTS bridge_author_avatar;

ALTER TABLE instances DROP COLUMN IF EXISTS protocol_version;
ALTER TABLE instances DROP COLUMN IF EXISTS capabilities;
