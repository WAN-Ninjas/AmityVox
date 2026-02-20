-- Add private_key_pem column to instances table for storing the local instance's
-- Ed25519 private key. Only the local instance row should have this populated.
-- Remote instances only have their public keys stored.
-- NOTE: Consider adding app-layer encryption (AES-GCM) keyed from config in a
-- future migration to protect this column at rest against DB dump exposure.
ALTER TABLE instances ADD COLUMN IF NOT EXISTS private_key_pem TEXT;
