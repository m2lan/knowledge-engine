package domain

import (
	"time"

	"github.com/google/uuid"
)

// PermissionRole represents the permission level.
type PermissionRole string

const (
	PermissionRoleViewer PermissionRole = "viewer"
	PermissionRoleEditor PermissionRole = "editor"
	PermissionRoleAdmin  PermissionRole = "admin"
	PermissionRoleOwner  PermissionRole = "owner"
)

// PermissionSubject identifies who gets the permission.
type PermissionSubject string

const (
	SubjectUser PermissionSubject = "user"
	SubjectTeam PermissionSubject = "team"
	SubjectOrg  PermissionSubject = "org"
)

// Permission represents an access grant on a knowledge base.
type Permission struct {
	ID              uuid.UUID              `json:"id" db:"id"`
	KnowledgeBaseID uuid.UUID              `json:"knowledge_base_id" db:"knowledge_base_id"`
	UserID          *uuid.UUID             `json:"user_id,omitempty" db:"user_id"`
	TeamID          *uuid.UUID             `json:"team_id,omitempty" db:"team_id"`
	OrgID           *uuid.UUID             `json:"org_id,omitempty" db:"org_id"`
	Role            PermissionRole         `json:"role" db:"role"`
	Conditions      map[string]interface{} `json:"conditions,omitempty" db:"conditions"`
	GrantedBy       uuid.UUID              `json:"granted_by" db:"granted_by"`
	ExpiresAt       *time.Time             `json:"expires_at,omitempty" db:"expires_at"`
	CreatedAt       time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at" db:"updated_at"`
}

// PermissionAuditLog records permission changes.
type PermissionAuditLog struct {
	ID              uuid.UUID       `json:"id" db:"id"`
	KnowledgeBaseID uuid.UUID       `json:"knowledge_base_id" db:"knowledge_base_id"`
	ActorID         uuid.UUID       `json:"actor_id" db:"actor_id"`
	Action          string          `json:"action" db:"action"` // "grant", "revoke", "update"
	TargetType      string          `json:"target_type" db:"target_type"`
	TargetID        uuid.UUID       `json:"target_id" db:"target_id"`
	OldRole         *PermissionRole `json:"old_role,omitempty" db:"old_role"`
	NewRole         *PermissionRole `json:"new_role,omitempty" db:"new_role"`
	Details         map[string]interface{} `json:"details,omitempty" db:"details"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
}
