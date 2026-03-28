package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"io"
	"net/http"
	"strings"

	"duolok/bifrost/gateway/internal/errors"
	"duolok/bifrost/gateway/internal/models"
	"duolok/bifrost/gateway/pkg/apiutil"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type gitHubPushEvent struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		CloneURL string `json:"clone_url"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
	Pusher struct {
		Name string `json:"name"`
	} `json:"pusher"`
}

func (h *Handler) HandleGitHubWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		apiutil.RespondError(c, errors.InvalidInput("failed to read request body"))
		return
	}

	var event gitHubPushEvent
	if err := json.Unmarshal(body, &event); err != nil {
		apiutil.RespondError(c, errors.InvalidInput("invalid JSON payload"))
		return
	}

	repoURL := event.Repository.CloneURL
	var p models.Project
	err = h.pool.QueryRow(c.Request.Context(),
		`SELECT id, name, repo_url, default_branch, webhook_secret
		 FROM projects
		 WHERE (repo_url = $1 OR repo_url = $2) AND status = $3`,
		repoURL, strings.TrimSuffix(repoURL, ".git"), models.ProjectActive,
	).Scan(&p.ID, &p.Name, &p.RepoURL, &p.DefaultBranch, &p.WebhookSecret)

	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			apiutil.RespondError(c, errors.NotFound(resourceProject, repoURL))
			return
		}
		apiutil.RespondError(c, errors.Internal("failed to look up project", err))
		return
	}

	signature := c.GetHeader("X-Hub-Signature-256")
	if !verifyHMAC(p.WebhookSecret, body, signature) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	branch := strings.TrimPrefix(event.Ref, "refs/heads/")
	commitSHA := event.After
	triggeredBy := triggerGitHubWebhook
	if event.Pusher.Name != "" {
		triggeredBy = triggerGitHubWebhook + ":" + event.Pusher.Name
	}

	var d models.Deployment
	err = h.pool.QueryRow(c.Request.Context(),
		`INSERT INTO deployments (project_id, commit_sha, branch, triggered_by, status)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (project_id, commit_sha) DO NOTHING
		 RETURNING id, project_id, commit_sha, branch, triggered_by, status, created_at`,
		p.ID, commitSHA, branch, triggeredBy, models.StatusQueued,
	).Scan(&d.ID, &d.ProjectID, &d.CommitSHA, &d.Branch, &d.TriggeredBy, &d.Status, &d.CreatedAt)

	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusOK, gin.H{"status": "already_processed", "commit_sha": commitSHA})
			return
		}
		apiutil.RespondError(c, errors.Internal("failed to create deployment", err))
		return
	}

	h.audit(c, auditWebhookReceived, resourceProject, p.ID, gin.H{
		"commit_sha": commitSHA,
		"branch":     branch,
	})

	h.validateAndBuild(c, d.ID, p.Name, p.RepoURL, commitSHA)

	c.JSON(http.StatusOK, gin.H{
		"status":        "accepted",
		"deployment_id": d.ID,
	})
}

func verifyHMAC(secret string, body []byte, signatureHeader string) bool {
	if !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}

	sigHex := strings.TrimPrefix(signatureHeader, "sha256=")
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(sigBytes, expected)
}
