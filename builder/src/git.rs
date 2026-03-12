use anyhow::Result;
use tokio::process::Command;
use std::path::Path;

pub async fn clone_at_commit(repo_url: &str, commit_sha: &str, dest_dir: &Path) -> Result<()> {
  let clone = Command::new("git")
      .env("GIT_TERMINAL_PROMPT", "0")
      .args(["clone", "--depth=50", repo_url, dest_dir.to_str().unwrap()])
      .output().await?;

    if !clone.status.success() {
        let stderr = String::from_utf8_lossy(&clone.stderr);
        anyhow::bail!("git clone failed: {}", stderr);
    }

  let checkout = Command::new("git")
      .args(["-C", dest_dir.to_str().unwrap(), "checkout", commit_sha])
      .output().await?;

    if !checkout.status.success() {
        let stderr = String::from_utf8_lossy(&checkout.stderr);
        anyhow::bail!("git checkout failed: {}", stderr);
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[tokio::test]
    async fn test_clone_at_commit() {
        let tmp = TempDir::new().unwrap();
        let dest = tmp.path().join("repo");

        let result = clone_at_commit(
            "https://github.com/rust-lang/log.git",
            "43f2c283",
            &dest,
        ).await;

        assert!(result.is_ok(), "clone failed: {:?}", result.err());
        assert!(dest.join("Cargo.toml").exists(), "Cargo.toml should exist after clone");
    }

    #[tokio::test]
    async fn test_clone_bad_commit() {
        let tmp = TempDir::new().unwrap();
        let dest = tmp.path().join("repo");

        let result = clone_at_commit(
            "https://github.com/rust-lang/log.git",
            "0000000000000000000000000000000000000000",
            &dest,
        ).await;

        assert!(result.is_err(), "should fail on nonexistent commit");
    }
}
