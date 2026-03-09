package api

const (
	resourceProject    = "project"
	resourceDeployment = "deployment"

	auditProjectCreated  = "project.created"
	auditProjectDeleted  = "project.deleted"
	auditDeployTriggered = "deployment.triggered"
	auditWebhookReceived = "webhook.received"
	auditDeployComplete  = "deployment.deployed"
	auditDeployFailed    = "deployment.failed"

	triggerAPI           = "api"
	triggerGitHubWebhook = "github-webhook"

	pgUniqueViolation = "23505"
)
