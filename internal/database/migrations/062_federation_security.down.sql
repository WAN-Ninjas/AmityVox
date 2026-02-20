DROP TABLE IF EXISTS federation_key_audit;
ALTER TABLE federation_peers DROP COLUMN IF EXISTS handshake_completed_at;
ALTER TABLE federation_peers DROP COLUMN IF EXISTS initiated_by;
ALTER TABLE instances DROP COLUMN IF EXISTS resolved_ips;
ALTER TABLE instances DROP COLUMN IF EXISTS key_fingerprint;
