-- Create an @everyone role at position 0 for every existing guild that doesn't already have one.
-- The role inherits the guild's default_permissions as permissions_allow.
INSERT INTO roles (id, guild_id, name, color, hoist, mentionable, position, permissions_allow, permissions_deny, created_at)
SELECT
    'R' || replace(gen_random_uuid()::text, '-', ''),
    id, '@everyone', NULL, false, false, 0, default_permissions, 0, now()
FROM guilds
WHERE NOT EXISTS (
    SELECT 1 FROM roles r WHERE r.guild_id = guilds.id AND r.name = '@everyone'
);
