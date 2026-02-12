-- Rollback migration 038: Social & Growth features

DROP TABLE IF EXISTS guild_auto_roles;
DROP TABLE IF EXISTS guild_welcome_config;
DROP TABLE IF EXISTS starboard_entries;
DROP TABLE IF EXISTS guild_starboard_config;
DROP TABLE IF EXISTS member_xp;
DROP TABLE IF EXISTS guild_level_roles;
DROP TABLE IF EXISTS guild_leveling_config;
DROP TABLE IF EXISTS user_achievements;
DROP TABLE IF EXISTS achievement_definitions;
DROP TABLE IF EXISTS vanity_url_claims;
DROP TABLE IF EXISTS guild_boosts;
DROP TABLE IF EXISTS guild_insights_hourly;
DROP TABLE IF EXISTS guild_insights_daily;

ALTER TABLE guilds DROP COLUMN IF EXISTS boost_count;
ALTER TABLE guilds DROP COLUMN IF EXISTS boost_tier;
