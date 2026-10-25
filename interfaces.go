// Package knowledge provides a pluggable, embeddable knowledge engine.
//
// All external dependencies are abstracted via interfaces.
// Shared types are in the types sub-package to avoid import cycles.
package knowledge

import (
	"github.com/omnidev/knowledge-engine/domain"
	"github.com/omnidev/knowledge-engine/internal/embedder"
	"github.com/omnidev/knowledge-engine/internal/service"
	"github.com/omnidev/knowledge-engine/types"
)

// Re-export shared types for convenience.
type (
	Database      = types.Database
	Cache         = types.Cache
	Storage       = types.Storage
	Authenticator = types.Authenticator
	Logger        = types.Logger
	UserInfo      = types.UserInfo
)

// Re-export service types for external consumers.
type (
	KnowledgeBaseService = service.KnowledgeBaseService
	SearchService        = service.SearchService
	CreateKBInput        = service.CreateKBInput
	SearchInput          = service.SearchInput
)

// Re-export domain entities for external consumers.
type (
	KnowledgeBase = domain.KnowledgeBase
	Document      = domain.Document
	DocumentChunk = domain.DocumentChunk
	SearchResult  = domain.SearchResult
)

// Re-export embedder types for external consumers.
type (
	EmbedderConfig = embedder.EmbedderConfig
	Embedder       = embedder.Embedder
)

// NewEmbedder creates a new embedder from config.
var NewEmbedder = embedder.New
