use anyhow::Result;
use std::path::Path;

/// Builds a container image using buildctl and pushes it to the registry.
///
/// Runs:
///   buildctl --addr={buildkitd_addr} build \
///     --frontend dockerfile.v0 \
///     --local context={context_dir} \
///     --local dockerfile={context_dir} \
///     --output type=image,name={image_uri},push=true \
///     --progress plain
///
/// Returns Ok(()) on success. Stderr from buildctl becomes the error message.
pub async fn build_and_push(
    buildkitd_addr: &str,
    context_dir: &Path,
    image_uri: &str,
) -> Result<()> {
    todo!()
}
