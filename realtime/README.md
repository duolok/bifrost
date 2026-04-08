<p align="center">
  <img src="https://img.shields.io/badge/Elixir-1.18+-4B275F?style=for-the-badge&logo=elixir&logoColor=white" alt="Elixir">
</p>

<h1 align="center">Realtime</h1>

<p align="center">
  WebSocket event broadcasting service for Bifrost.
</p>

---

## Overview

The realtime service is an Elixir/Phoenix application that acts as the event distribution hub for Bifrost. Backend services (the Go gateway and Rust builder) push events in via gRPC, and the realtime service broadcasts them to connected clients over WebSocket using Phoenix Channels.

This service is purely observational — it doesn't advance the deployment state machine. It receives events about state transitions happening elsewhere and fans them out to dashboards and CLI clients in real-time, so users can watch their builds and deployments happen live.

## How It Works

1. The Go gateway and Rust builder send events to the gRPC server on port 50051 using the `EventIngress` service.
2. The `EventDispatcher` receives each event and broadcasts it to the appropriate Phoenix PubSub topic.
3. Connected WebSocket clients subscribed to channels receive the events instantly.

There are three channel types:

- **`deploy:status:{deploy_id}`** — Deployment lifecycle events (queued, building, deploying, running, failed).
- **`build:logs:{deploy_id}`** — Streaming build log lines from the Rust builder.
- **`activity:feed`** — Global feed of all deployment events across all projects.

The service has no database. All state is in-memory via Phoenix PubSub. If the pod restarts, connected clients reconnect and pick up new events from that point forward.

## Project Structure

```
realtime/
├── lib/
│   ├── realtime/
│   │   ├── application.ex          # OTP supervisor: starts PubSub, gRPC, Phoenix
│   │   ├── event_dispatcher.ex     # Broadcasts events to PubSub topics
│   │   ├── otel_setup.ex           # OpenTelemetry tracing configuration
│   │   └── proto/
│   │       └── events.pb.ex        # Generated protobuf modules
│   ├── realtime_web/
│   │   ├── endpoint.ex             # Phoenix endpoint: WebSocket at /socket
│   │   ├── router.ex               # HTTP routes (health check only)
│   │   ├── telemetry.ex            # Telemetry metrics supervisor
│   │   ├── controllers/
│   │   │   ├── health_controller.ex  # GET /health
│   │   │   └── error_json.ex        # Error response formatting
│   │   └── channels/
│   │       ├── user_socket.ex      # Socket handler, registers all channels
│   │       ├── deploy_channel.ex   # deploy:status:{id} channel
│   │       ├── build_channel.ex    # build:logs:{id} channel
│   │       └── activity_channel.ex # activity:feed channel
│   └── grpc/
│       ├── server.ex               # gRPC server supervisor (port 50051)
│       ├── endpoint.ex             # gRPC endpoint with Logger interceptor
│       └── event_ingress.ex        # gRPC handler: SendEvent, StreamBuildLogs
├── config/
│   ├── config.exs                  # Base config (logger, JSON library)
│   ├── dev.exs                     # Dev overrides (code reloading, debug)
│   ├── prod.exs                    # Prod overrides (info-level logging)
│   ├── runtime.exs                 # Runtime config from environment variables
│   └── test.exs                    # Test config (port 4002, no server)
├── k8s/
│   ├── deployment.yaml             # Kubernetes Deployment
│   └── service.yaml                # ClusterIP (gRPC) + LoadBalancer (WebSocket)
├── mix.exs                         # Dependencies and project config
├── mix.lock
└── Dockerfile
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `4000` | Phoenix HTTP/WebSocket port |
| `GRPC_PORT` | `50051` | gRPC server port for event ingestion |
| `SECRET_KEY_BASE` | — (required in prod) | Cookie signing key, must be 64+ bytes |
| `PHX_SERVER` | — | Set to `true` to start the HTTP server |
| `PHX_HOST` | `example.com` | Hostname for URL generation |
| `DNS_CLUSTER_QUERY` | — | DNS query for Erlang node clustering |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OpenTelemetry collector endpoint |

## Dependencies

| Library | Purpose |
|---------|---------|
| [phoenix](https://hexdocs.pm/phoenix/) | Web framework — provides Channels (WebSocket abstraction), PubSub, and the endpoint/router infrastructure. The core of the realtime event distribution. |
| [bandit](https://hexdocs.pm/bandit/) | HTTP server adapter — modern, performant replacement for Cowboy. Written in pure Elixir. |
| [grpc](https://hexdocs.pm/grpc/) | gRPC server implementation for receiving events from the gateway and builder. |
| [protobuf](https://hexdocs.pm/protobuf/) | Protocol buffer serialization for gRPC message types (`PlatformEvent`, `BuildLogLine`, `Ack`). |
| [jason](https://hexdocs.pm/jason/) | JSON encoding/decoding for WebSocket payloads. |
| [dns_cluster](https://hexdocs.pm/dns_cluster/) | DNS-based Erlang node discovery for multi-pod clustering in Kubernetes. |
| [opentelemetry](https://hexdocs.pm/opentelemetry/) | Distributed tracing with OTLP export and trace context propagation from gRPC headers. |
| [telemetry_metrics](https://hexdocs.pm/telemetry_metrics/) / [telemetry_poller](https://hexdocs.pm/telemetry_poller/) | VM metrics collection (memory, process queue lengths) for observability. |

## Design Decisions

**Why Elixir for realtime?** The BEAM VM handles millions of concurrent WebSocket connections at ~2KB per process. OTP supervisors automatically restart crashed channel processes. Phoenix PubSub is built-in and efficient for fan-out broadcasting. This is exactly what Elixir was designed for.

**Why Phoenix Channels over raw WebSocket?** Channels provide topic-based multiplexing over a single WebSocket connection, automatic heartbeats, presence tracking, and a well-defined join/leave lifecycle. A single dashboard connection can subscribe to multiple deploy and build log channels simultaneously.

**Why no database?** The realtime service is stateless by design. It's a pipe — events go in via gRPC and come out via WebSocket. Historical data lives in Postgres (via the gateway). If you need to see past events, query the gateway API.

## Running

```bash
# From repository root
make realtime-run        # mix phx.server

# From realtime/
mix deps.get
mix phx.server           # starts on :4000 (HTTP/WS) and :50051 (gRPC)
```
