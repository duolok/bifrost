package models

import (
	"encoding/json"
	"time"

	"github.com/!azure/azure-sdk-for-go/sdk/internal/uuid"
	"github.com/google/uuid"
)

type DeploymentStatus string

const (
	StatusQueued     DeploymentStatus = "queued"
	StatusValidating DeploymentStatus = "validating"
	StatusBuilding   DeploymentStatus = "building"
	StatusBuilt      DeploymentStatus = "built"
	StatusDeploying  DeploymentStatus = "deploying"
	StatusRunning    DeploymentStatus = "running"
	StatusHealthy    DeploymentStatus = "healthy"
	StatusFailed     DeploymentStatus = "failed"
)

type Deployment struct {
	ID               uuid.UUID        `json:"id"`
	ProjectID        uuid.UUID        `json:"project_id"`
	CommitSHA        string           `json:"commit_sha"`
	Branch           *string          `json:"branch,omitempty"`
	TriggeredBy      *string          `json:"triggered_by,omitempty"`
	ImageURI         *string          `json:"image_uri,omitempty"`
	Status           DeploymentStatus `json:"status"`
	StatusMessage    *string          `json:"status_message,omitempty"`
	ConfigSnapshot   *json.RawMessage `json:"config_snapshot,omitempty"`
	BuildStartedAt   *time.Time       `json:"build_started_at,omitempty"`
	BuildFinishedAt  *time.Time       `json:"build_finished_at,omitempty"`
	DeployStartedAt  *time.Time       `json:"deploy_started_at,omitempty"`
	DeployFinishedAt *time.Time       `json:"deploy_finished_at,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
}

var validTransitions = map[DeploymentStatus][]DeploymentStatus{
	StatusQueued:     {StatusValidating, StatusFailed},
	StatusValidating: {StatusBuilding, StatusFailed},
	StatusBuilding:   {StatusBuilt, StatusFailed},
	StatusBuilt:      {StatusDeploying, StatusFailed},
	StatusDeploying:  {StatusRunning, StatusFailed},
	StatusRunning:    {StatusHealthy, StatusFailed},
	StatusHealthy:    {StatusDeploying, StatusFailed},
	StatusFailed:     {StatusQueued},
}

type TriggerDeployRequest struct {
	CommitSHA string `json:"commit_sha" binding:"required,min=7,max=40"`
	Branch    string `json:"branch"`
}

