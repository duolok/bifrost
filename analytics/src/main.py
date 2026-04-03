import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI, Query
from fastapi.responses import JSONResponse

from src import db
from src.config import settings
from src.metrics import analyze_health, compute_dora, compute_per_project
from src import telemetry

logging.basicConfig(
    level=logging.INFO,
    format='{"time":"%(asctime)s","level":"%(levelname)s","msg":"%(message)s"}',
)
log = logging.getLogger("analytics")


@asynccontextmanager
async def lifespan(app: FastAPI):
    log.info("connecting to postgres")
    await db.init_pool()
    log.info("analytics service ready on port %d", settings.port)
    yield
    await db.close_pool()
    log.info("shutdown complete")


app = FastAPI(
    title="Bifrost Analytics",
    version="1.0.0",
    lifespan=lifespan,
)

telemetry.init(app)


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.get("/api/v1/analytics/dora")
async def dora_metrics(days: int = Query(30, ge=1, le=365)):
    """DORA four key metrics across all projects."""
    deployments = await db.get_deployment_stats(days)
    return compute_dora(deployments, days)


@app.get("/api/v1/analytics/dora/projects")
async def dora_per_project(days: int = Query(30, ge=1, le=365)):
    """DORA metrics broken down per project."""
    deployments = await db.get_deployment_stats(days)
    return compute_per_project(deployments, days)


@app.get("/api/v1/analytics/health")
async def health_analytics(
    deployment_id: str | None = Query(None),
    hours: int = Query(24, ge=1, le=720),
):
    """Health check trend analysis and anomaly detection."""
    data = await db.get_health_metrics(deployment_id, hours)
    return analyze_health(data)


@app.get("/api/v1/analytics/projects")
async def project_summary():
    """Per-project deployment summary."""
    rows = await db.get_project_summary()
    projects = []
    for r in rows:
        projects.append({
            "id": str(r["id"]),
            "name": r["name"],
            "status": r["status"],
            "total_deploys": r["total_deploys"],
            "successful": r["successful"],
            "failed": r["failed"],
            "success_rate": round(
                r["successful"] / r["total_deploys"] * 100, 1
            ) if r["total_deploys"] > 0 else 0,
            "avg_lead_time_s": round(float(r["avg_lead_time_s"]), 1) if r["avg_lead_time_s"] else None,
            "last_deploy_at": r["last_deploy_at"].isoformat() if r["last_deploy_at"] else None,
        })
    return {"projects": projects}


@app.get("/api/v1/analytics/failures")
async def failure_analysis(days: int = Query(30, ge=1, le=365)):
    """Recent failures with error messages for pattern analysis."""
    failures = await db.get_failure_history(days)
    return {
        "total": len(failures),
        "failures": [
            {
                "id": str(f["id"]),
                "project": f["project_name"],
                "message": f["status_message"],
                "created_at": f["created_at"].isoformat(),
            }
            for f in failures
        ],
    }


@app.exception_handler(Exception)
async def global_exception_handler(request, exc):
    log.error("unhandled error: %s", exc)
    return JSONResponse(status_code=500, content={"error": "internal server error"})


if __name__ == "__main__":
    import uvicorn
    uvicorn.run("src.main:app", host="0.0.0.0", port=settings.port, reload=settings.env == "dev")
