-- Webhook execution logs for tracking incoming and outgoing webhook deliveries.
CREATE TABLE IF NOT EXISTS webhook_execution_logs (
    id          TEXT PRIMARY KEY,
    webhook_id  TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    status_code INTEGER NOT NULL DEFAULT 0,
    request_body TEXT,
    response_preview TEXT,
    success     BOOLEAN NOT NULL DEFAULT false,
    error_message TEXT,
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_webhook_execution_logs_webhook_id
    ON webhook_execution_logs(webhook_id, created_at DESC);

-- Add outgoing_events column to webhooks table for outgoing webhook event filtering.
ALTER TABLE webhooks ADD COLUMN IF NOT EXISTS outgoing_events TEXT[] DEFAULT '{}';
