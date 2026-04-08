<p align="center">
  <img src="https://img.shields.io/badge/Rust-1.x-DEA584?style=for-the-badge&logo=rust&logoColor=white" alt="Rust">
</p>

<h1 align="center">CLI</h1>

<p align="center">
  Command-line interface for managing projects and deployments on Bifrost.
</p>

---

## Overview

The Bifrost CLI is a Rust command-line tool that provides full access to the platform from the terminal. It communicates with the Go gateway over REST and connects to the Elixir realtime service over WebSocket for live log streaming.

Users can register, log in, manage projects and secrets, trigger deployments, watch build logs in real-time, check deployment status, and roll back — all without opening a browser.

## Commands

```
bifrost [--gateway URL] <COMMAND>

Authentication:
  login                                        Interactive login with email/password
  register                                     Create a new account
  logout                                       Clear stored auth token
  me                                           Show current user info

Projects:
  projects list                                List all projects
  projects create --name <N> --repo-url <URL>  Create a new project
  projects get <ID>                            Get project details
  projects delete <ID>                         Delete a project

Deployments:
  deploy --project <ID> [--commit <SHA>]       Trigger a deployment
  status [--project <ID>]                      Show deployment status
  rollback --project <ID>                      Rollback to previous version

Logs:
  logs <DEPLOYMENT_ID>                         Stream real-time build/deploy logs

Secrets:
  secrets list --project <ID>                  List project secrets
  secrets set --project <ID> --key <K> --secret-ref <REF>   Set a secret
  secrets delete --project <ID> --key <K>      Delete a secret
```

The `--gateway` flag overrides the API URL for a single invocation. The `deploy` command auto-detects the current git commit SHA if `--commit` is omitted.

## How It Works

**Authentication** — The CLI stores a JWT token at `~/.config/bifrost/token` (platform-specific via the `dirs` crate). After login or registration, the token is saved and used as a Bearer token for all subsequent API calls. `logout` deletes the file.

**API Communication** — All project, deployment, secret, and auth operations use REST calls to the gateway (`/api/v1/*` endpoints). Responses are displayed as formatted, color-coded terminal output.

**Live Logs** — The `logs` command opens a WebSocket connection to the Elixir realtime service using the Phoenix v2 channel protocol. It joins the `build:logs:{deployment_id}` and `deploy:status:{deployment_id}` channels and streams log lines to the terminal as they arrive.

## Project Structure

```
cli/
├── src/
│   ├── main.rs            # Clap CLI definition, command routing
│   ├── config.rs          # Env var loading, token file management
│   ├── client.rs          # HTTP client for all gateway API calls
│   └── commands/
│       ├── mod.rs          # Command module exports
│       ├── auth.rs         # login, register, logout, me
│       ├── deploy.rs       # Trigger deployment (with git SHA auto-detect)
│       ├── status.rs       # Show deployment status with colored output
│       ├── logs.rs         # WebSocket log streaming (Phoenix v2 protocol)
│       ├── rollback.rs     # Rollback to previous deployment
│       ├── projects.rs     # Project CRUD operations
│       └── secrets.rs      # Secret management
├── Cargo.toml
├── Cargo.lock
└── Makefile
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BIFROST_GATEWAY_URL` | `http://localhost:8080` | Gateway API base URL |
| `BIFROST_REALTIME_URL` | `http://localhost:4000` | Realtime WebSocket service URL |

## Dependencies

| Crate | Purpose |
|-------|---------|
| [clap](https://crates.io/crates/clap) | CLI argument parsing with derive macros — generates help text, validates arguments, and routes to subcommands. The standard Rust CLI framework. |
| [reqwest](https://crates.io/crates/reqwest) | HTTP client for all REST API calls to the gateway. Supports JSON serialization. |
| [tokio](https://tokio.rs/) | Async runtime — required by reqwest and tokio-tungstenite for non-blocking I/O. |
| [tokio-tungstenite](https://crates.io/crates/tokio-tungstenite) | WebSocket client for connecting to the Elixir realtime service and streaming logs. |
| [futures-util](https://crates.io/crates/futures-util) | Async stream utilities for reading from the WebSocket connection. |
| [serde](https://serde.rs/) / [serde_json](https://crates.io/crates/serde_json) | JSON serialization for API request/response payloads. |
| [colored](https://crates.io/crates/colored) | Terminal color output for status indicators (green for healthy, red for failed, etc.). |
| [rpassword](https://crates.io/crates/rpassword) | Secure password input that hides characters while typing. |
| [dirs](https://crates.io/crates/dirs) | Platform-specific config directory detection (`~/.config/bifrost/` on Linux, `~/Library/Application Support/` on macOS). |
| [anyhow](https://crates.io/crates/anyhow) | Error handling with context for readable error messages. |
| [toml](https://crates.io/crates/toml) | TOML parsing (included for potential config file support). |

## Design Decisions

**Why Rust for the CLI?** The CLI was already being built alongside the Rust builder, so the toolchain was already in place. Rust produces a single static binary with no runtime dependencies, fast startup, and cross-platform compilation. The strong type system catches API contract mismatches at compile time.

**Why WebSocket for logs instead of polling?** Build logs can be hundreds of lines per second. Polling would either miss lines (long interval) or hammer the API (short interval). The WebSocket connection to the Elixir service delivers every log line as it happens with no gaps.

**Why Phoenix v2 protocol?** The Elixir realtime service uses Phoenix Channels, which have their own multiplexing protocol over WebSocket. The CLI implements the v2 protocol directly (join, heartbeat, message handling) to subscribe to multiple channels over a single connection.

## Running

```bash
# Build
cd cli && cargo build
# or
make -C cli cli-build

# Run
./cli/target/debug/cli login
./cli/target/debug/cli projects list
./cli/target/debug/cli deploy --project <ID>
./cli/target/debug/cli logs <DEPLOYMENT_ID>
```
