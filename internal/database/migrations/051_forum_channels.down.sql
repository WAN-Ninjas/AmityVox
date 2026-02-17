ALTER TABLE channels DROP COLUMN IF EXISTS reply_count;
ALTER TABLE channels DROP COLUMN IF EXISTS pinned;
ALTER TABLE channels DROP COLUMN IF EXISTS forum_require_tags;
ALTER TABLE channels DROP COLUMN IF EXISTS forum_post_guidelines;
ALTER TABLE channels DROP COLUMN IF EXISTS forum_default_sort;

DROP TABLE IF EXISTS forum_post_tags;
DROP TABLE IF EXISTS forum_tags;
