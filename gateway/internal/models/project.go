package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ProjectStatus string

const (
	ProjectActive   ProjectStatus = "active"
	ProjectPaused   ProjectStatus = "paused"
	ProjectArchived ProjectStatus = "archived"
)

type Project struct {
	ID            uuid.UUID        `json:"id"`
	Name          string           `json:"name"`
	RepoURL       string           `json:"repo_url"`
	DefaultBranch string           `json:"default_branch"`
	WebhookSecret string           `json:"-"`
	Config        *json.RawMessage `json:"config,omitempty"`
	Status        ProjectStatus    `json:"status"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

type CreateProjectRequest struct {
	Name          string `json:"name" binding:"required,min=1,max=255"`
	RepoURL       string `json:"repo_url" binding:"required,url"`
	DefaultBranch string `json:"default_branch"`
}
