package models

import (
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleAdmin    Role = "admin"
	RoleDeployer Role = "deployer"
	RoleViewer   Role = "viewer"
)

// CanDeploy returns true if the role can trigger deploys, rollbacks, and manage secrets.
func (r Role) CanDeploy() bool {
	return r == RoleAdmin || r == RoleDeployer
}

// CanManage returns true if the role can manage team members, delete projects, etc.
func (r Role) CanManage() bool {
	return r == RoleAdmin
}

// MeetsMinimum checks if this role meets or exceeds the given minimum role.
func (r Role) MeetsMinimum(min Role) bool {
	order := map[Role]int{RoleViewer: 0, RoleDeployer: 1, RoleAdmin: 2}
	return order[r] >= order[min]
}

type Team struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TeamMember struct {
	ID        uuid.UUID `json:"id"`
	TeamID    uuid.UUID `json:"team_id"`
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email,omitempty"`
	Name      *string   `json:"name,omitempty"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}
