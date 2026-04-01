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
	auditDeployRetried   = "deployment.retried"
	auditDeployRollback  = "deployment.rollback"
	auditUserRegister    = "user.registered"
	auditAPIKeyCreated   = "apikey.created"
	auditAPIKeyRevoked   = "apikey.revoked"
	auditTeamInvite      = "team.invite"
	auditTeamRemove      = "team.remove"

	triggerAPI           = "api"
	triggerRollback      = "rollback"
	triggerGitHubWebhook = "github-webhook"

	pgUniqueViolation = "23505"

	defaultRulesScript = `
		if unhealthy_count >= 3 then
			alert("critical", project .. " is unhealthy (" .. unhealthy_count .. " consecutive failures)", "admin@bifrost.dev")
		end

		if cpu_percent > 90 then
			alert("warning", project .. " CPU at " .. string.format("%.1f", cpu_percent) .. "%", "admin@bifrost.dev")
		end

		if response_time_ms > 1000 then
			alert("warning", project .. " slow response: " .. response_time_ms .. "ms", "admin@bifrost.dev")
		end
		`
)
