<p align="center">
  <img src="res/logo.png" alt="Bifrost" width="200">
</p>

<h1 align="center">Bifrost</h1>

<p align="center">
  Self-service internal developer platform on GCP.
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/Rust-Builder-DEA584?logo=rust&logoColor=white" alt="Rust">
  <img src="https://img.shields.io/badge/OCaml-Validator-EC6813?logo=ocaml&logoColor=white" alt="OCaml">
  <img src="https://img.shields.io/badge/Elixir-Realtime-4B275F?logo=elixir&logoColor=white" alt="Elixir">
  <img src="https://img.shields.io/badge/Zig-Healthcheck-F7A41D?logo=zig&logoColor=white" alt="Zig">
  <img src="https://img.shields.io/badge/GCP-Cloud-4285F4?logo=googlecloud&logoColor=white" alt="GCP">
</p>

---

## Introduction

Bifrost is a self-service internal developer platform. A developer pushes code to GitHub, and Bifrost automatically builds a container image, deploys it to Kubernetes, assigns a URL, and monitors it.

The system is a polyglot state machine — every deployment follows `queued → validating → building → built → deploying → running → healthy`, with each service responsible for advancing or observing specific transitions.

## Architecture

<p align="center">
  <img src="res/architecture.png" alt="Architecture">
</p>

## Services

| Service | Language | Role |
|---------|----------|------|
| **Gateway** | Go | Central orchestrator, REST API, advances most state transitions |
| **Builder** | Rust | Builds container images, advances `building → built` |
| **Validator** | OCaml | Validates project config, guards `queued → validating` |
| **Realtime** | Elixir | Broadcasts state changes via WebSocket |
| **Healthcheck** | Zig | Sidecar that monitors running deployments |
| **Lua Scripts** | Lua | Embedded in Gateway, modifies routing behavior |
| **Analytics** | Python | Analyzes historical deployment data |
| **CLI** | Go | Command-line interface for platform management |
| **UI** | SvelteKit | Web dashboard |

Communication: sync gRPC/HTTP for immediate calls, async Pub/Sub for long-running work.

## Project Structure

```
├── gateway/           # Go — REST API and orchestrator
├── builder/           # Rust — container image builder
├── validator/         # OCaml — project config validation
├── realtime/          # Elixir — WebSocket state broadcasts
├── healthcheck/       # Zig — deployment health sidecar
├── lua-scripts/       # Lua — dynamic routing rules
├── analytics/         # Python — deployment analytics
├── cli/               # Go — CLI tool
├── ui/                # SvelteKit — web dashboard
├── infra/             # Terraform — GCP infrastructure
├── proto/             # Protobuf definitions
├── ops/               # Operational tooling
├── scripts/           # Development and deployment scripts
└── tests/             # Integration tests
```

## Development

### Prerequisites

- [Docker](https://docs.docker.com/install/) (version 20.10+)
- [Docker Compose](https://docs.docker.com/compose/install/) (version 2.0+)
- [Go](https://golang.org/dl/) (1.25+)

### Local Environment

```bash
make dev             # start Postgres + Pub/Sub emulator
make down            # stop local environment
make clean           # stop and delete all data
```

### Run the Gateway

```bash
make gateway-run     # run Go API on :8080
make test-api        # test all endpoints with curl
```

### Database

```bash
make migrate         # run SQL migrations
make psql            # open Postgres shell
```

### Infrastructure

```bash
make infra-plan      # Terraform plan
make infra-apply     # Terraform apply
```

### Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `BF_PORT` | `8080` | Gateway listen port |
| `BF_ENV` | `dev` | Environment name |
| `BF_DATABASE_URL` | `postgres://bifrost:localdev@localhost:5432/bifrost?sslmode=disable` | Postgres connection |

## License

Bifrost is open-source software licensed under the [MIT](LICENSE) license.
