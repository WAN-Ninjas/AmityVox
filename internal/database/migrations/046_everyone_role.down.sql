-- Remove @everyone roles created by this migration.
DELETE FROM roles WHERE name = '@everyone' AND position = 0;
