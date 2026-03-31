-- V10: Auth service tables
-- Creates auth schema with users, sessions, api_keys, audit_log, and pkce_challenges

CREATE SCHEMA IF NOT EXISTS auth;

-- Users table
CREATE TABLE auth.users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           VARCHAR(255) UNIQUE NOT NULL,
    password_hash   TEXT NOT NULL,
    role            VARCHAR(50) NOT NULL DEFAULT 'trader',
    locked_until    TIMESTAMPTZ,
    failed_attempts INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_auth_users_email ON auth.users (email);

-- Sessions table
CREATE TABLE auth.sessions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    refresh_token_hash  TEXT NOT NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    revoked             BOOLEAN NOT NULL DEFAULT FALSE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_auth_sessions_user_id ON auth.sessions (user_id);
CREATE INDEX idx_auth_sessions_refresh_token_hash ON auth.sessions (refresh_token_hash);

-- API keys table
CREATE TABLE auth.api_keys (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    key_hash    TEXT NOT NULL,
    name        VARCHAR(255),
    prefix      VARCHAR(16),
    permissions TEXT[],
    expires_at  TIMESTAMPTZ,
    revoked     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_auth_api_keys_user_id ON auth.api_keys (user_id);
CREATE INDEX idx_auth_api_keys_key_hash ON auth.api_keys (key_hash);

-- Audit log table
CREATE TABLE auth.audit_log (
    id          BIGSERIAL PRIMARY KEY,
    user_id     UUID,
    action      VARCHAR(100) NOT NULL,
    ip_address  VARCHAR(45),
    user_agent  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_auth_audit_log_user_id ON auth.audit_log (user_id);
CREATE INDEX idx_auth_audit_log_created_at ON auth.audit_log (created_at);

-- PKCE challenges table (used for OAuth2 PKCE flow)
CREATE TABLE auth.pkce_challenges (
    auth_code               VARCHAR(64) PRIMARY KEY,
    code_challenge          TEXT NOT NULL,
    code_challenge_method   VARCHAR(10) NOT NULL DEFAULT 'S256',
    user_id                 UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    redirect_uri            TEXT,
    expires_at              TIMESTAMPTZ NOT NULL,
    used                    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_auth_pkce_user_id ON auth.pkce_challenges (user_id);
