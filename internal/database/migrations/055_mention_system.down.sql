-- Revert: rename mention_here back to mention_everyone.
ALTER TABLE messages RENAME COLUMN mention_here TO mention_everyone;
ALTER TABLE notification_preferences RENAME COLUMN suppress_here TO suppress_everyone;
