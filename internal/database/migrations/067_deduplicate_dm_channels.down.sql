DROP TRIGGER IF EXISTS trg_check_dm_uniqueness ON channel_recipients;
DROP FUNCTION IF EXISTS check_dm_uniqueness();
-- Note: merged messages cannot be un-merged. Deleted duplicate channels are gone.
