# Knowledge Engine

一个可插拔、可嵌入的知识引擎，支持文档导入、分块、向量化和混合检索。设计为可作为独立服务、Go SDK 或嵌入式库集成到任何项目中。

## 特性

- **混合检索** — 向量相似度（pgvector）+ BM25 全文搜索，通过倒数排名融合（RRF）合并，并归一化到 `0..1`
- **中文支持** — 运行时自动检测 zhparser 扩展，支持中文全文搜索，自动降级到 `simple` 配置
- **多格式解析** — PDF、DOCX、PPTX、XLSX、CSV、HTML、JSON 及源代码通过 Apache Tika 解析；纯文本格式在 Tika 不可用时自动本地降级
- **自动迁移** — 启动时执行内嵌 SQL 迁移（`schema_migrations` + advisory lock），已有数据库 volume 也会自动升级
- **动态向量维度** — 启动时按 Embedding 模型解析向量列（1536 维用 `embedding`，其他维度用 `embedding_<dims>`），OpenAI、Gemini、Ollama 和本地模型可共用同一数据库
- **版本化搜索缓存** — Redis（不可用时回退内存）；入库/删除后按 KB generation 失效，不会返回旧结果
- **可插拔 Embedding** — 支持 OpenAI、Gemini、Ollama（OpenAI 兼容）；通过注册表模式轻松扩展
- **可插拔存储** — 本地文件系统、MinIO/S3；实现 `Storage` 接口即可自定义后端
- **可插拔认证** — JWT（HS256）、静态 API Key、无认证（开发模式）；实现 `Authenticator` 接口即可自定义认证
- **行业模板** — 可按行业配置元数据 Schema（法律、医疗、电商等）
- **RBAC 权限** — 用户、团队、组织级别的访问控制，支持审计日志
- **SDK + HTTP API** — 作为 Go 库直接调用，或通过 REST API 访问

## 架构

```
┌─────────────────────────────────────────────────────────────┐
│                      Knowledge Engine                        │
├─────────────┬─────────────┬──────────────┬──────────────────┤
│   传输层    │   服务层    │   内部组件    │     适配器层      │
│             │             │              │                  │
│  HTTP (chi) │  KB 服务    │  分块器      │  DB: PostgreSQL  │
│  gRPC (WIP) │  搜索服务   │  解析器      │  缓存: Redis     │
│  SDK (Go)   │  权限服务   │  Embedder    │  存储: MinIO     │
│             │             │  检索器      │  认证: JWT/Key   │
├─────────────┴─────────────┴──────────────┴──────────────────┤
│                       领域层                                 │
│  知识库 · 文档 · 分块 · 权限 · 模板                          │
└─────────────────────────────────────────────────────────────┘
```

## 快速开始

### 1. Docker Compose（推荐）

```bash
# 克隆仓库
git clone git@github.com:m2lan/knowledge-engine.git
cd knowledge-engine

# 设置 Embedding API Key
export EMBEDDING_API_KEY=sk-...

# 启动全部服务：PostgreSQL + pgvector、Redis、Apache Tika、Knowledge Engine
docker-compose -f deploy/docker/docker-compose.yml up -d

# 验证服务是否运行（compose 默认开启无认证本地试用模式）
curl http://localhost:8083/health
curl -X POST http://localhost:8083/api/v1/knowledge \
  -H "Content-Type: application/json" \
  -d '{"name": "试用知识库"}'
```

迁移会在启动时自动执行，不需要手动跑 SQL，也不依赖只在首次初始化 volume 时执行的 initdb 脚本。对外暴露部署前，请设置 `AUTH_TYPE=apikey` + `API_KEY=<密钥>`（或 `AUTH_TYPE=jwt` + `JWT_SECRET=<密钥>`）。

### 2. Go SDK

```go
package main

import (
    "context"
    "fmt"
    "os"

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

    // 创建知识库
    kb, _ := engine.CreateKnowledgeBase(ctx, "user-id", knowledge.CreateKBInput{
        Name: "产品文档",
    })

    // 上传文档
    file, _ := os.Open("manual.pdf")
    defer file.Close()
    info, _ := file.Stat()
    engine.UploadDocument(ctx, "user-id", kb.ID.String(), "manual.pdf", info.Size(), file)

    // 搜索
    results, _ := engine.Search(ctx, "user-id", kb.ID.String(), "如何重置密码", 5)
    for _, r := range results {
        fmt.Printf("[%.3f] %s\n", r.Score, r.Chunk.Content[:100])
    }
}
```

### 3. 嵌入到现有 Go 应用

如果你的应用已有 PostgreSQL 连接池：

```go
import (
    knowledge "github.com/omnidev/knowledge-engine"
)

// 创建 Embedder
emb, _ := knowledge.NewEmbedder(knowledge.EmbedderConfig{
    Provider: "openai",
    APIKey:   "sk-...",
})

// 从已有连接池创建引擎（会执行迁移并解析向量列维度）
engine, err := knowledge.NewFromPool(existingPool, emb, nil, nil)
if err != nil {
    return err
}
defer engine.Close()

// 直接使用
results, _ := engine.Search(ctx, userID, kbID, "查询内容", 5)
```

### 4. HTTP API

无认证模式下 `AUTH` 留空；启用认证后，每个请求都要带上对应凭据：

```bash
AUTH=                                   # noop
# AUTH='X-API-Key: my-secret'           # AUTH_TYPE=apikey
# AUTH='Authorization: Bearer <jwt>'    # AUTH_TYPE=jwt
```

```bash
# 健康检查（无需认证）
curl http://localhost:8083/health

# 创建知识库（chunk_size/chunk_overlap 按 KB 生效）
curl -X POST http://localhost:8083/api/v1/knowledge \
  -H "Content-Type: application/json" -H "$AUTH" \
  -d '{"name": "产品文档", "description": "主产品文档库", "chunk_size": 512, "chunk_overlap": 50}'

# 上传文档（异步：立即返回，后台完成解析/分块/向量化）
curl -X POST http://localhost:8083/api/v1/knowledge/{kb_id}/documents \
  -H "$AUTH" -F "file=@manual.pdf"

# 混合搜索
curl -X POST http://localhost:8083/api/v1/knowledge/{kb_id}/search \
  -H "Content-Type: application/json" -H "$AUTH" \
  -d '{"query": "如何重置密码", "top_k": 5, "min_score": 0.01}'

# 列出知识库
curl http://localhost:8083/api/v1/knowledge?page=1&page_size=20 -H "$AUTH"

# 删除文档
curl -X DELETE http://localhost:8083/api/v1/knowledge/{kb_id}/documents/{doc_id} -H "$AUTH"
```

## 配置

### 环境变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `DATABASE_URL` | — | PostgreSQL 连接字符串（必填） |
| `SKIP_MIGRATIONS` | `false` | 跳过启动时的内嵌 SQL 迁移 |
| `REDIS_URL` | — | Redis URL（可选；为空或连接失败时使用内存缓存） |
| `SEARCH_CACHE_TTL` | `60s` | 搜索结果缓存时长 |
| `STORAGE_TYPE` | `local` | 存储后端：`local`、`minio` |
| `STORAGE_BUCKET` | `knowledge-docs` | 存储桶名称 |
| `STORAGE_LOCAL_PATH` | `./data/docs` | 本地存储路径 |
| `STORAGE_ENDPOINT` | — | MinIO/S3 端点 |
| `STORAGE_ACCESS_KEY` | — | MinIO/S3 Access Key |
| `STORAGE_SECRET_KEY` | — | MinIO/S3 Secret Key |
| `STORAGE_USE_SSL` | `false` | MinIO/S3 是否使用 TLS |
| `EMBEDDING_PROVIDER` | `openai` | Embedding 提供商：`openai`、`gemini`、`ollama` |
| `EMBEDDING_API_KEY` | — | Embedding API Key（本地 OpenAI 兼容端点可为空） |
| `EMBEDDING_MODEL` | — | 模型名称（为空时使用提供商默认值） |
| `EMBEDDING_BASE_URL` | — | 自定义端点 URL（用于 Ollama 或兼容 API） |
| `EMBEDDING_DIMENSIONS` | 模型默认值 | 期望向量维度；数据库列按此解析 |
| `PARSER_TYPE` | `tika` | `tika` 或 `native`（仅纯文本格式） |
| `TIKA_ENDPOINT` | `http://localhost:9998` | Apache Tika 服务地址 |
| `TIKA_TIMEOUT` | `60s` | 单文档 Tika 请求超时 |
| `TIKA_DISABLE_NATIVE_FALLBACK` | `false` | Tika 不可用时直接报错，而不是本地解析 txt/md/csv/html/代码 |
| `AUTH_TYPE` | 自动推断 | `jwt`、`apikey` 或 `noop`；由 `JWT_SECRET`/`API_KEY` 推断 |
| `JWT_SECRET` | — | HS256 密钥；未设置 `AUTH_TYPE` 时启用 JWT |
| `JWT_ISSUER` | `knowledge-engine` | JWT 签发者校验（显式留空则关闭校验） |
| `API_KEY` | — | 静态 API Key；未设置 `AUTH_TYPE` 时启用 API Key 认证 |
| `API_KEY_USER_ID` | `00000000-0000-0000-0000-000000000001` | API Key 请求归属的 UUID |
| `DEFAULT_USER_ID` | `00000000-0000-0000-0000-000000000001` | 无认证模式下请求归属的 UUID |
| `SERVER_ADDRESS` | `:8083` | HTTP 监听地址 |
| `SERVER_READ_TIMEOUT` | `30s` | HTTP 读超时 |
| `SERVER_WRITE_TIMEOUT` | `300s` | HTTP 写超时（大文件上传） |

### Embedding 提供商

| 提供商 | 默认模型 | 默认维度 | 批量支持 |
|--------|---------|---------|---------|
| `openai` | `text-embedding-3-small` | 1536 | 支持（2048/批） |
| `gemini` | `gemini-embedding-001` | 768 | 不支持（顺序处理） |
| `ollama` | `nomic-embed-text` | 768 | 支持（OpenAI 兼容） |

OpenAI `text-embedding-3-*` 模型在设置 `EMBEDDING_DIMENSIONS` 时会同时发送 `dimensions`
参数，因此无需换模型也能使用 512 维等自定义维度。

### 向量维度与 Schema

迁移会创建 `embedding vector(1536)` 和 `knowledge_bases.embedding_dimensions`。
启动时引擎读取 Embedding 模型的维度并：

- 维度为 1536 时复用 `embedding` 列；
- 其他维度创建 `embedding_<dims>`（如 `embedding_768`、`embedding_384`）及 HNSW 索引。

因此同一个数据库可以服务多个模型，从 OpenAI 切到 Gemini/Ollama 不需要改表结构。
超过 2000 维（pgvector 限制）时跳过 HNSW 索引，检索仍可走精确扫描。
知识库会记录创建时的模型和维度，入库/检索时维度不匹配会直接报错，而不是静默写入错误长度的向量。

### 认证

| 模式 | 请求凭据 | 身份 |
|------|---------|------|
| `noop` | 无 | `DEFAULT_USER_ID`（仅开发/试用） |
| `apikey` | `X-API-Key: <API_KEY>` 或 `Authorization: Bearer <API_KEY>` | `API_KEY_USER_ID` |
| `jwt` | `Authorization: Bearer <JWT>`（HS256，`user_id` 或 `sub`，必须是 UUID） | token 主体 |

`AUTH_TYPE` 为空时：设置 `API_KEY` 即选 API Key 模式，否则设置 `JWT_SECRET` 选 JWT，否则 noop。
JWT 必须用 `JWT_SECRET` 签名，`user_id`（或 `sub`）必须是 UUID，签发者需匹配 `JWT_ISSUER`
（默认 `knowledge-engine`）；token 带 `exp` 时会校验有效期。
所有知识库和文档都按认证用户隔离，跨用户访问返回 `403`/`404`。

## API 参考

### 知识库

| 方法 | 端点 | 说明 |
|------|------|------|
| `GET` | `/api/v1/knowledge` | 列出知识库（分页） |
| `POST` | `/api/v1/knowledge` | 创建知识库 |
| `GET` | `/api/v1/knowledge/{id}` | 获取知识库详情 |
| `PATCH` | `/api/v1/knowledge/{id}` | 更新知识库 |
| `DELETE` | `/api/v1/knowledge/{id}` | 删除知识库 |

### 文档

| 方法 | 端点 | 说明 |
|------|------|------|
| `GET` | `/api/v1/knowledge/{id}/documents` | 列出文档（分页） |
| `POST` | `/api/v1/knowledge/{id}/documents` | 上传文档（multipart） |
| `DELETE` | `/api/v1/knowledge/{id}/documents/{doc_id}` | 删除文档 |

### 搜索

| 方法 | 端点 | 说明 |
|------|------|------|
| `POST` | `/api/v1/knowledge/{id}/search` | 混合搜索 |

`chunk_size`（100–8192，默认 512）和 `chunk_overlap`（默认 50，必须小于 `chunk_size`）保存在每个知识库上，
并对入库的每个文档生效。文档解析/分块/向量化是异步的，可轮询文档状态直到 `ready` 或 `failed`。

**搜索请求体：**
```json
{
  "query": "搜索文本",
  "top_k": 10,
  "min_score": 0.01
}
```

**搜索响应：**
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

`score` 为归一化后的 RRF 混合分（`0..1`），因此 `min_score` 在纯向量、纯关键词和混合结果下语义一致。
搜索结果按 KB generation 缓存，入库、删除文档或更新 KB 后自动失效。

## 集成指南

### 与 OmniDev 平台集成

OmniDev Gateway 通过 Go SDK 方式集成 knowledge-engine：

```go
// gateway 的 main.go
knowledgeEngine, err := knowledge.NewFromPool(db.Pool, emb, nil, nil)
if err != nil {
    return err
}
kbSvc := knowledgeEngine.KnowledgeBaseService()
searchSvc := knowledgeEngine.SearchService()
```

### 与任意 Go 应用集成

```go
import knowledge "github.com/omnidev/knowledge-engine"

// 方式 A：完整独立引擎
engine, _ := knowledge.New(knowledge.Config{...})

// 方式 B：从已有数据库连接池创建（执行迁移、解析向量列）
engine, err := knowledge.NewFromPool(pool, embedder, storage, logger)
if err != nil {
    return err
}

// 直接使用服务
kbSvc := engine.KnowledgeBaseService()
searchSvc := engine.SearchService()
```

### 与非 Go 应用集成

将 knowledge-engine 作为 sidecar 服务运行，通过 HTTP API 调用：

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

## 开发

```bash
# 单元测试（Tika、Embedding、认证、分块、检索）
go test ./...

# 集成测试需要 pgvector 数据库和 Redis
docker run --rm -d --name ke-test-pg -p 55432:5432 \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=knowledge_test \
  pgvector/pgvector:pg16

TEST_DATABASE_URL='postgres://postgres:postgres@localhost:55432/knowledge_test?sslmode=disable' \
TEST_REDIS_URL='redis://localhost:6379/0' \
go test ./internal/integration -v -count=1

# 构建并本地运行
go build -o knowledge-engine ./cmd/server
export DATABASE_URL=postgres://...
export EMBEDDING_API_KEY=sk-...
export TIKA_ENDPOINT=http://localhost:9998   # 或 PARSER_TYPE=native
./knowledge-engine
```

## 项目结构

```
knowledge-engine/
├── engine.go              # 入口：New() + NewFromPool()
├── interfaces.go          # 公共 API 导出（类型别名）
├── config.go              # 配置结构体
├── errors.go              # 错误类型
├── types/                 # 共享接口（避免循环引用）
├── domain/                # 核心实体
│   ├── knowledge.go       # 知识库、文档、分块
│   ├── search.go          # 搜索结果、搜索请求
│   ├── permission.go      # RBAC 实体
│   └── template.go        # 行业模板
├── internal/              # 业务逻辑（外部不可导入）
│   ├── chunker/           # 文本分块策略
│   ├── parser/            # Apache Tika 客户端 + 纯文本本地降级
│   ├── embedder/          # Embedding 提供商（注册表模式）
│   ├── retriever/         # 混合检索（向量 + BM25 + RRF）
│   └── service/           # KB、搜索、权限服务
├── adapters/              # 可插拔后端
│   ├── database/postgres/ # PostgreSQL + pgvector
│   ├── cache/redis/       # Redis 缓存
│   ├── cache/memory/      # 内存缓存（开发/测试）
│   ├── storage/minio/     # MinIO/S3 存储
│   ├── storage/local/     # 本地文件系统
│   ├── auth/jwt/          # JWT 认证
│   ├── auth/apikey/       # 静态 API Key 认证
│   └── auth/noop/         # 无认证（开发模式）
├── transport/http/        # HTTP 服务器（chi 路由）
├── cmd/server/            # 独立服务入口
├── migrations/            # 内嵌 SQL 迁移（含 advisory-lock runner）
└── deploy/docker/         # Docker + Compose 配置
```

## License

MIT
