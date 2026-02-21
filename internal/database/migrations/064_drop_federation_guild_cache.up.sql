-- Drop the federation_guild_cache table.
-- Federated guilds are now stored in the real guilds table with instance_id set.
-- The cache table is no longer used.
DROP INDEX IF EXISTS idx_federation_guild_cache_user;
DROP INDEX IF EXISTS idx_federation_guild_cache_instance;
DROP INDEX IF EXISTS idx_federation_guild_cache_guild;
DROP TABLE IF EXISTS federation_guild_cache;
