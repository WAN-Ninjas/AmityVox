-- Migration 003: Message edit history tracking.

CREATE TABLE message_edits (
    id          TEXT PRIMARY KEY,
    message_id  TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    content     TEXT NOT NULL,
    edited_at   TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_message_edits_message ON message_edits(message_id, edited_at DESC);
