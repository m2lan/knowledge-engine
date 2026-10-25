-- Backfill embedding dimensions for knowledge bases that were created by the
-- legacy RAG service. Its schema hard-coded vector(768); the default
-- embedding_dimensions column added in 001 would otherwise claim 1536 and make
-- every search fail the dimension check.
DO $$
DECLARE
    legacy_dims INT;
BEGIN
    SELECT substring(format_type(atttypid, atttypmod) from 'vector\((\d+)\)')::INT
      INTO legacy_dims
      FROM pg_attribute
     WHERE attrelid = 'document_chunks'::regclass
       AND attname = 'embedding'
       AND NOT attisdropped;

    IF legacy_dims IS NOT NULL AND legacy_dims <> 1536 THEN
        UPDATE knowledge_bases
           SET embedding_dimensions = legacy_dims
         WHERE chunk_count > 0
           AND embedding_dimensions = 1536;
    END IF;
END $$;
