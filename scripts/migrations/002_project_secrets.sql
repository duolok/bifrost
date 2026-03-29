CREATE TABLE IF NOT EXISTS project_secrets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    key_name    TEXT NOT NULL,
    secret_ref  TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, key_name)
);

CREATE INDEX IF NOT EXISTS idx_project_secrets_project_id ON project_secrets(project_id);
