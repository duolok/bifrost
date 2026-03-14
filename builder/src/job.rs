use std::path::PathBuf;

use crate::config::Config;
use crate::{buildkit, git};
use crate::message::{BuildComplete, BuildRequest};

pub async fn run(cfg: &Config, req: BuildRequest) -> BuildComplete {
    let work_dir = PathBuf::from(&cfg.workspace_dir).join(&req.deploy_id);

    let result = execute(cfg, &req, &work_dir).await;

    let _ = tokio::fs::remove_dir_all(&work_dir).await;

    match result {
        Ok(()) => BuildComplete::success(&req.deploy_id, &req.image_uri),
        Err(e) => BuildComplete::failure(&req.deploy_id, &e.to_string()),
    }
}

async fn execute(cfg: &Config, req: &BuildRequest, work_dir: &PathBuf) -> anyhow::Result<()> {
    tokio::fs::create_dir_all(work_dir).await?;

    git::clone_at_commit(&req.repo_url, &req.commit_sha, work_dir).await?;

    buildkit::build_and_push(
        &cfg.gcp_project,
        work_dir,
        &req.repo_url,
        &req.commit_sha,
        &req.image_uri,
    ).await?;

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::Config;
    use crate::message::BuildRequest;
    use tempfile::TempDir;

    fn test_config(workspace: &str) -> Config {
        Config {
            gcp_project: "test".into(),
            subscription: "test".into(),
            complete_topic: "test".into(),
            workspace_dir: workspace.into(),
        }
    }

    #[tokio::test]
    async fn test_job_clone_succeeds_build_fails() {
        let tmp = TempDir::new().unwrap();
        let cfg = test_config(tmp.path().to_str().unwrap());

        let req = BuildRequest {
            deploy_id: "test-deploy-1".into(),
            project_name: "log".into(),
            repo_url: "https://github.com/rust-lang/log.git".into(),
            commit_sha: "43f2c283".into(),
            image_uri: "fake.registry/log:abc".into(),
        };

        let result = run(&cfg, req).await;

        assert!(!result.success);
        assert_eq!(result.deploy_id, "test-deploy-1");
        assert!(!result.error_message.is_empty());
    }

    #[tokio::test]
    async fn test_job_bad_repo_fails() {
        let tmp = TempDir::new().unwrap();
        let cfg = test_config(tmp.path().to_str().unwrap());

        let req = BuildRequest {
            deploy_id: "test-deploy-2".into(),
            project_name: "nope".into(),
            repo_url: "https://github.com/nonexistent/nonexistent.git".into(),
            commit_sha: "abc".into(),
            image_uri: "fake.registry/nope:abc".into(),
        };

        let result = run(&cfg, req).await;

        assert!(!result.success);
        assert!(result.error_message.contains("clone failed"));
    }

    #[tokio::test]
    async fn test_job_cleans_up_workdir() {
        let tmp = TempDir::new().unwrap();
        let cfg = test_config(tmp.path().to_str().unwrap());

        let req = BuildRequest {
            deploy_id: "cleanup-test".into(),
            project_name: "log".into(),
            repo_url: "https://github.com/rust-lang/log.git".into(),
            commit_sha: "43f2c283".into(),
            image_uri: "fake.registry/log:abc".into(),
        };

        let _ = run(&cfg, req).await;

        let work_dir = tmp.path().join("cleanup-test");
        assert!(!work_dir.exists(), "work dir should be removed after job");
    }
}
