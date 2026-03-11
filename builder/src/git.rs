use anyhow::Result;
use tokio::process::Command;
use std::path::Path;

pub async fn clone_at_commit(repo_url: &str, commit_sha: &str, dest_dir: &Path) -> Result<()> {
  let clone = Command::new("git")
      .args(["clone", "--depth=50", repo_url, dest_dir.to_str().unwrap()])
      .output().await?;

    if !clone.status.success() {
        let stderr = String::from_utf8_lossy(&clone.stderr);
        anyhow::bail!("git clone failed: {}", stderr);
    }

  let checkout = Command::new("git")
      .args(["-C", dest_dir.to_str().unwrap(), "checkout", commit_sha])
      .output().await?;

    if checkout.status.success() {
        let stderr = String::from_utf8_lossy(&checkout.stderr);
        anyhow::bail!("git checkout failed: {}", stderr);
    }

    Ok(())
}
