package domain

import (
	"time"

	"github.com/google/uuid"
)

// Template represents an industry-specific knowledge base template.
type Template struct {
	ID                   uuid.UUID              `json:"id" db:"id"`
	Slug                 string                 `json:"slug" db:"slug"`
	Name                 string                 `json:"name" db:"name"`
	Description          *string                `json:"description,omitempty" db:"description"`
	DefaultChunkSize     int                    `json:"default_chunk_size" db:"default_chunk_size"`
	DefaultChunkOverlap  int                    `json:"default_chunk_overlap" db:"default_chunk_overlap"`
	RecommendedEmbedding string                 `json:"recommended_embedding,omitempty" db:"recommended_embedding"`
	MetadataSchema       map[string]interface{} `json:"metadata_schema" db:"metadata_schema"`
	SupportedFileTypes   []string               `json:"supported_file_types" db:"supported_file_types"`
	ChunkEnhancements    map[string]interface{} `json:"chunk_enhancements" db:"chunk_enhancements"`
	IsSystem             bool                   `json:"is_system" db:"is_system"`
	Status               string                 `json:"status" db:"status"`
	CreatedAt            time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at" db:"updated_at"`
}
