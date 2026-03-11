use anyhow::Result;
use tokio::process::Command;
use std::path::Path;

pub async fn build_and_push(
    buildkitd_addr: &str,
    context_dir: &Path,
    image_uri: &str,
) -> Result<()> {
    let output = Command::new("buildctl")
        .args([
            "--addr", buildkitd_addr,
            "build",
            "--frontend", "dockerfile.v0",
            "--local", &format!("context={}", context_dir.display()),
            "--local", &format!("dockerfile={}", context_dir.display()),
            "--output", &format!("type=image,name={},push=true", image_uri),
            "--progress", "plain",
        ])
        .output().await?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        anyhow::bail!("buildctl failed: {}", stderr);
    }

    Ok(())
}
