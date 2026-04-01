#![allow(dead_code)]

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};

pub struct BifrostClient {
    http: reqwest::Client,
    base_url: String,
    token: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct Project {
    pub id: String,
    pub name: String,
    pub repo_url: String,
    pub default_branch: String,
    pub status: String,
    pub created_at: String,
    pub webhook_secret: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct Deployment {
    pub id: String,
    pub project_id: String,
    pub commit_sha: String,
    pub status: String,
    pub triggered_by: String,
    pub created_at: String,
}

#[derive(Deserialize)]
struct ProjectsResponse {
    projects: Vec<Project>,
}

#[derive(Deserialize)]
struct DeploymentsResponse {
    deployments: Vec<Deployment>,
}

#[derive(Serialize)]
struct DeployRequest {
    commit_sha: String,
}

#[derive(Debug, Deserialize)]
pub struct RollbackResponse {
    pub status: String,
    pub deployment_id: String,
    pub rolled_back_to: String,
    pub previous_deploy: String,
}

#[derive(Debug, Deserialize)]
pub struct Secret {
    pub id: String,
    pub project_id: String,
    pub key_name: String,
    pub secret_ref: String,
    pub created_at: String,
}

#[derive(Deserialize)]
struct SecretsResponse {
    secrets: Vec<Secret>,
}

#[derive(Serialize)]
struct SetSecretRequest {
    key_name: String,
    secret_ref: String,
}

#[derive(Serialize)]
struct CreateProjectRequest {
    name: String,
    repo_url: String,
}

#[derive(Serialize)]
struct LoginRequest {
    email: String,
    password: String,
}

#[derive(Serialize)]
struct RegisterRequest {
    email: String,
    password: String,
    name: String,
}

#[derive(Debug, Deserialize)]
pub struct LoginResponse {
    pub token: String,
    pub user_id: String,
    pub team_id: String,
    pub role: String,
}

#[derive(Debug, Deserialize)]
pub struct MeResponse {
    pub user: MeUser,
    pub team: MeTeam,
    pub role: String,
}

#[derive(Debug, Deserialize)]
pub struct MeUser {
    pub id: String,
    pub email: String,
    pub name: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct MeTeam {
    pub id: String,
    pub name: String,
}

impl BifrostClient {
    pub fn new(base_url: &str, token: Option<String>) -> Self {
        Self {
            http: reqwest::Client::new(),
            base_url: base_url.trim_end_matches('/').to_string(),
            token,
        }
    }

    fn auth(&self, req: reqwest::RequestBuilder) -> reqwest::RequestBuilder {
        match &self.token {
            Some(t) => req.bearer_auth(t),
            None => req,
        }
    }

    pub async fn login(&self, email: &str, password: &str) -> Result<LoginResponse> {
        let resp = self.http
            .post(format!("{}/api/v1/auth/login", self.base_url))
            .json(&LoginRequest {
                email: email.to_string(),
                password: password.to_string(),
            })
            .send().await
            .context("failed to reach gateway")?;
        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            anyhow::bail!("login failed ({}): {}", status, body);
        }
        Ok(resp.json::<LoginResponse>().await?)
    }

    pub async fn register(&self, email: &str, password: &str, name: &str) -> Result<LoginResponse> {
        let resp = self.http
            .post(format!("{}/api/v1/auth/register", self.base_url))
            .json(&RegisterRequest {
                email: email.to_string(),
                password: password.to_string(),
                name: name.to_string(),
            })
            .send().await
            .context("failed to reach gateway")?;
        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            anyhow::bail!("registration failed ({}): {}", status, body);
        }
        Ok(resp.json::<LoginResponse>().await?)
    }

    pub async fn get_me(&self) -> Result<MeResponse> {
        let req = self.http.get(format!("{}/api/v1/auth/me", self.base_url));
        let resp = self.auth(req)
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<MeResponse>().await?;
        Ok(resp)
    }

    pub async fn create_project(&self, name: &str, repo_url: &str) -> Result<Project> {
        let req = self.http
            .post(format!("{}/api/v1/project", self.base_url))
            .json(&CreateProjectRequest {
                name: name.to_string(),
                repo_url: repo_url.to_string(),
            });
        let resp = self.auth(req)
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<Project>().await?;
        Ok(resp)
    }

    pub async fn delete_project(&self, id: &str) -> Result<()> {
        let req = self.http
            .delete(format!("{}/api/v1/project/{}", self.base_url, id));
        self.auth(req)
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?;
        Ok(())
    }

    pub async fn list_projects(&self) -> Result<Vec<Project>> {
        let req = self.http
            .get(format!("{}/api/v1/projects", self.base_url));
        let resp = self.auth(req)
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<ProjectsResponse>().await?;
        Ok(resp.projects)
    }

    pub async fn get_project(&self, id: &str) -> Result<Project> {
        let req = self.http
            .get(format!("{}/api/v1/project/{}", self.base_url, id));
        let resp = self.auth(req)
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<Project>().await?;
        Ok(resp)
    }

    pub async fn trigger_deploy(&self, project_id: &str, commit_sha: &str) -> Result<Deployment> {
        let req = self.http
            .post(format!("{}/api/v1/projects/{}/deploy", self.base_url, project_id))
            .json(&DeployRequest { commit_sha: commit_sha.to_string() });
        let resp = self.auth(req)
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<Deployment>().await?;
        Ok(resp)
    }

    pub async fn get_deployment(&self, id: &str) -> Result<Deployment> {
        let req = self.http
            .get(format!("{}/api/v1/deployments/{}", self.base_url, id));
        let resp = self.auth(req)
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<Deployment>().await?;
        Ok(resp)
    }

    pub async fn rollback(&self, project_id: &str) -> Result<RollbackResponse> {
        let req = self.http
            .post(format!("{}/api/v1/projects/{}/rollback", self.base_url, project_id));
        let resp = self.auth(req)
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<RollbackResponse>().await?;
        Ok(resp)
    }

    pub async fn list_secrets(&self, project_id: &str) -> Result<Vec<Secret>> {
        let req = self.http
            .get(format!("{}/api/v1/projects/{}/secrets", self.base_url, project_id));
        let resp = self.auth(req)
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<SecretsResponse>().await?;
        Ok(resp.secrets)
    }

    pub async fn set_secret(&self, project_id: &str, key_name: &str, secret_ref: &str) -> Result<Secret> {
        let req = self.http
            .post(format!("{}/api/v1/projects/{}/secrets", self.base_url, project_id))
            .json(&SetSecretRequest {
                key_name: key_name.to_string(),
                secret_ref: secret_ref.to_string(),
            });
        let resp = self.auth(req)
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<Secret>().await?;
        Ok(resp)
    }

    pub async fn delete_secret(&self, project_id: &str, key_name: &str) -> Result<()> {
        let req = self.http
            .delete(format!("{}/api/v1/projects/{}/secrets/{}", self.base_url, project_id, key_name));
        self.auth(req)
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?;
        Ok(())
    }

    pub async fn list_deployments(&self, project_id: &str) -> Result<Vec<Deployment>> {
        let req = self.http
            .get(format!("{}/api/v1/projects/{}/deployments", self.base_url, project_id));
        let resp = self.auth(req)
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<DeploymentsResponse>().await?;
        Ok(resp.deployments)
    }
}
