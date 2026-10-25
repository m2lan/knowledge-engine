-- =============================================================================
-- Migration: Create Knowledge Engine tables
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS vector;

-- Knowledge Bases
CREATE TABLE IF NOT EXISTS knowledge_bases (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL,
    org_id         UUID,
    template_id    UUID,
    name           VARCHAR(100) NOT NULL,
    description    TEXT,
    embedding_model VARCHAR(100) NOT NULL DEFAULT 'text-embedding-3-small',
    embedding_dimensions INT NOT NULL DEFAULT 1536,
    chunk_size     INT NOT NULL DEFAULT 512,
    chunk_overlap  INT NOT NULL DEFAULT 50,
    doc_count      INT NOT NULL DEFAULT 0,
    chunk_count    INT NOT NULL DEFAULT 0,
    total_tokens   BIGINT NOT NULL DEFAULT 0,
    total_size     BIGINT NOT NULL DEFAULT 0,
    settings       JSONB NOT NULL DEFAULT '{}',
    status         VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ
);

ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS embedding_dimensions INT NOT NULL DEFAULT 1536;
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS template_id UUID;

CREATE INDEX IF NOT EXISTS idx_kb_user ON knowledge_bases(user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_kb_org ON knowledge_bases(org_id) WHERE deleted_at IS NULL;

-- Documents
CREATE TABLE IF NOT EXISTS documents (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    knowledge_base_id UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    filename         VARCHAR(255) NOT NULL,
    file_type        VARCHAR(50) NOT NULL,
    file_size        BIGINT NOT NULL,
    file_url         VARCHAR(500) NOT NULL,
    status           VARCHAR(20) NOT NULL DEFAULT 'uploading',
    error            TEXT,
    chunk_count      INT NOT NULL DEFAULT 0,
    total_tokens     BIGINT NOT NULL DEFAULT 0,
    tags             TEXT[] DEFAULT '{}',
    metadata         JSONB NOT NULL DEFAULT '{}',
    processed_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_docs_kb ON documents(knowledge_base_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_docs_status ON documents(status) WHERE status != 'ready';

-- Columns added after the initial RAG schema; required when adopting an
-- existing database created by the legacy gateway/rag service.
ALTER TABLE documents ADD COLUMN IF NOT EXISTS tags TEXT[] DEFAULT '{}';

-- Document Chunks
-- The default column matches the default embedding provider (OpenAI
-- text-embedding-3-small, 1536 dims). Engines embedding with a different
-- dimension create an additional `embedding_<dimensions>` column at startup.
CREATE TABLE IF NOT EXISTS document_chunks (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id      UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    knowledge_base_id UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    chunk_index      INT NOT NULL,
    content          TEXT NOT NULL,
    content_length   INT NOT NULL,
    token_count      INT NOT NULL,
    start_page       INT,
    end_page         INT,
    heading          TEXT,
    metadata         JSONB NOT NULL DEFAULT '{}',
    embedding        vector(1536),
    content_tsv      tsvector,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(document_id, chunk_index)
);

CREATE INDEX IF NOT EXISTS idx_chunks_doc ON document_chunks(document_id);
CREATE INDEX IF NOT EXISTS idx_chunks_kb ON document_chunks(knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_chunks_embedding ON document_chunks
    USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);
CREATE INDEX IF NOT EXISTS idx_chunks_fts ON document_chunks USING GIN(content_tsv);

-- Trigger to update tsvector (supports Chinese via zhparser if available)
CREATE OR REPLACE FUNCTION update_chunk_tsv() RETURNS trigger AS $$
BEGIN
    -- Try Chinese config first, fall back to simple
    BEGIN
        NEW.content_tsv := to_tsvector('chinese', NEW.content);
    EXCEPTION WHEN OTHERS THEN
        NEW.content_tsv := to_tsvector('simple', NEW.content);
    END;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_chunk_tsv ON document_chunks;
CREATE TRIGGER trg_chunk_tsv
    BEFORE INSERT OR UPDATE ON document_chunks
    FOR EACH ROW EXECUTE FUNCTION update_chunk_tsv();

-- Auto-update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS update_knowledge_bases_updated_at ON knowledge_bases;
CREATE TRIGGER update_knowledge_bases_updated_at
    BEFORE UPDATE ON knowledge_bases
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_documents_updated_at ON documents;
CREATE TRIGGER update_documents_updated_at
    BEFORE UPDATE ON documents
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
