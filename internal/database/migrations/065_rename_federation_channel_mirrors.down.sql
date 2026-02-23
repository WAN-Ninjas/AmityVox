-- Revert: rename federation_dm_channel_map back to federation_channel_mirrors.
ALTER TABLE IF EXISTS federation_dm_channel_map RENAME TO federation_channel_mirrors;

ALTER INDEX IF EXISTS federation_dm_channel_map_pkey RENAME TO federation_channel_mirrors_pkey;
ALTER INDEX IF EXISTS idx_federation_dm_channel_map_remote RENAME TO idx_federation_channel_mirrors_remote;
