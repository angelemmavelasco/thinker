CREATE TABLE IF NOT EXISTS knowledge_mocs (
    id UUID PRIMARY KEY,
    code VARCHAR(64) UNIQUE NOT NULL,
    title VARCHAR(150) NOT NULL,
    description TEXT NOT NULL,
    embedding TEXT NOT NULL DEFAULT '',
    display_order INTEGER NOT NULL DEFAULT 0,
    is_locked INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS knowledge_hubs (
    id UUID PRIMARY KEY,
    moc_id UUID NOT NULL REFERENCES knowledge_mocs(id) ON DELETE CASCADE,
    title VARCHAR(150) NOT NULL,
    summary TEXT NOT NULL,
    search_aliases TEXT NOT NULL DEFAULT '[]',
    embedding TEXT NOT NULL DEFAULT '',
    display_order INTEGER NOT NULL DEFAULT 0,
    is_locked INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS hub_idea_edges (
    hub_id UUID NOT NULL REFERENCES knowledge_hubs(id) ON DELETE CASCADE,
    idea_id UUID NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    relationship_type VARCHAR(50) NOT NULL DEFAULT 'GOVERNS',
    weight DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    is_manual_override INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (hub_id, idea_id)
);

CREATE TABLE IF NOT EXISTS knowledge_settings (
    key VARCHAR(64) PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_knowledge_hubs_moc ON knowledge_hubs(moc_id);
CREATE INDEX IF NOT EXISTS idx_hub_edges_idea ON hub_idea_edges(idea_id);
CREATE INDEX IF NOT EXISTS idx_hub_edges_hub ON hub_idea_edges(hub_id);
