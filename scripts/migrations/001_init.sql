CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    repo_url VARCHAR(512) NOT NULL,
    default_branch VARCHAR(255) DEFAULT 'main',
    webhook_secret VARCHAR(255),
    config JSONB,
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    commit_sha VARCHAR(40) NOT NULL,
    branch VARCHAR(255),
    triggered_by VARCHAR(255),
    image_uri VARCHAR(512),
    status VARCHAR(30) DEFAULT 'queued',
    status_message TEXT,
    config_snapshot JSONB,
    build_started_at TIMESTAMPTZ,
    build_finished_at TIMESTAMPTZ,
    deploy_started_at TIMESTAMPTZ,
    deploy_finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(project_id, commit_sha)
);

CREATE TABLE health_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL,
    http_status INT,
    response_time_ms INT,
    cpu_percent REAL,
    memory_bytes BIGINT,
    memory_percent REAL,
    fd_count INT,
    checked_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor VARCHAR(255),
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50),
    resource_id UUID,
    details JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_deployments_project_id ON deployments(project_id);
CREATE INDEX idx_deployments_status ON deployments(status);
CREATE INDEX idx_health_checks_deployment_id ON health_checks(deployment_id);
CREATE INDEX idx_audit_log_resource ON audit_log(resource_type, resource_id);
CREATE INDEX idx_projects_repo_url ON projects(repo_url);
