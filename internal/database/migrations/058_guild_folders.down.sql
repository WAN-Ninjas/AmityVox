ALTER TABLE user_guild_positions DROP COLUMN IF EXISTS folder_position;
ALTER TABLE user_guild_positions DROP COLUMN IF EXISTS folder_id;
DROP TABLE IF EXISTS guild_folders;
