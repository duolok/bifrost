<p align="center">
  <img src="https://img.shields.io/badge/Rust-1.91+-DEA584?style=for-the-badge&logo=rust&logoColor=white" alt="Rust">
</p>

<h1 align="center">Builder</h1>

<p align="center">
  Container image builder for Bifrost.
</p>

---

## Overview

The builder is a Rust service that listens for build requests on Google Cloud Pub/Sub, clones repositories, builds container images using Google Cloud Build, and pushes the result to Artifact Registry. It advances the Bifrost deployment state machine from `building` to `built`.

Build logs and status updates are streamed to the Elixir realtime service via gRPC so users can watch builds happen live on the dashboard.

## How It Works

1. The Go gateway publishes a `BuildRequest` message to Pub/Sub with a repo URL, commit SHA, and deployment ID.
2. The builder receives the message, clones the repository at the exact commit (shallow clone, depth 50).
3. Source code is tarred up and uploaded to a GCS bucket (`{project}_cloudbuild`).
4. A Cloud Build job is submitted via the Cloud Build API with a Docker build step.
5. The builder polls Cloud Build status every 10 seconds, streaming updates to the realtime service.
6. On success, the builder publishes a `BuildComplete` message to Pub/Sub with the image URI.
7. The gateway picks up the completion message and proceeds to deploy.

If the build fails, `BuildComplete` is published with `success: false` and an error message. The gateway marks the deployment as `failed`.

## Project Structure

```
builder/
├── src/
│   ├── main.rs          # Entrypoint: init telemetry, load config, start listener
│   ├── config.rs        # Configuration from environment variables
│   ├── pubsub.rs        # Pub/Sub subscriber loop and completion publisher
│   ├── job.rs           # Job orchestration: clone → build → cleanup
│   ├── git.rs           # Git clone at specific commit
│   ├── buildkit.rs      # Cloud Build API: upload source, submit build, poll status
│   ├── message.rs       # BuildRequest and BuildComplete message types
│   ├── realtime.rs      # gRPC client for streaming events to realtime service
│   └── telemetry.rs     # OpenTelemetry and JSON logging setup
├── build.rs             # Protobuf compilation (events.proto)
├── k8s/
│   ├── deployment.yaml  # Kubernetes Deployment (GKE)
│   └── sa.yaml          # ServiceAccount with GCP Workload Identity
├── Cargo.toml
├── Cargo.lock
└── Dockerfile
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GCP_PROJECT` | — (required) | Google Cloud project ID |
| `PUBSUB_SUBSCRIPTION` | `builder-subscription` | Pub/Sub subscription for build requests |
| `PUBSUB_COMPLETE_TOPIC` | `build-complete` | Pub/Sub topic for build completion |
| `WORKSPACE_DIR` | `/tmp/builder` | Temporary directory for git clones |
| `BF_REALTIME_URL` | — | gRPC address of realtime service (optional, logs disabled if unset) |
| `RUST_LOG` | `info` | Log level filter |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OpenTelemetry collector endpoint |

## Dependencies

| Crate | Purpose |
|-------|---------|
| [tokio](https://tokio.rs/) | Async runtime — the standard for async Rust. Full feature set for concurrent message processing. |
| [google-cloud-pubsub](https://crates.io/crates/google-cloud-pubsub) | Google Cloud Pub/Sub client for receiving build requests and publishing completions. |
| [google-cloud-auth](https://crates.io/crates/google-cloud-auth) | GCP credential handling and token refresh for Cloud Build and GCS API calls. |
| [reqwest](https://crates.io/crates/reqwest) | HTTP client for Cloud Build API and GCS uploads. |
| [tonic](https://crates.io/crates/tonic) | gRPC client framework for streaming events and build logs to the realtime service. |
| [prost](https://crates.io/crates/prost) | Protocol buffer serialization, used with tonic for gRPC message types. |
| [serde](https://serde.rs/) / [serde_json](https://crates.io/crates/serde_json) | JSON serialization for Pub/Sub message payloads. |
| [tracing](https://crates.io/crates/tracing) / [tracing-subscriber](https://crates.io/crates/tracing-subscriber) | Structured JSON logging with log-level filtering. |
| [opentelemetry](https://crates.io/crates/opentelemetry) / [opentelemetry-otlp](https://crates.io/crates/opentelemetry-otlp) | Distributed tracing with OTLP export. Integrates with tracing via tracing-opentelemetry. |
| [anyhow](https://crates.io/crates/anyhow) / [thiserror](https://crates.io/crates/thiserror) | Error handling — anyhow for application errors, thiserror for typed error definitions. |
| [uuid](https://crates.io/crates/uuid) | UUID v4 generation for trace correlation. |
| [tempfile](https://crates.io/crates/tempfile) | Temporary file/directory creation for tests. |

## Design Decisions

**Why Rust for the builder?** The builder handles untrusted user code (cloning and building arbitrary repositories). Rust provides memory safety without a garbage collector, no GC pauses during long builds, and deterministic resource cleanup. Workspaces are always cleaned up even on panic paths.

**Why Cloud Build instead of Buildkit/Kaniko?** Cloud Build runs builds in Google's infrastructure with no need for Docker-in-Docker or privileged containers in our cluster. The builder just submits the job and polls — no local Docker socket required.

**Why Pub/Sub?** Builds are long-running (minutes). Pub/Sub provides retry safety, dead letter queues, and decoupled scaling. The gateway doesn't block waiting for builds to finish.

## Running

```bash
# From repository root
make healthcheck-build   # build with cargo

# Requires GCP credentials and a running Pub/Sub emulator or real project
GCP_PROJECT=my-project cargo run
```
