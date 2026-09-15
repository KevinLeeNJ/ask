CREATE TABLE IF NOT EXISTS conversations (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    title_source TEXT NOT NULL DEFAULT 'provisional'
        CHECK (title_source IN ('provisional', 'agent', 'user')),
    title_attempted_at INTEGER NOT NULL DEFAULT 0,
    system_prompt_snapshot TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_conversations_updated_at
    ON conversations(updated_at);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('system', 'user', 'assistant')),
    content TEXT NOT NULL,
    reasoning_content TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('complete', 'partial', 'failed')),
    provider TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    input_tokens INTEGER,
    output_tokens INTEGER,
    reasoning_tokens INTEGER,
    error TEXT,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_messages_conversation_created
    ON messages(conversation_id, created_at);

CREATE TABLE IF NOT EXISTS conversation_leases (
    conversation_id TEXT PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
    lease_owner TEXT NOT NULL,
    lease_expires_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_conversation_leases_expires
    ON conversation_leases(lease_expires_at);

CREATE TABLE IF NOT EXISTS app_state (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
