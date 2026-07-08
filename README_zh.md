# Knowledge Engine

一个可插拔、可嵌入的知识引擎，支持文档导入、分块、向量化和混合检索。设计为可作为独立服务、Go SDK 或嵌入式库集成到任何项目中。

## 特性

- **混合检索** — 向量相似度（pgvector）+ BM25 全文搜索，通过倒数排名融合（RRF）合并结果
- **中文支持** — 运行时自动检测 zhparser 扩展，支持中文全文搜索，自动降级到 `simple` 配置
- **多格式解析** — PDF、DOCX、PPTX、XLSX、CSV、HTML、JSON 及源代码（Go、Python、JS、TS、Java、Rust、C++ 等）
- **可插拔 Embedding** — 支持 OpenAI、Gemini、Ollama（OpenAI 兼容）；通过注册表模式轻松扩展
- **可插拔存储** — 本地文件系统、MinIO/S3；实现 `Storage` 接口即可自定义后端
- **可插拔认证** — JWT、无认证（开发模式）；实现 `Authenticator` 接口即可自定义认证
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
│             │             │  检索器      │  认证: JWT       │
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

# 启动所有服务（PostgreSQL + pgvector、Redis、Knowledge Engine）
docker-compose -f deploy/docker/docker-compose.yml up -d

# 验证服务是否运行
curl http://localhost:8083/health
```

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

// 从已有连接池创建引擎
engine := knowledge.NewFromPool(existingPool, emb, nil, nil)
defer engine.Close()

// 直接使用
results, _ := engine.Search(ctx, userID, kbID, "查询内容", 5)
```

### 4. HTTP API

```bash
# 健康检查
curl http://localhost:8083/health

# 创建知识库
curl -X POST http://localhost:8083/api/v1/knowledge \
  -H "Content-Type: application/json" \
  -d '{"name": "产品文档", "description": "主产品文档库"}'

# 上传文档
curl -X POST http://localhost:8083/api/v1/knowledge/{kb_id}/documents \
  -F "file=@manual.pdf"

# 混合搜索
curl -X POST http://localhost:8083/api/v1/knowledge/{kb_id}/search \
  -H "Content-Type: application/json" \
  -d '{"query": "如何重置密码", "top_k": 5, "min_score": 0.01}'

# 列出知识库
curl http://localhost:8083/api/v1/knowledge?page=1&page_size=20

# 删除文档
curl -X DELETE http://localhost:8083/api/v1/knowledge/{kb_id}/documents/{doc_id}
```

## 配置

### 环境变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `DATABASE_URL` | — | PostgreSQL 连接字符串（必填） |
| `REDIS_URL` | — | Redis URL（可选，为空时使用内存缓存） |
| `STORAGE_TYPE` | `local` | 存储后端：`local`、`minio` |
| `STORAGE_BUCKET` | `knowledge-docs` | 存储桶名称 |
| `STORAGE_LOCAL_PATH` | `./data/docs` | 本地存储路径 |
| `STORAGE_ENDPOINT` | — | MinIO/S3 端点 |
| `STORAGE_ACCESS_KEY` | — | MinIO/S3 Access Key |
| `STORAGE_SECRET_KEY` | — | MinIO/S3 Secret Key |
| `EMBEDDING_PROVIDER` | `openai` | Embedding 提供商：`openai`、`gemini`、`ollama` |
| `EMBEDDING_API_KEY` | — | Embedding API Key |
| `EMBEDDING_MODEL` | — | 模型名称（为空时使用提供商默认值） |
| `EMBEDDING_BASE_URL` | — | 自定义端点 URL（用于 Ollama 或兼容 API） |
| `JWT_SECRET` | — | JWT 密钥（为空时为无认证模式） |
| `JWT_ISSUER` | `knowledge-engine` | JWT 签发者验证 |
| `SERVER_ADDRESS` | `:8083` | HTTP 监听地址 |

### Embedding 提供商

| 提供商 | 默认模型 | 维度 | 批量支持 |
|--------|---------|------|---------|
| `openai` | `text-embedding-3-small` | 1536 | 支持（2048/批） |
| `gemini` | `gemini-embedding-2` | 768 | 不支持（顺序处理） |
| `ollama` | `nomic-embed-text` | 768 | 支持（OpenAI 兼容） |

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
      "score": 0.016,
      "source": "hybrid"
    }
  ]
}
```

## 集成指南

### 与 OmniDev 平台集成

OmniDev Gateway 通过 Go SDK 方式集成 knowledge-engine：

```go
// gateway 的 main.go
knowledgeEngine := knowledge.NewFromPool(db.Pool, emb, nil, nil)
kbSvc := knowledgeEngine.KnowledgeBaseService()
searchSvc := knowledgeEngine.SearchService()
```

### 与任意 Go 应用集成

```go
import knowledge "github.com/omnidev/knowledge-engine"

// 方式 A：完整独立引擎
engine, _ := knowledge.New(knowledge.Config{...})

// 方式 B：从已有数据库连接池创建
engine := knowledge.NewFromPool(pool, embedder, storage, logger)

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
# 运行测试
go test ./...

# 构建
go build -o knowledge-engine ./cmd/server

# 本地运行
export DATABASE_URL=postgres://...
export EMBEDDING_API_KEY=sk-...
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
│   ├── parser/            # 文档解析
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
│   └── auth/noop/         # 无认证（开发模式）
├── transport/http/        # HTTP 服务器（chi 路由）
├── cmd/server/            # 独立服务入口
├── migrations/            # SQL 迁移脚本
└── deploy/docker/         # Docker + Compose 配置
```

## License

MIT
