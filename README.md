# Knowledge Engine

A pluggable, embeddable knowledge engine for document ingestion, chunking, embedding, and hybrid search. Designed to be integrated into any project as a standalone service, Go SDK, or embedded library.

## Features

- **Hybrid Search** — Vector similarity (pgvector) + BM25 full-text search, merged via Reciprocal Rank Fusion (RRF) and normalized to `0..1`
- **Chinese Support** — Runtime zhparser detection for Chinese full-text search, automatic fallback to `simple` config
- **Multi-format Parsing** — PDF, DOCX, PPTX, XLSX, CSV, HTML, JSON, and source code via Apache Tika; plain-text formats fall back to a native parser when Tika is unavailable
- **Automatic Migrations** — Embedded SQL migrations run on startup (`schema_migrations` + advisory lock), so existing database volumes upgrade themselves
- **Dynamic Embedding Dimensions** — the vector column is resolved at startup from the embedder (`embedding` for 1536, `embedding_<dims>` otherwise), so OpenAI, Gemini, Ollama and local models can share one database
- **Versioned Search Cache** — Redis (or in-memory fallback); ingestion/deletion bumps a per-KB generation to invalidate stale results
- **Pluggable Embedding** — OpenAI, Gemini, Ollama (OpenAI-compatible); easy to add custom providers via registry
- **Pluggable Storage** — Local filesystem, MinIO/S3; implement the `Storage` interface for custom backends
- **Pluggable Auth** — JWT (HS256), static API key, no-op (development); implement the `Authenticator` interface for custom auth
- **Industry Templates** — Configurable metadata schemas per industry (legal, medical, ecommerce, etc.)
- **RBAC Permissions** — User, team, and organization-level access control with audit logging
- **SDK & HTTP API** — Use as a Go library or call via REST API

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Knowledge Engine                        │
├─────────────┬─────────────┬──────────────┬──────────────────┤
│  Transport  │   Service   │   Internal   │    Adapters      │
│             │             │              │                  │
│  HTTP (chi) │  KB Service │  Chunker     │  DB: PostgreSQL  │
│  gRPC (WIP) │  Search Svc │  Parser      │  Cache: Redis    │
│  SDK (Go)   │  Perm Svc   │  Embedder    │  Storage: MinIO  │
│             │             │  Retriever   │  Auth: JWT/Key   │
├─────────────┴─────────────┴──────────────┴──────────────────┤
│                       Domain Layer                           │
│  KnowledgeBase · Document · Chunk · Permission · Template    │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

### 1. Docker Compose (Recommended)

```bash
# Clone the repo
git clone git@github.com:m2lan/knowledge-engine.git
cd knowledge-engine

# Set your embedding API key
export EMBEDDING_API_KEY=sk-...

# Start everything: PostgreSQL + pgvector, Redis, Apache Tika, Knowledge Engine
docker-compose -f deploy/docker/docker-compose.yml up -d

# Verify it's running (compose defaults to no-auth local mode)
curl http://localhost:8083/health
curl -X POST http://localhost:8083/api/v1/knowledge \
  -H "Content-Type: application/json" \
  -d '{"name": "Trial KB"}'
```

Migrations run automatically on startup — no manual SQL step, and no initdb scripts that only execute on a fresh volume. To protect an exposed deployment, set `AUTH_TYPE=apikey` and `API_KEY=<secret>` (or `AUTH_TYPE=jwt` and `JWT_SECRET=<secret>`) before starting.

### 2. Go SDK

```go
package main

import (
    "context"
    "fmt"

    knowledge "github.com/omnidev/knowledge-engine"
)

func main() {
    engine, err := knowledge.New(knowledge.Config{
        DatabaseURL: "postgres://postgres:postgres@localhost:5432/knowledge?sslmode=disable",
        Embedding: knowledge.EmbeddingConfig{
            Provider: "openai",
            APIKey:   "sk-...",
        },
    })
    if err != nil {
        panic(err)
    }
    defer engine.Close()

    ctx := context.Background()

    // Create a knowledge base
    kb, _ := engine.CreateKnowledgeBase(ctx, "user-id", knowledge.CreateKBInput{
        Name: "Product Documentation",
    })

    // Upload a document
    file, _ := os.Open("manual.pdf")
    defer file.Close()
    doc, _ := engine.UploadDocument(ctx, "user-id", kb.ID.String(), "manual.pdf", fileSize, file)

    // Search
    results, _ := engine.Search(ctx, "user-id", kb.ID.String(), "how to reset password", 5)
    for _, r := range results {
        fmt.Printf("[%.3f] %s\n", r.Score, r.Chunk.Content[:100])
    }
}
```

### 3. Embed into Existing Go Application

If your application already has a PostgreSQL connection pool:

```go
import knowledge "github.com/omnidev/knowledge-engine"

// Create embedder
emb, _ := knowledge.NewEmbedder(knowledge.EmbedderConfig{
    Provider: "openai",
    APIKey:   "sk-...",
})

// Create engine from an existing pool. This runs pending migrations and
// resolves the vector column for the embedder's dimensions.
engine, err := knowledge.NewFromPool(existingPool, emb, nil, nil)
if err != nil {
    return err
}
defer engine.Close()

// Use directly
results, _ := engine.Search(ctx, userID, kbID, "query", 5)
```

### 4. HTTP API

Set `AUTH` to an empty string for no-auth mode, or pass the configured credential on every request:

```bash
AUTH=                                   # noop
# AUTH='X-API-Key: my-secret'           # AUTH_TYPE=apikey
# AUTH='Authorization: Bearer <jwt>'    # AUTH_TYPE=jwt
```

```bash
# Health check (unauthenticated)
curl http://localhost:8083/health

# Create knowledge base (chunk_size/chunk_overlap are per-KB)
curl -X POST http://localhost:8083/api/v1/knowledge \
  -H "Content-Type: application/json" -H "$AUTH" \
  -d '{"name": "Product Docs", "description": "Main product documentation", "chunk_size": 512, "chunk_overlap": 50}'

# Upload document (async: returns immediately, chunks are written in the background)
curl -X POST http://localhost:8083/api/v1/knowledge/{kb_id}/documents \
  -H "$AUTH" -F "file=@manual.pdf"

# Hybrid search
curl -X POST http://localhost:8083/api/v1/knowledge/{kb_id}/search \
  -H "Content-Type: application/json" -H "$AUTH" \
  -d '{"query": "how to reset password", "top_k": 5, "min_score": 0.01}'

# List knowledge bases
curl http://localhost:8083/api/v1/knowledge?page=1&page_size=20 -H "$AUTH"

# Delete document
curl -X DELETE http://localhost:8083/api/v1/knowledge/{kb_id}/documents/{doc_id} -H "$AUTH"
```

### 5. MCP Server

The same engine is exposed as a [Model Context Protocol](https://modelcontextprotocol.io) server, so
Claude Desktop, Cursor, VS Code, and other MCP clients can use a knowledge base as tools without
writing glue code.

Two transports are available:

| Transport | How to run | Authentication |
|-----------|------------|----------------|
| Streamable HTTP | Built into the main server at `POST /mcp` | Same as the REST API (JWT / API key / noop) |
| stdio | `go build -o bin/knowledge-mcp ./cmd/mcp` | `MCP_USER_ID` environment variable (defaults to `DEFAULT_USER_ID`) |

**MCP client configuration (stdio):**

```json
{
  "mcpServers": {
    "knowledge": {
      "command": "/path/to/knowledge-mcp",
      "env": {
        "DATABASE_URL": "postgres://postgres:postgres@localhost:5432/knowledge?sslmode=disable",
        "EMBEDDING_PROVIDER": "openai",
        "EMBEDDING_API_KEY": "sk-...",
        "MCP_USER_ID": "00000000-0000-0000-0000-000000000001"
      }
    }
  }
}
```

**HTTP transport:**

```bash
curl -X POST http://localhost:8083/mcp \
  -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" -H "$AUTH" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"curl","version":"1.0"}}}'

curl -X POST http://localhost:8083/mcp \
  -H "Content-Type: application/json" -H "$AUTH" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'

curl -X POST http://localhost:8083/mcp \
  -H "Content-Type: application/json" -H "$AUTH" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"knowledge_search","arguments":{"knowledge_base_id":"<kb_id>","query":"reset password","top_k":5}}}'
```

The HTTP endpoint is stateless: each POST carries one JSON-RPC message (or batch) and returns JSON.
Notifications such as `notifications/initialized` return `202 Accepted`. JSON-RPC protocol version:
`2024-11-05`.

**Tools:**

| Tool | Purpose |
|------|---------|
| `knowledge_search` | Hybrid vector + full-text search in one knowledge base |
| `knowledge_list_bases` | List the caller's knowledge bases |
| `knowledge_get_base` | Get knowledge base metadata and statistics |
| `knowledge_create_base` | Create a knowledge base (per-KB chunk size/overlap) |
| `knowledge_list_documents` | List documents and their ingestion status |
| `knowledge_ingest_text` | Ingest plain text or Markdown (async parsing/embedding) |

All tools operate under the authenticated user identity, so the same ownership rules apply as the
REST API.

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | — | PostgreSQL connection string (required) |
| `SKIP_MIGRATIONS` | `false` | Skip embedded SQL migrations on startup |
| `REDIS_URL` | — | Redis URL (optional; in-memory cache if empty or unreachable) |
| `SEARCH_CACHE_TTL` | `60s` | How long search results stay cached |
| `STORAGE_TYPE` | `local` | Storage backend: `local`, `minio` |
| `STORAGE_BUCKET` | `knowledge-docs` | Storage bucket name |
| `STORAGE_LOCAL_PATH` | `./data/docs` | Local storage path |
| `STORAGE_ENDPOINT` | — | MinIO/S3 endpoint |
| `STORAGE_ACCESS_KEY` | — | MinIO/S3 access key |
| `STORAGE_SECRET_KEY` | — | MinIO/S3 secret key |
| `STORAGE_USE_SSL` | `false` | Use TLS for MinIO/S3 |
| `EMBEDDING_PROVIDER` | `openai` | Embedding provider: `openai`, `gemini`, `ollama` |
| `EMBEDDING_API_KEY` | — | API key for embedding provider (optional for local OpenAI-compatible endpoints) |
| `EMBEDDING_MODEL` | — | Model name (provider default if empty) |
| `EMBEDDING_BASE_URL` | — | Custom endpoint URL (for Ollama or compatible APIs) |
| `EMBEDDING_DIMENSIONS` | model default | Expected vector size; the DB column is resolved from it |
| `PARSER_TYPE` | `tika` | `tika` or `native` (plain-text formats only) |
| `TIKA_ENDPOINT` | `http://localhost:9998` | Apache Tika server base URL |
| `TIKA_TIMEOUT` | `60s` | Per-document Tika request timeout |
| `TIKA_DISABLE_NATIVE_FALLBACK` | `false` | Fail instead of parsing txt/md/csv/html/code locally when Tika is down |
| `AUTH_TYPE` | inferred | `jwt`, `apikey`, or `noop`; inferred from `JWT_SECRET`/`API_KEY` |
| `JWT_SECRET` | — | HS256 secret; enables JWT auth when `AUTH_TYPE` is unset |
| `JWT_ISSUER` | `knowledge-engine` | JWT issuer validation (empty disables issuer check) |
| `API_KEY` | — | Static API key; enables API-key auth when `AUTH_TYPE` is unset |
| `API_KEY_USER_ID` | `00000000-0000-0000-0000-000000000001` | UUID attributed to API-key requests |
| `DEFAULT_USER_ID` | `00000000-0000-0000-0000-000000000001` | UUID attributed to requests in no-op mode |
| `MCP_USER_ID` | `DEFAULT_USER_ID` | Identity used by the stdio MCP server (`cmd/mcp`) |
| `SERVER_ADDRESS` | `:8083` | HTTP listen address |
| `SERVER_READ_TIMEOUT` | `30s` | HTTP read timeout |
| `SERVER_WRITE_TIMEOUT` | `300s` | HTTP write timeout (large uploads) |

### Embedding Providers

| Provider | Default Model | Default Dimensions | Batch Support |
|----------|--------------|--------------------|---------------|
| `openai` | `text-embedding-3-small` | 1536 | Yes (2048/batch) |
| `gemini` | `gemini-embedding-001` | 768 | No (sequential) |
| `ollama` | `nomic-embed-text` | 768 | Yes (OpenAI-compatible) |

For OpenAI `text-embedding-3-*` models, setting `EMBEDDING_DIMENSIONS` also sends the
`dimensions` request parameter, so you can run e.g. 512-dimensional vectors without
changing the model.

### Embedding Dimensions and Schema

The migration creates `embedding vector(1536)` plus `knowledge_bases.embedding_dimensions`.
At startup the engine asks the embedder for its dimensions and:

- reuses `embedding` when dimensions are 1536;
- otherwise creates `embedding_<dims>` (e.g. `embedding_768`, `embedding_384`) and its HNSW index.

That means a single database can serve several models, and switching from an OpenAI model to
Gemini/Ollama does not require rewriting schema. HNSW indexes are skipped above 2000 dimensions
(pgvector's limit); search still works with exact scan. Knowledge bases record the model and
dimensions they were created with, and ingestion/search reject a mismatch instead of silently
writing vectors of the wrong size.

### Authentication

| Mode | Request credential | Identity |
|------|--------------------|----------|
| `noop` | none | `DEFAULT_USER_ID` (development/trial only) |
| `apikey` | `X-API-Key: <API_KEY>` or `Authorization: Bearer <API_KEY>` | `API_KEY_USER_ID` |
| `jwt` | `Authorization: Bearer <JWT>` (HS256, `user_id` claim or `sub`, must be a UUID) | token subject |

If `AUTH_TYPE` is empty, setting `API_KEY` selects API-key mode, else `JWT_SECRET` selects JWT,
else no-op. JWTs must be signed with `JWT_SECRET`, carry a UUID in `user_id` (or `sub`), and be
issued by `JWT_ISSUER` (default `knowledge-engine`); `exp`, when present, is validated. Every
knowledge base and document is scoped to the authenticated user; cross-user access returns
`403`/`404`.

## API Reference

### Knowledge Bases

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/knowledge` | List knowledge bases (paginated) |
| `POST` | `/api/v1/knowledge` | Create knowledge base |
| `GET` | `/api/v1/knowledge/{id}` | Get knowledge base |
| `PATCH` | `/api/v1/knowledge/{id}` | Update knowledge base |
| `DELETE` | `/api/v1/knowledge/{id}` | Delete knowledge base |

### Documents

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/knowledge/{id}/documents` | List documents (paginated) |
| `POST` | `/api/v1/knowledge/{id}/documents` | Upload document (multipart) |
| `DELETE` | `/api/v1/knowledge/{id}/documents/{doc_id}` | Delete document |

### Search

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/knowledge/{id}/search` | Hybrid search |

`chunk_size` (100–8192, default 512) and `chunk_overlap` (default 50, must be smaller than
`chunk_size`) are stored per knowledge base and applied to every document ingested into it.
Documents are parsed/chunked/embedded asynchronously; poll the document until its status is
`ready` or `failed`.

**Search Request Body:**
```json
{
  "query": "search text",
  "top_k": 10,
  "min_score": 0.01
}
```

**Search Response:**
```json
{
  "data": [
    {
      "chunk": {
        "id": "...",
        "content": "...",
        "metadata": {}
      },
      "score": 0.83,
      "source": "hybrid",
      "vector_score": 0.91,
      "keyword_score": 0.62
    }
  ]
}
```

`score` is the normalized RRF hybrid score in `0..1`, so `min_score` behaves consistently
across vector-only, keyword-only, and hybrid results. Search results are cached per KB
generation and invalidated after ingestion, document deletion, or KB updates.

## Integration Guide

### With OmniDev Platform

The OmniDev Gateway integrates knowledge-engine as a Go SDK:

```go
// In gateway's main.go
knowledgeEngine, err := knowledge.NewFromPool(db.Pool, emb, nil, nil)
if err != nil {
    return err
}
kbSvc := knowledgeEngine.KnowledgeBaseService()
searchSvc := knowledgeEngine.SearchService()
```

### With Any Go Application

```go
import knowledge "github.com/omnidev/knowledge-engine"

// Option A: Full standalone engine
engine, _ := knowledge.New(knowledge.Config{...})

// Option B: From existing DB pool (runs migrations, resolves vector column)
engine, err := knowledge.NewFromPool(pool, embedder, storage, logger)
if err != nil {
    return err
}

// Use services directly
kbSvc := engine.KnowledgeBaseService()
searchSvc := engine.SearchService()
```

### With Non-Go Applications

Run knowledge-engine as a sidecar service and call via HTTP API:

```yaml
# docker-compose.yml
services:
  your-app:
    image: your-app
    environment:
      KNOWLEDGE_ENGINE_URL: http://knowledge-engine:8083

  knowledge-engine:
    image: omnidev/knowledge-engine:latest
    ports:
      - "8083:8083"
```

## Development

```bash
# Unit tests (Tika, embedding, auth, chunking, retrieval)
go test ./...

# Integration tests need a pgvector database and Redis
docker run --rm -d --name ke-test-pg -p 55432:5432 \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=knowledge_test \
  pgvector/pgvector:pg16

TEST_DATABASE_URL='postgres://postgres:postgres@localhost:55432/knowledge_test?sslmode=disable' \
TEST_REDIS_URL='redis://localhost:6379/0' \
go test ./internal/integration -v -count=1

# Build and run locally
go build -o knowledge-engine ./cmd/server
export DATABASE_URL=postgres://...
export EMBEDDING_API_KEY=sk-...
export TIKA_ENDPOINT=http://localhost:9998   # or PARSER_TYPE=native
./knowledge-engine
```

## Project Structure

```
knowledge-engine/
├── engine.go              # Facade: New() + NewFromPool()
├── interfaces.go          # Public API exports (type aliases)
├── config.go              # Configuration struct
├── errors.go              # Error types
├── types/                 # Shared interfaces (avoids import cycles)
├── domain/                # Core entities
│   ├── knowledge.go       # KnowledgeBase, Document, Chunk
│   ├── search.go          # SearchResult, SearchRequest
│   ├── permission.go      # RBAC entities
│   └── template.go        # Industry templates
├── internal/              # Business logic (not importable externally)
│   ├── chunker/           # Text chunking strategies
│   ├── parser/            # Apache Tika client + native plain-text fallback
│   ├── embedder/          # Embedding providers (registry pattern)
│   ├── retriever/         # Hybrid search (vector + BM25 + RRF)
│   └── service/           # KB, Search, Permission services
├── adapters/              # Pluggable backends
│   ├── database/postgres/ # PostgreSQL + pgvector
│   ├── cache/redis/       # Redis cache
│   ├── cache/memory/      # In-memory cache (dev/test)
│   ├── storage/minio/     # MinIO/S3 storage
│   ├── storage/local/     # Local filesystem
│   ├── auth/jwt/          # JWT authentication
│   ├── auth/apikey/       # Static API-key authentication
│   └── auth/noop/         # No-auth (development)
├── transport/http/        # HTTP server (chi router)
├── cmd/server/            # Standalone entry point
├── migrations/            # Embedded SQL migrations (+ advisory-lock runner)
└── deploy/docker/         # Docker + Compose
```

## License

MIT
