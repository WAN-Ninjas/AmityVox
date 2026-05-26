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

DROP INDEX IF EXISTS idx_messages_expires_at;
ALTER TABLE messages DROP COLUMN IF EXISTS expires_at;
