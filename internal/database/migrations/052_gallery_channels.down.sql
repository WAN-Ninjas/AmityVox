DROP TABLE IF EXISTS gallery_post_tags;
DROP TABLE IF EXISTS gallery_tags;
ALTER TABLE channels DROP COLUMN IF EXISTS gallery_default_sort;
ALTER TABLE channels DROP COLUMN IF EXISTS gallery_post_guidelines;
ALTER TABLE channels DROP COLUMN IF EXISTS gallery_require_tags;
