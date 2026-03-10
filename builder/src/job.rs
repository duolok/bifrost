use anyhow::Result;
use std::path::Path;

use crate::config::Config;
use crate::message::{BuildComplete, BuildRequest};

/// Runs a single build job end-to-end.
///
/// 1. Create a temp directory under workspace_dir
/// 2. git::clone_at_commit(repo_url, commit_sha, temp_dir)
/// 3. buildkit::build_and_push(buildkitd_addr, temp_dir, image_uri)
/// 4. Return BuildComplete::success or BuildComplete::failure
/// 5. Clean up temp directory (even on error)
///
/// This function never returns Err — build failures are captured in BuildComplete.
pub async fn run(cfg: &Config, req: BuildRequest) -> BuildComplete {
    todo!()
}
