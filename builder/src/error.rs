use thiserror::Error;

#[derive(Debug, Error)]
pub enum BuildError {
    #[error("git clone failed: {0}")]
    GitClone(String),

    #[error("buildkit build failed: {0}")]
    BuildKit(String),

    #[error("invalid build request: {0}")]
    InvalidRequest(String),
}
