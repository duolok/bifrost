package api

const (
	resourceProject    = "project"
	resourceDeployment = "deployment"

	auditProjectCreated     = "project.created"
	auditProjectDeleted     = "project.deleted"
	auditDeployTriggered    = "deployment.triggered"
	auditWebhookReceived    = "webhook.received"

	triggerAPI            = "api"
	triggerGitHubWebhook  = "github-webhook"

	pgUniqueViolation = "23505"
)
