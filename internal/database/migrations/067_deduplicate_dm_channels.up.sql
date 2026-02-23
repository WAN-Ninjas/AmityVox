-- Deduplicate DM channels: merge messages from newer duplicates into the oldest,
-- then delete the duplicate channels. Also add trigger-based enforcement to prevent
-- future duplicates at the database level.

-- Step 1: Move messages from duplicate DM channels to the oldest one.
-- For each pair of users with multiple DMs, keep the oldest channel (MIN id).
WITH dm_pairs AS (
  SELECT cr1.user_id AS u1, cr2.user_id AS u2,
         MIN(cr1.channel_id) AS keep_id,
         array_agg(DISTINCT cr1.channel_id) AS all_ids
  FROM channel_recipients cr1
  JOIN channel_recipients cr2
    ON cr1.channel_id = cr2.channel_id AND cr1.user_id < cr2.user_id
  JOIN channels c ON c.id = cr1.channel_id
  WHERE c.channel_type = 'dm'
  GROUP BY cr1.user_id, cr2.user_id
  HAVING COUNT(DISTINCT cr1.channel_id) > 1
),
dupe_channels AS (
  SELECT keep_id, unnest(all_ids) AS channel_id
  FROM dm_pairs
)
UPDATE messages m SET channel_id = dc.keep_id
FROM dupe_channels dc
WHERE m.channel_id = dc.channel_id AND dc.channel_id <> dc.keep_id;

-- Step 1b: Refresh last_message_id for DM channels after merge.
WITH latest AS (
  SELECT DISTINCT ON (m.channel_id) m.channel_id, m.id AS last_id
  FROM messages m
  JOIN channels c ON c.id = m.channel_id
  WHERE c.channel_type = 'dm'
  ORDER BY m.channel_id, m.created_at DESC
)
UPDATE channels c SET last_message_id = l.last_id
FROM latest l
WHERE c.id = l.channel_id AND (c.last_message_id IS DISTINCT FROM l.last_id);

-- Step 2: Delete duplicate DM channels (now empty of messages).
WITH dm_pairs AS (
  SELECT cr1.user_id AS u1, cr2.user_id AS u2,
         MIN(cr1.channel_id) AS keep_id,
         array_agg(DISTINCT cr1.channel_id) AS all_ids
  FROM channel_recipients cr1
  JOIN channel_recipients cr2
    ON cr1.channel_id = cr2.channel_id AND cr1.user_id < cr2.user_id
  JOIN channels c ON c.id = cr1.channel_id
  WHERE c.channel_type = 'dm'
  GROUP BY cr1.user_id, cr2.user_id
  HAVING COUNT(DISTINCT cr1.channel_id) > 1
),
dupe_channels AS (
  SELECT unnest(all_ids) AS channel_id, keep_id
  FROM dm_pairs
)
DELETE FROM channels
WHERE id IN (SELECT channel_id FROM dupe_channels WHERE channel_id <> keep_id);

-- Step 3: Trigger-based uniqueness enforcement for DM channels.
-- A unique index isn't possible since the user pair spans two rows in
-- channel_recipients. The trigger uses an advisory lock on the sorted user pair
-- hash to serialize concurrent DM creation for the same pair.
CREATE OR REPLACE FUNCTION check_dm_uniqueness() RETURNS TRIGGER AS $$
DECLARE
  other_user TEXT;
  existing_id TEXT;
  pair_hash BIGINT;
BEGIN
  -- Only applies to DM channels (2 recipients).
  IF (SELECT channel_type FROM channels WHERE id = NEW.channel_id) <> 'dm' THEN
    RETURN NEW;
  END IF;

  -- Find the other recipient already inserted for this channel.
  SELECT user_id INTO other_user
  FROM channel_recipients
  WHERE channel_id = NEW.channel_id AND user_id <> NEW.user_id;

  -- If this is the first recipient, allow it (we need both to check).
  IF other_user IS NULL THEN
    RETURN NEW;
  END IF;

  -- Advisory lock on the sorted pair to serialize concurrent inserts.
  pair_hash := hashtext(LEAST(NEW.user_id, other_user) || ':' || GREATEST(NEW.user_id, other_user));
  PERFORM pg_advisory_xact_lock(pair_hash);

  -- Check if another DM channel already exists for this pair.
  SELECT c.id INTO existing_id
  FROM channels c
  JOIN channel_recipients cr1 ON c.id = cr1.channel_id AND cr1.user_id = NEW.user_id
  JOIN channel_recipients cr2 ON c.id = cr2.channel_id AND cr2.user_id = other_user
  WHERE c.channel_type = 'dm' AND c.id <> NEW.channel_id
  LIMIT 1;

  IF existing_id IS NOT NULL THEN
    RAISE EXCEPTION 'Duplicate DM channel: existing channel % already exists between users % and %',
      existing_id, NEW.user_id, other_user;
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_check_dm_uniqueness
  BEFORE INSERT ON channel_recipients
  FOR EACH ROW
  EXECUTE FUNCTION check_dm_uniqueness();
