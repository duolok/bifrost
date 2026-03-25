from contextlib import asynccontextmanager
from datetime import datetime, timedelta

from psycopg.rows import dict_row
from psycopg_pool import AsyncConnectionPool

from src.config import settings


pool: AsyncConnectionPool | None = None


async def init_pool():
    global pool
    pool = AsyncConnectionPool(
        conninfo=settings.database_url,
        min_size=2,
        max_size=10,
        kwargs={"row_factory": dict_row},
    )
    await pool.open()


async def close_pool():
    if pool:
        await pool.close()


@asynccontextmanager
async def get_conn():
    async with pool.connection() as conn:
        yield conn


async def get_deployment_stats(days: int = 30):
    """Deployment counts and durations over the last N days."""
    since = datetime.utcnow() - timedelta(days=days)
    async with get_conn() as conn:
        rows = await conn.execute(
            """
            SELECT
                d.id,
                d.project_id,
                p.name AS project_name,
                d.status,
                d.created_at,
                d.build_started_at,
                d.build_finished_at,
                d.deploy_started_at,
                d.deploy_finished_at,
                EXTRACT(EPOCH FROM (d.build_finished_at - d.build_started_at)) AS build_duration_s,
                EXTRACT(EPOCH FROM (d.deploy_finished_at - d.deploy_started_at)) AS deploy_duration_s,
                EXTRACT(EPOCH FROM (d.deploy_finished_at - d.created_at)) AS total_lead_time_s
            FROM deployments d
            JOIN projects p ON p.id = d.project_id
            WHERE d.created_at >= %s
            ORDER BY d.created_at DESC
            """,
            (since,),
        )
        return await rows.fetchall()


async def get_failure_history(days: int = 30):
    """All failed deployments with their error messages."""
    since = datetime.utcnow() - timedelta(days=days)
    async with get_conn() as conn:
        rows = await conn.execute(
            """
            SELECT
                d.id,
                d.project_id,
                p.name AS project_name,
                d.status_message,
                d.created_at,
                d.build_started_at,
                d.deploy_started_at
            FROM deployments d
            JOIN projects p ON p.id = d.project_id
            WHERE d.status = 'failed' AND d.created_at >= %s
            ORDER BY d.created_at DESC
            """,
            (since,),
        )
        return await rows.fetchall()


async def get_health_metrics(deployment_id: str | None = None, hours: int = 24):
    """Health check data for anomaly detection."""
    since = datetime.utcnow() - timedelta(hours=hours)
    params: list = [since]
    where_clause = "WHERE h.checked_at >= %s"

    if deployment_id:
        where_clause += " AND h.deployment_id = %s"
        params.append(deployment_id)

    async with get_conn() as conn:
        rows = await conn.execute(
            f"""
            SELECT
                h.deployment_id,
                p.name AS project_name,
                h.status,
                h.response_time_ms,
                h.cpu_percent,
                h.memory_percent,
                h.memory_bytes,
                h.fd_count,
                h.checked_at
            FROM health_checks h
            JOIN deployments d ON d.id = h.deployment_id
            JOIN projects p ON p.id = d.project_id
            {where_clause}
            ORDER BY h.checked_at DESC
            """,
            params,
        )
        return await rows.fetchall()


async def get_project_summary():
    async with get_conn() as conn:
        rows = await conn.execute(
            """
            SELECT
                p.id,
                p.name,
                p.status,
                COUNT(d.id) AS total_deploys,
                COUNT(d.id) FILTER (WHERE d.status = 'running' OR d.status = 'healthy') AS successful,
                COUNT(d.id) FILTER (WHERE d.status = 'failed') AS failed,
                MAX(d.created_at) AS last_deploy_at,
                AVG(EXTRACT(EPOCH FROM (d.deploy_finished_at - d.created_at)))
                    FILTER (WHERE d.deploy_finished_at IS NOT NULL) AS avg_lead_time_s
            FROM projects p
            LEFT JOIN deployments d ON d.project_id = p.id
            GROUP BY p.id, p.name, p.status
            ORDER BY total_deploys DESC
            """
        )
        return await rows.fetchall()
