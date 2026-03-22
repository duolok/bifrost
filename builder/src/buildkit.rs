use anyhow::{Context, Result};
use google_cloud_auth::credentials::{AccessTokenCredentials, Builder};
use serde::{Deserialize, Serialize};
use std::path::Path;
use std::time::Duration;
use tokio::process::Command;
use crate::realtime::RealtimeClient;

/// Submits a build to Cloud Build API using a source tarball uploaded to GCS.
/// The caller must have already cloned the repo into context_dir.
pub async fn build_and_push(
    gcp_project: &str,
    context_dir: &Path,
    _repo_url: &str,
    commit_sha: &str,
    image_uri: &str,
    deploy_id: &str,
    mut rt: Option<&mut RealtimeClient>,
) -> Result<()> {
    let creds: AccessTokenCredentials = Builder::default()
        .with_scopes(["https://www.googleapis.com/auth/cloud-platform"])
        .build_access_token_credentials()
        .context("failed to build credentials")?;

    let client = reqwest::Client::new();
    let token = creds.access_token().await.context("failed to get token")?;

    // Tar up the build context
    let tarball = context_dir.with_extension("tar.gz");
    let tar_output = Command::new("tar")
        .args([
            "-czf",
            tarball.to_str().unwrap(),
            "-C",
            context_dir.to_str().unwrap(),
            ".",
        ])
        .output()
        .await
        .context("failed to run tar")?;

    if !tar_output.status.success() {
        let stderr = String::from_utf8_lossy(&tar_output.stderr);
        anyhow::bail!("tar failed: {}", stderr);
    }

    let tar_bytes = tokio::fs::read(&tarball).await
        .context("failed to read tarball")?;
    let _ = tokio::fs::remove_file(&tarball).await;

    // Upload tarball to GCS
    let bucket = format!("{}_cloudbuild", gcp_project);
    let object_name = format!("source/{}.tar.gz", commit_sha);
    let upload_url = format!(
        "https://storage.googleapis.com/upload/storage/v1/b/{}/o?uploadType=media&name={}",
        bucket, object_name
    );

    let resp = client
        .post(&upload_url)
        .bearer_auth(token.token.as_str())
        .header("Content-Type", "application/gzip")
        .body(tar_bytes)
        .send()
        .await
        .context("failed to upload source to GCS")?;

    if !resp.status().is_success() {
        let status = resp.status();
        let body = resp.text().await.unwrap_or_default();
        anyhow::bail!("GCS upload failed ({}): {}", status, body);
    }

    tracing::info!(bucket = %bucket, object = %object_name, "source uploaded to GCS");

    // Submit build to Cloud Build
    let build_request = CloudBuildRequest {
        source: Source {
            storage_source: Some(StorageSource {
                bucket: bucket.clone(),
                object: object_name,
            }),
        },
        steps: vec![BuildStep {
            name: "gcr.io/cloud-builders/docker".to_string(),
            args: vec![
                "build".to_string(),
                "-t".to_string(),
                image_uri.to_string(),
                ".".to_string(),
            ],
        }],
        images: vec![image_uri.to_string()],
    };

    let url = format!(
        "https://cloudbuild.googleapis.com/v1/projects/{}/builds",
        gcp_project
    );

    let token = creds.access_token().await.context("failed to get token")?;

    let resp = client
        .post(&url)
        .bearer_auth(token.token.as_str())
        .json(&build_request)
        .send()
        .await
        .context("failed to submit Cloud Build request")?;

    if !resp.status().is_success() {
        let status = resp.status();
        let body = resp.text().await.unwrap_or_default();
        anyhow::bail!("Cloud Build submission failed ({}): {}", status, body);
    }

    let op: OperationResponse = resp.json().await
        .context("failed to parse Cloud Build response")?;

    let build_id = op.metadata.build.id;
    tracing::info!(build_id = %build_id, "cloud build submitted, polling for completion");

    // Poll until build completes
    let build_url = format!(
        "https://cloudbuild.googleapis.com/v1/projects/{}/builds/{}",
        gcp_project, build_id
    );

    loop {
        tokio::time::sleep(Duration::from_secs(10)).await;

        let token = creds.access_token().await.context("failed to refresh token")?;

        let resp = client
            .get(&build_url)
            .bearer_auth(token.token.as_str())
            .send()
            .await
            .context("failed to poll Cloud Build status")?;

        let build: BuildStatusResponse = resp.json().await
            .context("failed to parse build status")?;

        if let Some(ref mut client) = rt {
              client.send_log(deploy_id, format!("Build status: {}", build.status)).await;
          }

        tracing::info!(build_id = %build_id, status = %build.status, "build status");

        match build.status.as_str() {
            "SUCCESS" => return Ok(()),
            "FAILURE" | "INTERNAL_ERROR" | "TIMEOUT" | "CANCELLED" | "EXPIRED" => {
                let msg = build.status_detail.unwrap_or_else(|| build.status.clone());
                anyhow::bail!("Cloud Build failed: {}", msg);
            }
            _ => continue,
        }
    }
}

#[derive(Serialize)]
struct CloudBuildRequest {
    source: Source,
    steps: Vec<BuildStep>,
    images: Vec<String>,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct Source {
    #[serde(skip_serializing_if = "Option::is_none")]
    storage_source: Option<StorageSource>,
}

#[derive(Serialize)]
struct StorageSource {
    bucket: String,
    object: String,
}

#[derive(Serialize)]
struct BuildStep {
    name: String,
    args: Vec<String>,
}

#[derive(Deserialize)]
struct OperationResponse {
    metadata: OperationMetadata,
}

#[derive(Deserialize)]
struct OperationMetadata {
    build: BuildRef,
}

#[derive(Deserialize)]
struct BuildRef {
    id: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct BuildStatusResponse {
    status: String,
    status_detail: Option<String>,
}
