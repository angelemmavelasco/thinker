CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) UNIQUE NOT NULL,
    family_id UUID NOT NULL,
    replaced_by_id UUID,
    revoked INTEGER NOT NULL DEFAULT 0,
    user_agent TEXT NOT NULL DEFAULT '',
    ip_address VARCHAR(64) NOT NULL DEFAULT '',
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS action_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) UNIQUE NOT NULL,
    token_type VARCHAR(32) NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    consumed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_lower ON users(LOWER(email));
CREATE INDEX IF NOT EXISTS idx_users_google_id ON users(google_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family_id ON refresh_tokens(family_id);
CREATE INDEX IF NOT EXISTS idx_action_tokens_user_type ON action_tokens(user_id, token_type);
