-- Add private_key_pem column to instances table for storing the local instance's
-- Ed25519 private key. Only the local instance row should have this populated.
-- Remote instances only have their public keys stored.
ALTER TABLE instances ADD COLUMN IF NOT EXISTS private_key_pem TEXT;
