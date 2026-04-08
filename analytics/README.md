<p align="center">
  <img src="https://img.shields.io/badge/Python-3.12+-3776AB?style=for-the-badge&logo=python&logoColor=white" alt="Python">
</p>

<h1 align="center">Analytics</h1>

<p align="center">
  DORA metrics and deployment analytics for Bifrost.
</p>

---

## Overview

The analytics service is a FastAPI application that computes deployment performance metrics from Bifrost's Postgres database. It implements the four DORA (DevOps Research and Assessment) metrics — deployment frequency, lead time for changes, change failure rate, and mean time to recovery — plus health analytics with anomaly detection.

This service is observational. It reads from the same Postgres database the gateway writes to, computes aggregate statistics, and serves them through a REST API consumed by the dashboard.

## How It Works

The service connects to the Bifrost Postgres database and queries three tables:

- **`deployments`** — Timestamps, statuses, and timing data for every deployment. Used to compute DORA metrics.
- **`projects`** — Project metadata. Used for per-project breakdowns.
- **`health_checks`** — CPU, memory, response time, and status data from the Zig sidecar. Used for health trend analysis.

**DORA metrics are computed as:**

- **Deployment Frequency** — `count(deployments) / days`, reported as daily and weekly rates.
- **Lead Time for Changes** — Time from deployment creation to completion (`deploy_finished_at - created_at`), reported as average, p50, and p95 percentiles.
- **Change Failure Rate** — `count(status='failed') / count(total) * 100`.
- **Mean Time to Recovery (MTTR)** — Average time from a failed deployment to the next successful one, computed per project.

**Health analytics** include trend statistics (average CPU, memory, response time) and z-score anomaly detection. Data points with a z-score above 2.5 standard deviations are flagged as anomalies.

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Service health check |
| GET | `/api/v1/analytics/dora` | All four DORA metrics (default: last 30 days) |
| GET | `/api/v1/analytics/dora/projects` | DORA metrics broken down per project |
| GET | `/api/v1/analytics/health` | Health trends and anomaly detection (default: last 24 hours) |
| GET | `/api/v1/analytics/projects` | Per-project deployment summary with success rates |
| GET | `/api/v1/analytics/failures` | Failure history with error messages |

**Query parameters:**
- `days` (1–365) — Time range for deployment metrics
- `hours` (1–720) — Time range for health metrics
- `deployment_id` — Filter health metrics to a specific deployment

## Project Structure

```
analytics/
├── src/
│   ├── main.py          # FastAPI app, endpoint definitions, lifespan management
│   ├── config.py        # Pydantic Settings: loads BF_ANALYTICS_* env vars
│   ├── db.py            # Async Postgres connection pool and query functions
│   ├── metrics.py       # DORA computation, per-project breakdown, anomaly detection
│   ├── telemetry.py     # OpenTelemetry setup (optional OTLP export)
│   └── __init__.py
├── tests/
│   ├── test_api.py      # API endpoint tests (19 cases)
│   ├── test_metrics.py  # Metrics computation tests (26 cases)
│   └── __init__.py
├── k8s/
│   ├── deployment.yaml  # Kubernetes Deployment (ClusterIP, port 8090)
│   └── service.yaml     # Kubernetes Service
├── requirements.txt     # Production dependencies
├── requirements-dev.txt # Test dependencies
└── Dockerfile
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BF_ANALYTICS_DATABASE_URL` | `postgresql://bifrost:localdev@localhost:5432/bifrost` | PostgreSQL connection string |
| `BF_ANALYTICS_PORT` | `8090` | HTTP server port |
| `BF_ANALYTICS_ENV` | `dev` | Environment (`dev` enables auto-reload) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OpenTelemetry collector endpoint |

Configuration uses Pydantic v2 `BaseSettings` with the `BF_ANALYTICS_` prefix. All settings can be overridden via environment variables.

## Dependencies

| Library | Purpose |
|---------|---------|
| [fastapi](https://fastapi.tiangolo.com/) | Web framework — async-native, automatic OpenAPI docs, Pydantic validation. The standard for modern Python APIs. |
| [uvicorn](https://www.uvicorn.org/) | ASGI server for running FastAPI. |
| [psycopg[binary]](https://www.psycopg.org/psycopg3/) | PostgreSQL driver — async support with connection pooling via `psycopg_pool`. The modern successor to psycopg2. |
| [pydantic](https://docs.pydantic.dev/) / [pydantic-settings](https://docs.pydantic.dev/latest/concepts/pydantic_settings/) | Data validation and settings management. Loads and validates environment variables with type coercion. |
| [numpy](https://numpy.org/) | Statistical computations — percentile calculations (p50, p95) for lead time metrics and z-score anomaly detection. |
| [opentelemetry-*](https://opentelemetry.io/docs/languages/python/) | Distributed tracing with OTLP gRPC export. Instruments both FastAPI requests and psycopg database queries. |

**Dev dependencies:**
| Library | Purpose |
|---------|---------|
| [pytest](https://pytest.org/) | Test framework. |
| [httpx](https://www.python-httpx.org/) | Async HTTP client used with FastAPI's `TestClient` for API testing. |

## Design Decisions

**Why Python for analytics?** Python has the strongest ecosystem for data analysis — numpy for statistics, pandas-compatible patterns for data manipulation, and first-class BigQuery SDK support. The analytics service is read-heavy and compute-light, so Python's performance characteristics are fine here.

**Why a separate service instead of endpoints on the gateway?** Analytics queries can be expensive (aggregating thousands of deployments over months). Running them in a separate service prevents slow analytics queries from affecting gateway latency. It also allows independent scaling and deployment.

**Why psycopg3 instead of SQLAlchemy?** The queries are straightforward aggregations with parameterized filters. An ORM would add complexity without benefit. Raw async queries with `psycopg` are simpler and faster for this use case.

## Running

```bash
# Setup
cd analytics
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt

# Run (requires Postgres with Bifrost schema)
python -m src.main       # starts on :8090

# From repository root
make analytics-run

# Tests
pip install -r requirements-dev.txt
pytest
```
