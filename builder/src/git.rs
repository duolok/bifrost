use anyhow::Result;
use std::path::Path;

/// Clones repo_url into dest_dir and checks out commit_sha.
/// Shells out to `git` (available in the builder container image).
///
/// Steps:
///   1. git clone --depth=50 {repo_url} {dest_dir}
///   2. git -C {dest_dir} checkout {commit_sha}
///
/// Returns the path to the cloned directory on success.
pub async fn clone_at_commit(repo_url: &str, commit_sha: &str, dest_dir: &Path) -> Result<()> {
    todo!()
}
