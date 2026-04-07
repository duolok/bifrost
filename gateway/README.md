<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go">
</p>

<h1 align="center">Gateway</h1>

<p align="center">
  Central API and deployment orchestrator for Bifrost.
</p>

---

## Overview

The gateway is the heart of Bifrost. It exposes the REST API, handles authentication, manages projects and deployments, orchestrates the build pipeline via Pub/Sub, applies Kubernetes manifests, and monitors application health through a Lua-based rules engine.

Every deployment state transition flows through the gateway. It advances the state machine from `queued` through `validating`, `building`, `deploying`, `running`, and `healthy` — coordinating with the validator, builder, and healthcheck services along the way.

## How It Works

1. A user triggers a deployment via the API, CLI, or GitHub webhook.
2. The gateway creates a deployment record (`queued`), fetches `deploy.toml` from the repository, and sends it to the OCaml validator.
3. If validation passes, the gateway publishes a build request to Google Cloud Pub/Sub.
4. When the Rust builder finishes, it publishes a `build-complete` message. The gateway receives it, generates Kubernetes manifests (app container + Zig healthcheck sidecar), and applies them to GKE.
5. The healthcheck sidecar reports metrics back to the gateway, which evaluates Lua health rules and sends alerts if thresholds are breached.
6. Throughout this process, the gateway emits events to the Elixir realtime service via gRPC for live dashboard updates.

## Project Structure

```
gateway/
├── cmd/
│   └── main.go                    # Entrypoint, service initialization, graceful shutdown
├── internal/
│   ├── api/
│   │   ├── router.go              # Gin route definitions, middleware wiring
│   │   ├── handler.go             # Base handler, audit logging helper
│   │   ├── handler_auth.go        # Registration, login, JWT issuance
│   │   ├── handler_oauth.go       # Google & GitHub OAuth2 flows
│   │   ├── handler_projects.go    # Project CRUD (team-scoped)
│   │   ├── handler_deployments.go # Deploy trigger, retry, rollback, status
│   │   ├── handler_teams.go       # Team management, invitations, roles
│   │   ├── handler_apikeys.go     # API key CRUD
│   │   ├── handler_secrets.go     # Project secret management
│   │   ├── handler_webhook.go     # GitHub webhook ingestion, HMAC verification
│   │   ├── handler_health.go      # Health endpoint and sidecar report ingestion
│   │   └── consts.go              # Audit actions, trigger types, default Lua rules
│   ├── auth/
│   │   └── auth.go                # JWT generation/validation, bcrypt, API key hashing
│   ├── db/
│   │   └── postgres.go            # pgx connection pool setup
│   ├── errors/
│   │   └── errors.go              # Typed error codes and HTTP status mapping
│   ├── events/
│   │   └── emitter.go             # gRPC client for realtime event streaming
│   ├── k8s/
│   │   ├── deployer.go            # K8s client, manifest apply, status transitions
│   │   └── manifests.go           # Deployment + Service manifest builders
│   ├── middleware/
│   │   ├── auth.go                # JWT & API key authentication, RBAC
│   │   ├── logging.go             # Structured request logging with trace IDs
│   │   └── cors.go                # CORS configuration
│   ├── models/
│   │   ├── user.go                # User, RegisterRequest, LoginRequest
│   │   ├── team.go                # Team, TeamMember, Role enum
│   │   ├── project.go             # Project, ProjectStatus enum
│   │   ├── deployment.go          # Deployment, DeploymentStatus, state transitions
│   │   └── apikey.go              # APIKey model
│   ├── notify/
│   │   └── publisher.go           # RabbitMQ AMQP publisher for email notifications
│   ├── pubsub/
│   │   ├── publisher.go           # Pub/Sub publisher for build requests
│   │   └── subscriber.go          # Pub/Sub subscriber for build-complete
│   ├── rules/
│   │   ├── engine.go              # Lua-based health rule evaluation
│   │   └── lua.go                 # Lua VM setup and action collection
│   ├── telemetry/
│   │   └── telemetry.go           # OpenTelemetry OTLP gRPC tracer init
│   └── validator/
│       └── client.go              # HTTP client for OCaml validator, deploy.toml fetching
├── pkg/
│   └── apiutil/
│       └── util.go                # Response helpers, UUID parsing, secret generation
├── k8s/
│   ├── deployment.yaml            # Kubernetes Deployment manifest
│   ├── service.yaml               # LoadBalancer Service
│   ├── rbac.yaml                  # Role + RoleBinding for K8s API access
│   └── sa.yaml                    # ServiceAccount with GCP Workload Identity
├── go.mod
├── go.sum
└── Dockerfile
```

## API Routes

### Public

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Service health check |
| POST | `/api/v1/auth/register` | User registration |
| POST | `/api/v1/auth/login` | User login |
| GET | `/api/v1/auth/google` | Google OAuth redirect |
| GET | `/api/v1/auth/google/callback` | Google OAuth callback |
| GET | `/api/v1/auth/github` | GitHub OAuth redirect |
| GET | `/api/v1/auth/github/callback` | GitHub OAuth callback |
| POST | `/api/v1/webhook/github` | GitHub webhook ingestion |

### Authenticated

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/api/v1/auth/me` | any | Current user info |
| POST | `/api/v1/project` | deployer+ | Create project |
| GET | `/api/v1/projects` | any | List projects (team-scoped) |
| GET | `/api/v1/project/:id` | any | Get project details |
| DELETE | `/api/v1/project/:id` | admin | Archive project |
| POST | `/api/v1/projects/:id/deploy` | deployer+ | Trigger deployment |
| GET | `/api/v1/projects/:id/deployments` | any | List deployments |
| GET | `/api/v1/deployments/:id` | any | Get deployment details |
| POST | `/api/v1/deployments/:id/retry` | deployer+ | Retry failed deployment |
| POST | `/api/v1/projects/:id/rollback` | deployer+ | Rollback to previous version |
| POST | `/api/v1/deployments/:id/health` | any | Report health metrics |
| GET | `/api/v1/projects/:id/secrets` | deployer+ | List secrets |
| POST | `/api/v1/projects/:id/secrets` | deployer+ | Set/update secret |
| DELETE | `/api/v1/projects/:id/secrets/:key` | admin | Delete secret |
| GET | `/api/v1/team` | any | Get team info |
| PUT | `/api/v1/team` | admin | Update team |
| GET | `/api/v1/team/members` | any | List team members |
| POST | `/api/v1/team/invite` | admin | Invite member |
| DELETE | `/api/v1/team/members/:id` | admin | Remove member |
| PUT | `/api/v1/team/members/:id/role` | admin | Update member role |
| GET | `/api/v1/api-keys` | any | List API keys |
| POST | `/api/v1/api-keys` | deployer+ | Create API key |
| DELETE | `/api/v1/api-keys/:id` | deployer+ | Delete API key |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BF_PORT` | `8080` | HTTP server port |
| `BF_ENV` | `dev` | Environment (`dev` or `prod`) |
| `BF_DATABASE_URL` | `postgres://bifrost:localdev@localhost:5432/bifrost?sslmode=disable` | PostgreSQL connection string |
| `BF_JWT_SECRET` | random 32 bytes | JWT signing secret |
| `BF_GCP_PROJECT` | — | Google Cloud project ID |
| `BF_AR_REPO` | — | Artifact Registry repository |
| `BF_K8S_IN_CLUSTER` | `false` | Use in-cluster K8s config |
| `BF_KUBECONFIG` | `~/.kube/config` | Path to kubeconfig |
| `BF_K8S_NAMESPACE` | `bifrost-apps` | K8s namespace for deployments |
| `BF_REALTIME_URL` | — | gRPC address of realtime service |
| `BF_VALIDATOR_URL` | — | HTTP address of OCaml validator |
| `BF_RABBITMQ_URL` | `amqp://bifrost:localdev@localhost:5672/` | RabbitMQ connection string |
| `BF_GOOGLE_CLIENT_ID` | — | Google OAuth client ID |
| `BF_GOOGLE_CLIENT_SECRET` | — | Google OAuth client secret |
| `BF_GITHUB_CLIENT_ID` | — | GitHub OAuth client ID |
| `BF_GITHUB_CLIENT_SECRET` | — | GitHub OAuth client secret |
| `BF_AUTH_CALLBACK_URL` | `http://localhost:8080` | OAuth callback base URL |
| `BF_UI_URL` | — | Frontend UI base URL (for OAuth redirects) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OpenTelemetry collector endpoint |

## Dependencies

| Library | Purpose |
|---------|---------|
| [gin](https://github.com/gin-gonic/gin) | HTTP framework — fast, minimal, middleware-friendly. Standard choice for Go REST APIs. |
| [pgx](https://github.com/jackc/pgx) | PostgreSQL driver — native Go implementation with connection pooling, faster than `database/sql` with `lib/pq`. |
| [client-go](https://github.com/kubernetes/client-go) | Kubernetes API client — official library for creating Deployments, Services, and managing pod lifecycles. |
| [golang-jwt](https://github.com/golang-jwt/jwt) | JWT signing and validation for authentication. |
| [oauth2](https://pkg.go.dev/golang.org/x/oauth2) | Google and GitHub OAuth2 flows. |
| [cloud.google.com/go/pubsub](https://pkg.go.dev/cloud.google.com/go/pubsub) | Google Cloud Pub/Sub client for async build request/completion messaging. |
| [grpc](https://google.golang.org/grpc) | gRPC client for streaming events to the Elixir realtime service. |
| [protobuf](https://google.golang.org/protobuf) | Protocol buffer serialization for gRPC messages. |
| [amqp091-go](https://github.com/rabbitmq/amqp091-go) | RabbitMQ client for publishing email notification messages. |
| [gopher-lua](https://github.com/yuin/gopher-lua) | Embedded Lua VM for evaluating health monitoring rules. Same pattern as OpenResty/Kong. |
| [otel](https://opentelemetry.io/docs/languages/go/) | OpenTelemetry distributed tracing with Gin, gRPC, and HTTP instrumentation. |
| [testcontainers-go](https://github.com/testcontainers/testcontainers-go) | Spins up real Postgres containers for integration tests instead of mocking. |
| [testify](https://github.com/stretchr/testify) | Test assertions and mocking. |
| [uuid](https://github.com/google/uuid) | UUID generation for resource IDs. |
| [crypto](https://pkg.go.dev/golang.org/x/crypto) | bcrypt password hashing. |

## Running

```bash
# From repository root
make dev             # start Postgres + Pub/Sub emulator
make migrate         # run SQL migrations
make gateway-run     # run on :8080

# From gateway/
go build -o gateway cmd/main.go
go run cmd/main.go
go test ./...
```
