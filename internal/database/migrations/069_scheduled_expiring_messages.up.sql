ALTER TABLE messages ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_messages_expires_at ON messages (expires_at) WHERE expires_at IS NOT NULL;

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_message_type_check;
ALTER TABLE messages ADD CONSTRAINT messages_message_type_check CHECK (
    message_type IN (
        'default',
        'system_join',
        'system_leave',
        'system_kick',
        'system_ban',
        'system_pin',
        'reply',
        'thread_created',
        'voice',
        'poll',
        'code_snippet',
        'forward',
        'scheduled',
        'system_lockdown'
    )
);
