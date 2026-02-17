-- Remove @everyone roles seeded by this migration.
-- Only delete roles at position=0 that have no member_roles referencing them,
-- since pre-existing @everyone roles may have been assigned.
DELETE FROM roles
WHERE name = '@everyone' AND position = 0
  AND id NOT IN (SELECT DISTINCT role_id FROM member_roles);
