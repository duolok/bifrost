<p align="center">
  <img src="https://img.shields.io/badge/Zig-0.14-F7A41D?style=for-the-badge&logo=zig&logoColor=white" alt="Zig">
</p>

<h1 align="center">Healthcheck</h1>

<p align="center">
  Lightweight sidecar that monitors deployed applications for Bifrost.
</p>

---

## Overview

The healthcheck is a Zig sidecar container injected into every pod deployed by Bifrost. It continuously probes the application's health endpoint, collects system metrics from the Linux `/proc` filesystem, and reports everything back to the Go gateway. The gateway uses these metrics to decide when a deployment transitions from `running` to `healthy`, and to trigger alerts via Lua health rules.

The entire binary is under 500KB with less than 5MB memory usage at runtime. There are zero external dependencies — only the Zig standard library.

## How It Works

Every `BF_INTERVAL_S` seconds (default 10), the sidecar runs a three-phase check:

**1. Health Probe** — Opens a TCP connection to the application's health endpoint (`BF_PROBE_HOST:BF_PROBE_PORT`), sends an HTTP GET request to `BF_PROBE_PATH`, and checks if the response status is 200. Measures response time in milliseconds. Any connection failure or non-200 status is reported as unhealthy.

**2. Metrics Collection** — Reads three data points from `/proc`:
  - **Memory**: Parses `/proc/meminfo` for `MemTotal` and `MemAvailable`, computes used memory.
  - **CPU**: Parses `/proc/stat` for aggregate CPU time fields, computes usage percentage as `(total - idle) / total * 100`.
  - **File descriptors**: Counts entries in `/proc/self/fd` to track open file descriptor count.

**3. Report** — Builds a JSON payload with all metrics and POSTs it to the gateway at `/api/v1/deployments/{deploy_id}/health`. Includes a W3C `traceparent` header with a randomly generated trace ID for distributed tracing correlation.

If any phase fails, the sidecar logs a warning and continues to the next cycle. It never crashes — the loop runs indefinitely.

## Project Structure

```
healthcheck/
├── src/
│   ├── main.zig       # Entrypoint: env var parsing, main loop orchestration
│   ├── probe.zig      # HTTP health probe: TCP connect, send GET, parse status
│   ├── proc.zig       # /proc reader: memory, CPU, file descriptors
│   └── reporter.zig   # JSON report builder, HTTP POST to gateway, traceparent generation
├── build.zig          # Zig build configuration
├── Dockerfile         # Multi-stage Alpine build (Zig 0.14.0)
└── k8s/               # (manifests are in the gateway's K8s config, sidecar injection)
```

## Modules

### `main.zig`
Entry point. Reads all environment variables with fallback defaults, logs the startup configuration, and runs the infinite check loop. Each iteration calls `proc.collect()`, `probe.check()`, and `reporter.send()` in sequence, then sleeps for the configured interval.

### `probe.zig`
Opens a raw TCP connection to the target host and port, writes an HTTP/1.1 GET request, reads the first 1024 bytes of the response, and extracts the status code from bytes 9-12. Returns a `ProbeResult` with `healthy: bool` and `response_time_ms: u64`. On any error (connection refused, timeout, parse failure), returns unhealthy.

### `proc.zig`
Reads Linux `/proc` files using buffered I/O. Parses `MemTotal` and `MemAvailable` from `/proc/meminfo`, computes CPU usage from `/proc/stat` (user, nice, system, idle, iowait, irq, softirq, steal fields), and counts directory entries in `/proc/self/fd`. Returns a `Metrics` struct. Gracefully returns zeros if any `/proc` file is unavailable.

### `reporter.zig`
Builds a JSON string from `HealthReport` data using `std.fmt.bufPrint` into a fixed stack buffer — no heap allocation. Generates a W3C `traceparent` header using `std.crypto.random` for trace and span IDs. Opens a TCP connection to the gateway and sends the full HTTP POST request. Logs a warning if the gateway responds with status >= 400.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BF_DEPLOY_ID` | `unknown` | Deployment ID for the health report endpoint path |
| `BF_PROBE_HOST` | `localhost` | Health endpoint hostname (usually the app in the same pod) |
| `BF_PROBE_PORT` | `8080` | Health endpoint port |
| `BF_PROBE_PATH` | `/health` | Health endpoint HTTP path |
| `BF_GATEWAY_HOST` | `localhost` | Gateway hostname for reporting |
| `BF_GATEWAY_PORT` | `8080` | Gateway port for reporting |
| `BF_INTERVAL_S` | `10` | Seconds between health checks |

## Dependencies

None. The healthcheck uses only the Zig standard library:

- `std.net` — TCP connections for HTTP probe and report sending
- `std.fs` — File reads for `/proc/meminfo`, `/proc/stat`, `/proc/self/fd`
- `std.fmt` — String formatting for HTTP requests and JSON payloads
- `std.crypto.random` — Random bytes for W3C traceparent generation
- `std.time` — Timestamps and sleep intervals
- `std.log` — Structured logging
- `std.posix` — Environment variable access

This is intentional. The sidecar runs in every single pod — one per deployed application. Zero dependencies means a tiny binary, fast startup, minimal attack surface, and no supply chain risk.

## Design Decisions

**Why Zig for the sidecar?** The sidecar is injected into every deployed pod, so resource overhead matters. Zig compiles to a static binary under 500KB, uses less than 5MB of memory at runtime, has zero startup latency, and provides direct syscall access for reading `/proc`. No runtime, no garbage collector, no dependencies.

**Why raw TCP instead of an HTTP library?** The sidecar makes exactly two HTTP requests per cycle (one probe, one report), both to known endpoints with predictable payloads. A full HTTP client library would add unnecessary binary size. The raw TCP approach fits in fixed stack buffers with no heap allocation.

**Why `/proc` instead of cgroups?** `/proc/meminfo` and `/proc/stat` give host-level metrics that are simple to parse and available in every Linux container. For pod-level resource limits, Kubernetes already enforces cgroup constraints — the sidecar reports what the application actually sees.

## Running

```bash
# Build
cd healthcheck && zig build -Doptimize=ReleaseSmall

# Run (needs a target app and gateway running)
BF_PROBE_HOST=localhost BF_PROBE_PORT=8080 BF_GATEWAY_HOST=localhost zig build run

# From repository root
make healthcheck-build
make healthcheck-run
```
