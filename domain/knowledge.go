// Package domain defines the core business entities for the Knowledge Engine.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// DocumentStatus represents the processing status of a document.
type DocumentStatus string

const (
	DocumentStatusUploading  DocumentStatus = "uploading"
	DocumentStatusProcessing DocumentStatus = "processing"
	DocumentStatusReady      DocumentStatus = "ready"
	DocumentStatusFailed     DocumentStatus = "failed"
)

// KnowledgeBase represents a collection of documents.
type KnowledgeBase struct {
	ID                  uuid.UUID              `json:"id" db:"id"`
	UserID              uuid.UUID              `json:"user_id" db:"user_id"`
	OrgID               *uuid.UUID             `json:"org_id,omitempty" db:"org_id"`
	Name                string                 `json:"name" db:"name"`
	Description         *string                `json:"description,omitempty" db:"description"`
	TemplateID          *uuid.UUID             `json:"template_id,omitempty" db:"template_id"`
	EmbeddingModel      string                 `json:"embedding_model" db:"embedding_model"`
	EmbeddingDimensions int                    `json:"embedding_dimensions" db:"embedding_dimensions"`
	ChunkSize           int                    `json:"chunk_size" db:"chunk_size"`
	ChunkOverlap        int                    `json:"chunk_overlap" db:"chunk_overlap"`
	DocCount            int                    `json:"doc_count" db:"doc_count"`
	ChunkCount          int                    `json:"chunk_count" db:"chunk_count"`
	TotalTokens         int64                  `json:"total_tokens" db:"total_tokens"`
	TotalSize           int64                  `json:"total_size" db:"total_size"`
	Settings            map[string]interface{} `json:"settings" db:"settings"`
	Status              string                 `json:"status" db:"status"`
	CreatedAt           time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at" db:"updated_at"`
	DeletedAt           *time.Time             `json:"-" db:"deleted_at"`
}

// Document represents an uploaded document.
type Document struct {
	ID              uuid.UUID              `json:"id" db:"id"`
	KnowledgeBaseID uuid.UUID              `json:"knowledge_base_id" db:"knowledge_base_id"`
	Filename        string                 `json:"filename" db:"filename"`
	FileType        string                 `json:"file_type" db:"file_type"`
	FileSize        int64                  `json:"file_size" db:"file_size"`
	FileURL         string                 `json:"file_url" db:"file_url"`
	Status          DocumentStatus         `json:"status" db:"status"`
	Error           *string                `json:"error,omitempty" db:"error"`
	ChunkCount      int                    `json:"chunk_count" db:"chunk_count"`
	TotalTokens     int64                  `json:"total_tokens" db:"total_tokens"`
	Tags            []string               `json:"tags,omitempty" db:"tags"`
	Metadata        map[string]interface{} `json:"metadata" db:"metadata"`
	ProcessedAt     *time.Time             `json:"processed_at,omitempty" db:"processed_at"`
	CreatedAt       time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at" db:"updated_at"`
	DeletedAt       *time.Time             `json:"-" db:"deleted_at"`
}

// DocumentChunk represents a chunk of a document.
type DocumentChunk struct {
	ID              uuid.UUID              `json:"id" db:"id"`
	DocumentID      uuid.UUID              `json:"document_id" db:"document_id"`
	KnowledgeBaseID uuid.UUID              `json:"knowledge_base_id" db:"knowledge_base_id"`
	ChunkIndex      int                    `json:"chunk_index" db:"chunk_index"`
	Content         string                 `json:"content" db:"content"`
	ContentLength   int                    `json:"content_length" db:"content_length"`
	TokenCount      int                    `json:"token_count" db:"token_count"`
	StartPage       *int                   `json:"start_page,omitempty" db:"start_page"`
	EndPage         *int                   `json:"end_page,omitempty" db:"end_page"`
	Heading         *string                `json:"heading,omitempty" db:"heading"`
	Metadata        map[string]interface{} `json:"metadata" db:"metadata"`
	Embedding       []float32              `json:"-" db:"embedding"`
	CreatedAt       time.Time              `json:"created_at" db:"created_at"`
}
