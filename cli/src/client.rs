use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};

pub struct BifrostClient {
    http: reqwest::Client,
    base_url: String,
}

#[derive(Debug, Deserialize)]
pub struct Project {
    pub id: String,
    pub name: String,
    pub repo_url: String,
    pub default_branch: String,
    pub status: String,
    pub created_at: String,
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

impl BifrostClient {
    pub fn new(base_url: &str) -> Self {
        Self {
            http: reqwest::Client::new(),
            base_url: base_url.trim_end_matches('/').to_string(),
        }
    }

    pub async fn list_projects(&self) -> Result<Vec<Project>> {
        let resp = self.http
            .get(format!("{}/api/v1/projects", self.base_url))
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<ProjectsResponse>().await?;
        Ok(resp.projects)
    }

    pub async fn get_project(&self, id: &str) -> Result<Project> {
        let resp = self.http
            .get(format!("{}/api/v1/project/{}", self.base_url, id))
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<Project>().await?;
        Ok(resp)
    }

    pub async fn trigger_deploy(&self, project_id: &str, commit_sha: &str) -> Result<Deployment> {
        let resp = self.http
            .post(format!("{}/api/v1/projects/{}/deploy", self.base_url, project_id))
            .json(&DeployRequest { commit_sha: commit_sha.to_string() })
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<Deployment>().await?;
        Ok(resp)
    }

    pub async fn get_deployment(&self, id: &str) -> Result<Deployment> {
        let resp = self.http
            .get(format!("{}/api/v1/deployments/{}", self.base_url, id))
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<Deployment>().await?;
        Ok(resp)
    }

    pub async fn list_deployments(&self, project_id: &str) -> Result<Vec<Deployment>> {
        let resp = self.http
            .get(format!("{}/api/v1/projects/{}/deployments", self.base_url, project_id))
            .send().await
            .context("failed to reach gateway")?
            .error_for_status()?
            .json::<DeploymentsResponse>().await?;
        Ok(resp.deployments)
    }
}
