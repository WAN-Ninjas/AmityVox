-- Federation security hardening: IP verification, key fingerprinting, mutual handshake

ALTER TABLE instances ADD COLUMN IF NOT EXISTS resolved_ips TEXT[];
ALTER TABLE instances ADD COLUMN IF NOT EXISTS key_fingerprint TEXT;

ALTER TABLE federation_peers ADD COLUMN IF NOT EXISTS handshake_completed_at TIMESTAMPTZ;
ALTER TABLE federation_peers ADD COLUMN IF NOT EXISTS initiated_by TEXT;

CREATE TABLE IF NOT EXISTS federation_key_audit (
    id              TEXT PRIMARY KEY,
    instance_id     TEXT NOT NULL REFERENCES instances(id),
    old_fingerprint TEXT NOT NULL,
    new_fingerprint TEXT NOT NULL,
    old_public_key  TEXT NOT NULL,
    detected_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    acknowledged_by TEXT REFERENCES users(id),
    acknowledged_at TIMESTAMPTZ
);

-- Grandfather existing active peers
UPDATE federation_peers SET handshake_completed_at = established_at
WHERE status = 'active' AND handshake_completed_at IS NULL;
