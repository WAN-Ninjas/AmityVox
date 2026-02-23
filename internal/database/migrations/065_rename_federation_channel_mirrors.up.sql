-- Rename federation_channel_mirrors to federation_dm_channel_map.
-- This table is only used for DM channel ID mapping between federated instances.
ALTER TABLE IF EXISTS federation_channel_mirrors RENAME TO federation_dm_channel_map;

-- Rename indexes to match the new table name.
ALTER INDEX IF EXISTS federation_channel_mirrors_pkey RENAME TO federation_dm_channel_map_pkey;
ALTER INDEX IF EXISTS idx_federation_channel_mirrors_remote RENAME TO idx_federation_dm_channel_map_remote;
