# Knowledge Engine

A pluggable, embeddable knowledge engine for document ingestion, chunking, embedding, and hybrid search. Designed to be integrated into any project as a standalone service, Go SDK, or embedded library.

## Features

- **Hybrid Search** — Vector similarity (pgvector) + BM25 full-text search, merged via Reciprocal Rank Fusion (RRF)
- **Chinese Support** — Runtime zhparser detection for Chinese full-text search, automatic fallback to `simple` config
- **Multi-format Parsing** — PDF, DOCX, PPTX, XLSX, CSV, HTML, JSON, and source code (Go, Python, JS, TS, Java, Rust, C++, etc.)
- **Pluggable Embedding** — OpenAI, Gemini, Ollama (OpenAI-compatible); easy to add custom providers via registry
- **Pluggable Storage** — Local filesystem, MinIO/S3; implement the `Storage` interface for custom backends
- **Pluggable Auth** — JWT, no-op (development); implement the `Authenticator` interface for custom auth
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
│             │             │  Retriever   │  Auth: JWT       │
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

# Start all services (PostgreSQL + pgvector, Redis, Knowledge Engine)
docker-compose -f deploy/docker/docker-compose.yml up -d

# Verify it's running
curl http://localhost:8083/health
```

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
import (
    knowledge "github.com/omnidev/knowledge-engine"
    "github.com/omnidev/knowledge-engine/internal/embedder"
)

// Create embedder
emb, _ := knowledge.NewEmbedder(knowledge.EmbedderConfig{
    Provider: "openai",
    APIKey:   "sk-...",
})

// Create engine from existing pool
engine := knowledge.NewFromPool(existingPool, emb, nil, nil)
defer engine.Close()

// Use directly
results, _ := engine.Search(ctx, userID, kbID, "query", 5)
```

### 4. HTTP API

```bash
# Health check
curl http://localhost:8083/health

# Create knowledge base
curl -X POST http://localhost:8083/api/v1/knowledge \
  -H "Content-Type: application/json" \
  -d '{"name": "Product Docs", "description": "Main product documentation"}'

# Upload document
curl -X POST http://localhost:8083/api/v1/knowledge/{kb_id}/documents \
  -F "file=@manual.pdf"

# Hybrid search
curl -X POST http://localhost:8083/api/v1/knowledge/{kb_id}/search \
  -H "Content-Type: application/json" \
  -d '{"query": "how to reset password", "top_k": 5, "min_score": 0.01}'

# List knowledge bases
curl http://localhost:8083/api/v1/knowledge?page=1&page_size=20

# Delete document
curl -X DELETE http://localhost:8083/api/v1/knowledge/{kb_id}/documents/{doc_id}
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | — | PostgreSQL connection string (required) |
| `REDIS_URL` | — | Redis URL (optional, uses in-memory cache if empty) |
| `STORAGE_TYPE` | `local` | Storage backend: `local`, `minio` |
| `STORAGE_BUCKET` | `knowledge-docs` | Storage bucket name |
| `STORAGE_LOCAL_PATH` | `./data/docs` | Local storage path |
| `STORAGE_ENDPOINT` | — | MinIO/S3 endpoint |
| `STORAGE_ACCESS_KEY` | — | MinIO/S3 access key |
| `STORAGE_SECRET_KEY` | — | MinIO/S3 secret key |
| `EMBEDDING_PROVIDER` | `openai` | Embedding provider: `openai`, `gemini`, `ollama` |
| `EMBEDDING_API_KEY` | — | API key for embedding provider |
| `EMBEDDING_MODEL` | — | Model name (provider default if empty) |
| `EMBEDDING_BASE_URL` | — | Custom endpoint URL (for Ollama or compatible APIs) |
| `JWT_SECRET` | — | JWT secret for authentication (no-auth if empty) |
| `JWT_ISSUER` | `knowledge-engine` | JWT issuer for validation |
| `SERVER_ADDRESS` | `:8083` | HTTP listen address |

### Embedding Providers

| Provider | Default Model | Dimensions | Batch Support |
|----------|--------------|------------|---------------|
| `openai` | `text-embedding-3-small` | 1536 | Yes (2048/batch) |
| `gemini` | `gemini-embedding-2` | 768 | No (sequential) |
| `ollama` | `nomic-embed-text` | 768 | Yes (OpenAI-compatible) |

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
      "score": 0.016,
      "source": "hybrid"
    }
  ]
}
```

## Integration Guide

### With OmniDev Platform

The OmniDev Gateway integrates knowledge-engine as a Go SDK:

```go
// In gateway's main.go
knowledgeEngine := knowledge.NewFromPool(db.Pool, emb, nil, nil)
kbSvc := knowledgeEngine.KnowledgeBaseService()
searchSvc := knowledgeEngine.SearchService()
```

### With Any Go Application

```go
import knowledge "github.com/omnidev/knowledge-engine"

// Option A: Full standalone engine
engine, _ := knowledge.New(knowledge.Config{...})

// Option B: From existing DB pool
engine := knowledge.NewFromPool(pool, embedder, storage, logger)

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
# Run tests
go test ./...

# Build
go build -o knowledge-engine ./cmd/server

# Run locally
export DATABASE_URL=postgres://...
export EMBEDDING_API_KEY=sk-...
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
│   ├── parser/            # Document parsing
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
│   └── auth/noop/         # No-auth (development)
├── transport/http/        # HTTP server (chi router)
├── cmd/server/            # Standalone entry point
├── migrations/            # SQL migrations
└── deploy/docker/         # Docker + Compose
```

## License

MIT
