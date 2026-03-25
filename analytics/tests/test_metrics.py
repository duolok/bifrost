from datetime import datetime, timedelta
from uuid import uuid4


from src.metrics import (
    _compute_mttr,
    _detect_anomalies,
    _health_by_project,
    analyze_health,
    compute_dora,
    compute_per_project,
)


def _deploy(status, lead_time=None, project_id=None, project_name="app-1", created_at=None):
    return {
        "id": str(uuid4()),
        "project_id": project_id or str(uuid4()),
        "project_name": project_name,
        "status": status,
        "created_at": created_at or datetime.utcnow(),
        "build_started_at": None,
        "build_finished_at": None,
        "deploy_started_at": None,
        "deploy_finished_at": None,
        "total_lead_time_s": lead_time,
    }


def _health(status="healthy", response_time_ms=50, cpu=20.0, mem=40.0, project="app-1"):
    return {
        "deployment_id": str(uuid4()),
        "project_name": project,
        "status": status,
        "response_time_ms": response_time_ms,
        "cpu_percent": cpu,
        "memory_percent": mem,
        "memory_bytes": 1024 * 1024 * 100,
        "fd_count": 30,
        "checked_at": datetime.utcnow(),
    }


class TestComputeDora:
    def test_empty_deployments(self):
        result = compute_dora([], 30)
        assert result["deployment_frequency"]["total"] == 0
        assert result["lead_time"]["avg_s"] == 0
        assert result["change_failure_rate"] == 0
        assert result["mttr_s"] == 0

    def test_frequency_calculation(self):
        deploys = [_deploy("running") for _ in range(10)]
        result = compute_dora(deploys, days=10)
        assert result["deployment_frequency"]["total"] == 10
        assert result["deployment_frequency"]["daily"] == 1.0
        assert result["deployment_frequency"]["weekly"] == 7.0

    def test_lead_time_percentiles(self):
        # 10 deploys with lead times 10, 20, 30, ... 100
        deploys = [_deploy("running", lead_time=float(i * 10))
                   for i in range(1, 11)]
        result = compute_dora(deploys, 30)
        assert result["lead_time"]["avg_s"] == 55.0
        assert result["lead_time"]["p50_s"] == 55.0
        assert result["lead_time"]["p95_s"] > 90

    def test_lead_time_ignores_failed(self):
        deploys = [
            _deploy("running", lead_time=100.0),
            _deploy("failed", lead_time=9999.0),
            _deploy("running", lead_time=200.0),
        ]
        result = compute_dora(deploys, 30)
        assert result["lead_time"]["avg_s"] == 150.0

    def test_change_failure_rate(self):
        deploys = [
            _deploy("running"),
            _deploy("running"),
            _deploy("failed"),
            _deploy("running"),
        ]
        result = compute_dora(deploys, 30)
        assert result["change_failure_rate"] == 25.0

    def test_zero_failure_rate(self):
        deploys = [_deploy("running") for _ in range(5)]
        result = compute_dora(deploys, 30)
        assert result["change_failure_rate"] == 0.0

    def test_all_failed(self):
        deploys = [_deploy("failed") for _ in range(3)]
        result = compute_dora(deploys, 30)
        assert result["change_failure_rate"] == 100.0


class TestMTTR:
    def test_no_failures(self):
        deploys = [_deploy("running") for _ in range(3)]
        assert _compute_mttr(deploys) == 0

    def test_failure_then_recovery(self):
        pid = str(uuid4())
        now = datetime.utcnow()
        deploys = [
            _deploy("failed", project_id=pid, created_at=now),
            _deploy("running", project_id=pid,
                    created_at=now + timedelta(minutes=30)),
        ]
        mttr = _compute_mttr(deploys)
        assert 1790 <= mttr <= 1810  # ~1800 seconds = 30 minutes

    def test_multiple_recoveries_averaged(self):
        pid = str(uuid4())
        now = datetime.utcnow()
        deploys = [
            _deploy("failed", project_id=pid, created_at=now),
            _deploy("running", project_id=pid,
                    created_at=now + timedelta(minutes=10)),
            _deploy("failed", project_id=pid,
                    created_at=now + timedelta(minutes=20)),
            _deploy("running", project_id=pid,
                    created_at=now + timedelta(minutes=50)),
        ]
        mttr = _compute_mttr(deploys)
        # Recovery 1: 10min=600s, Recovery 2: 30min=1800s, avg=1200s
        assert 1190 <= mttr <= 1210

    def test_failure_without_recovery(self):
        pid = str(uuid4())
        now = datetime.utcnow()
        deploys = [
            _deploy("running", project_id=pid, created_at=now),
            _deploy("failed", project_id=pid,
                    created_at=now + timedelta(minutes=5)),
        ]
        assert _compute_mttr(deploys) == 0


class TestPerProject:
    def test_groups_by_project(self):
        deploys = [
            _deploy("running", project_name="alpha"),
            _deploy("running", project_name="alpha"),
            _deploy("failed", project_name="beta"),
        ]
        results = compute_per_project(deploys, 30)
        assert len(results) == 2
        names = [r["project"] for r in results]
        assert "alpha" in names
        assert "beta" in names

    def test_per_project_metrics_independent(self):
        deploys = [
            _deploy("running", project_name="alpha", lead_time=100.0),
            _deploy("failed", project_name="beta"),
        ]
        results = compute_per_project(deploys, 30)
        alpha = next(r for r in results if r["project"] == "alpha")
        beta = next(r for r in results if r["project"] == "beta")
        assert alpha["change_failure_rate"] == 0.0
        assert beta["change_failure_rate"] == 100.0


class TestAnalyzeHealth:
    def test_empty_data(self):
        result = analyze_health([])
        assert result["summary"] == {}
        assert result["anomalies"] == []
        assert result["by_project"] == {}

    def test_summary_stats(self):
        data = [_health(response_time_ms=i * 10, cpu=float(i),
                        mem=float(i * 5)) for i in range(1, 11)]
        result = analyze_health(data)
        s = result["summary"]
        assert s["total_checks"] == 10
        assert s["unhealthy_count"] == 0
        assert s["unhealthy_rate"] == 0.0
        assert "response_time" in s
        assert "cpu" in s
        assert "memory" in s
        assert s["response_time"]["avg_ms"] == 55.0
        assert s["response_time"]["max_ms"] == 100.0

    def test_unhealthy_rate(self):
        data = [_health(status="healthy") for _ in range(8)]
        data += [_health(status="unhealthy") for _ in range(2)]
        result = analyze_health(data)
        assert result["summary"]["unhealthy_count"] == 2
        assert result["summary"]["unhealthy_rate"] == 20.0

    def test_by_project_breakdown(self):
        data = [
            _health(project="alpha"),
            _health(project="alpha"),
            _health(project="beta", status="unhealthy"),
        ]
        result = analyze_health(data)
        assert "alpha" in result["by_project"]
        assert "beta" in result["by_project"]
        assert result["by_project"]["alpha"]["total_checks"] == 2
        assert result["by_project"]["beta"]["unhealthy_count"] == 1


class TestAnomalyDetection:
    def test_no_anomalies_uniform_data(self):
        data = [_health(response_time_ms=50, cpu=20.0, mem=40.0)
                for _ in range(10)]
        anomalies = _detect_anomalies(data)
        assert anomalies == []

    def test_detects_response_time_spike(self):
        data = [_health(response_time_ms=50) for _ in range(20)]
        data.append(_health(response_time_ms=5000))  # massive spike
        anomalies = _detect_anomalies(data)
        rt_anomalies = [a for a in anomalies if a["metric"]
                        == "response_time_ms"]
        assert len(rt_anomalies) >= 1
        assert rt_anomalies[0]["value"] == 5000.0

    def test_detects_cpu_spike(self):
        data = [_health(cpu=20.0) for _ in range(20)]
        data.append(_health(cpu=99.0))
        anomalies = _detect_anomalies(data)
        cpu_anomalies = [a for a in anomalies if a["metric"] == "cpu_percent"]
        assert len(cpu_anomalies) >= 1

    def test_skips_too_few_datapoints(self):
        data = [_health() for _ in range(3)]
        anomalies = _detect_anomalies(data)
        assert anomalies == []

    def test_anomaly_contains_metadata(self):
        data = [_health(response_time_ms=50, project="myapp")
                for _ in range(20)]
        data.append(_health(response_time_ms=5000, project="myapp"))
        anomalies = _detect_anomalies(data)
        assert len(anomalies) >= 1
        a = anomalies[0]
        assert "metric" in a
        assert "value" in a
        assert "z_score" in a
        assert "mean" in a
        assert "project" in a
        assert a["project"] == "myapp"


class TestHealthByProject:
    def test_groups_correctly(self):
        data = [
            _health(project="svc-a", response_time_ms=100),
            _health(project="svc-a", response_time_ms=200),
            _health(project="svc-b", response_time_ms=50),
        ]
        result = _health_by_project(data)
        assert result["svc-a"]["total_checks"] == 2
        assert result["svc-a"]["avg_response_ms"] == 150.0
        assert result["svc-b"]["total_checks"] == 1

    def test_counts_unhealthy(self):
        data = [
            _health(project="svc-a", status="healthy"),
            _health(project="svc-a", status="unhealthy"),
            _health(project="svc-a", status="unhealthy"),
        ]
        result = _health_by_project(data)
        assert result["svc-a"]["unhealthy_count"] == 2
