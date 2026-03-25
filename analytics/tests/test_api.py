from datetime import datetime
from unittest.mock import AsyncMock, patch
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient

from src.main import app


client = TestClient(app)


FAKE_DEPLOYS = [
    {
        "id": uuid4(),
        "project_id": uuid4(),
        "project_name": "web-app",
        "status": "running",
        "created_at": datetime(2026, 3, 20, 10, 0),
        "build_started_at": datetime(2026, 3, 20, 10, 1),
        "build_finished_at": datetime(2026, 3, 20, 10, 5),
        "deploy_started_at": datetime(2026, 3, 20, 10, 5),
        "deploy_finished_at": datetime(2026, 3, 20, 10, 8),
        "total_lead_time_s": 480.0,
    },
    {
        "id": uuid4(),
        "project_id": uuid4(),
        "project_name": "api-svc",
        "status": "failed",
        "created_at": datetime(2026, 3, 21, 12, 0),
        "build_started_at": datetime(2026, 3, 21, 12, 1),
        "build_finished_at": None,
        "deploy_started_at": None,
        "deploy_finished_at": None,
        "total_lead_time_s": None,
    },
]

FAKE_HEALTH = [
    {
        "deployment_id": uuid4(),
        "project_name": "web-app",
        "status": "healthy",
        "response_time_ms": 45,
        "cpu_percent": 15.0,
        "memory_percent": 35.0,
        "memory_bytes": 100_000_000,
        "fd_count": 25,
        "checked_at": datetime(2026, 3, 25, 14, i),
    }
    for i in range(10)
]

FAKE_PROJECTS = [
    {
        "id": uuid4(),
        "name": "web-app",
        "status": "active",
        "total_deploys": 5,
        "successful": 4,
        "failed": 1,
        "last_deploy_at": datetime(2026, 3, 25, 10, 0),
        "avg_lead_time_s": 300.0,
    },
]

FAKE_FAILURES = [
    {
        "id": uuid4(),
        "project_id": uuid4(),
        "project_name": "api-svc",
        "status_message": "build failed: Dockerfile not found",
        "created_at": datetime(2026, 3, 21, 12, 0),
        "build_started_at": datetime(2026, 3, 21, 12, 1),
        "deploy_started_at": None,
    },
]


def test_health():
    resp = client.get("/health")
    assert resp.status_code == 200
    assert resp.json() == {"status": "ok"}


@patch("src.main.db.get_deployment_stats", new_callable=AsyncMock, return_value=FAKE_DEPLOYS)
def test_dora_endpoint(mock_db):
    resp = client.get("/api/v1/analytics/dora")
    assert resp.status_code == 200
    data = resp.json()
    assert "deployment_frequency" in data
    assert "lead_time" in data
    assert "change_failure_rate" in data
    assert "mttr_s" in data
    assert data["deployment_frequency"]["total"] == 2
    assert data["change_failure_rate"] == 50.0


@patch("src.main.db.get_deployment_stats", new_callable=AsyncMock, return_value=[])
def test_dora_empty(mock_db):
    resp = client.get("/api/v1/analytics/dora")
    assert resp.status_code == 200
    assert resp.json()["deployment_frequency"]["total"] == 0


@patch("src.main.db.get_deployment_stats", new_callable=AsyncMock, return_value=FAKE_DEPLOYS)
def test_dora_custom_days(mock_db):
    resp = client.get("/api/v1/analytics/dora?days=7")
    assert resp.status_code == 200
    mock_db.assert_called_once_with(7)


def test_dora_invalid_days():
    resp = client.get("/api/v1/analytics/dora?days=0")
    assert resp.status_code == 422


def test_dora_days_too_large():
    resp = client.get("/api/v1/analytics/dora?days=999")
    assert resp.status_code == 422


@patch("src.main.db.get_deployment_stats", new_callable=AsyncMock, return_value=FAKE_DEPLOYS)
def test_dora_per_project(mock_db):
    resp = client.get("/api/v1/analytics/dora/projects")
    assert resp.status_code == 200
    data = resp.json()
    assert isinstance(data, list)
    assert len(data) == 2
    names = {d["project"] for d in data}
    assert "web-app" in names
    assert "api-svc" in names


@patch("src.main.db.get_health_metrics", new_callable=AsyncMock, return_value=FAKE_HEALTH)
def test_health_analytics(mock_db):
    resp = client.get("/api/v1/analytics/health")
    assert resp.status_code == 200
    data = resp.json()
    assert "summary" in data
    assert "anomalies" in data
    assert "by_project" in data
    assert data["summary"]["total_checks"] == 10


@patch("src.main.db.get_health_metrics", new_callable=AsyncMock, return_value=FAKE_HEALTH)
def test_health_with_deployment_filter(mock_db):
    did = str(uuid4())
    resp = client.get(f"/api/v1/analytics/health?deployment_id={did}&hours=48")
    assert resp.status_code == 200
    mock_db.assert_called_once_with(did, 48)


@patch("src.main.db.get_health_metrics", new_callable=AsyncMock, return_value=[])
def test_health_empty(mock_db):
    resp = client.get("/api/v1/analytics/health")
    assert resp.status_code == 200
    assert resp.json()["summary"] == {}


@patch("src.main.db.get_project_summary", new_callable=AsyncMock, return_value=FAKE_PROJECTS)
def test_projects_summary(mock_db):
    resp = client.get("/api/v1/analytics/projects")
    assert resp.status_code == 200
    data = resp.json()
    assert "projects" in data
    assert len(data["projects"]) == 1
    p = data["projects"][0]
    assert p["name"] == "web-app"
    assert p["total_deploys"] == 5
    assert p["success_rate"] == 80.0
    assert p["avg_lead_time_s"] == 300.0


@patch("src.main.db.get_project_summary", new_callable=AsyncMock, return_value=[])
def test_projects_empty(mock_db):
    resp = client.get("/api/v1/analytics/projects")
    assert resp.status_code == 200
    assert resp.json()["projects"] == []


@patch("src.main.db.get_failure_history", new_callable=AsyncMock, return_value=FAKE_FAILURES)
def test_failures(mock_db):
    resp = client.get("/api/v1/analytics/failures")
    assert resp.status_code == 200
    data = resp.json()
    assert data["total"] == 1
    assert data["failures"][0]["project"] == "api-svc"
    assert "Dockerfile not found" in data["failures"][0]["message"]


@patch("src.main.db.get_failure_history", new_callable=AsyncMock, return_value=[])
def test_failures_empty(mock_db):
    resp = client.get("/api/v1/analytics/failures")
    assert resp.status_code == 200
    assert resp.json()["total"] == 0
