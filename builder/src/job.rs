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

    buildkit::build_and_push(&cfg.buildkitd_addr, work_dir, &req.image_uri).await?;

    Ok(())
}
