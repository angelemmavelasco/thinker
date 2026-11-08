CREATE TABLE IF NOT EXISTS keywords (
    id UUID PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    embedding TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS idea_keywords (
    idea_id UUID NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    keyword_id UUID NOT NULL REFERENCES keywords(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (idea_id, keyword_id)
);

CREATE TABLE IF NOT EXISTS idea_links (
    id UUID PRIMARY KEY,
    source_idea_id UUID NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    target_idea_id UUID NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    relation_type VARCHAR(50) NOT NULL DEFAULT 'related',
    weight DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (source_idea_id, target_idea_id, relation_type)
);

CREATE TABLE IF NOT EXISTS idea_chunks (
    id UUID PRIMARY KEY,
    idea_id UUID NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    content TEXT NOT NULL,
    embedding TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_idea_keywords_keyword_id ON idea_keywords(keyword_id);
CREATE INDEX IF NOT EXISTS idx_idea_links_source ON idea_links(source_idea_id);
CREATE INDEX IF NOT EXISTS idx_idea_links_target ON idea_links(target_idea_id);
CREATE INDEX IF NOT EXISTS idx_idea_chunks_idea_id ON idea_chunks(idea_id);
