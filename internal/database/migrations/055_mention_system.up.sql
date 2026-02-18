-- Rename mention_everyone to mention_here (replacing @everyone with @here semantics).
ALTER TABLE messages RENAME COLUMN mention_everyone TO mention_here;
ALTER TABLE notification_preferences RENAME COLUMN suppress_everyone TO suppress_here;
