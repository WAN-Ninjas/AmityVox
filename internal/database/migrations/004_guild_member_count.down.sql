-- Rollback migration 004

DROP TRIGGER IF EXISTS trg_guild_member_count_dec ON guild_members;
DROP TRIGGER IF EXISTS trg_guild_member_count_inc ON guild_members;
DROP FUNCTION IF EXISTS guild_member_count_dec();
DROP FUNCTION IF EXISTS guild_member_count_inc();
ALTER TABLE guilds DROP COLUMN IF EXISTS member_count;
