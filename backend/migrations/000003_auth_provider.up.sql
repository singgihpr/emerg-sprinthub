ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_provider TEXT NOT NULL DEFAULT 'local';
ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_provider_id TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_auth_provider_id ON users(auth_provider, auth_provider_id) WHERE auth_provider_id IS NOT NULL;
