-- Admin console schema.
--
-- The customer-facing API identifies callers with X-Demo-Customer-ID, which is a
-- dev-only header and must never be reused for staff access. The console
-- therefore gets its own tables: real accounts, server-side sessions, and a
-- ticket note trail so an operator's handling steps are reviewable.
--
-- Passwords are stored as PBKDF2-HMAC-SHA256 with a per-user random salt in the
-- form pbkdf2_sha256$<iter>$<salt-b64>$<hash-b64>. Session ids in the table are
-- SHA-256 digests of the cookie token; the raw token never touches the disk.

CREATE TABLE IF NOT EXISTS admin_users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'admin' CHECK (role IN ('admin','supervisor','agent')),
    status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    last_login_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_admin_users_status ON admin_users(status);

CREATE TABLE IF NOT EXISTS admin_sessions (
    id            TEXT PRIMARY KEY,
    admin_user_id TEXT NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
    created_at    TEXT NOT NULL,
    expires_at    TEXT NOT NULL,
    revoked_at    TEXT,
    remote_addr   TEXT,
    user_agent    TEXT
);
CREATE INDEX IF NOT EXISTS idx_admin_sessions_user ON admin_sessions(admin_user_id, expires_at DESC);
CREATE INDEX IF NOT EXISTS idx_admin_sessions_expiry ON admin_sessions(expires_at);

-- Operator handling notes for a ticket: the free-text trail behind a status
-- change. Kept separate from service_tickets so the ticket row stays immutable
-- except for its lifecycle columns.
CREATE TABLE IF NOT EXISTS service_ticket_notes (
    id            TEXT PRIMARY KEY,
    ticket_id     TEXT NOT NULL REFERENCES service_tickets(id) ON DELETE CASCADE,
    admin_user_id TEXT NOT NULL,
    author_name   TEXT NOT NULL,
    note          TEXT NOT NULL,
    from_status   TEXT,
    to_status     TEXT,
    created_at    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ticket_notes_ticket ON service_ticket_notes(ticket_id, created_at);
