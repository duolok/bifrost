from collections import defaultdict
from datetime import datetime, timedelta

import numpy as np


def compute_dora(deployments: list[dict], days: int = 30) -> dict:
    if not deployments:
        return {
            "deployment_frequency": {"daily": 0, "weekly": 0, "total": 0},
            "lead_time": {"avg_s": 0, "p50_s": 0, "p95_s": 0},
            "change_failure_rate": 0,
            "mttr_s": 0,
        }

    total = len(deployments)
    failed = sum(1 for d in deployments if d["status"] == "failed")

    daily = total / max(days, 1)
    weekly = daily * 7

    # Lead time (only for successful deploys with timing data)
    lead_times = [
        d["total_lead_time_s"]
        for d in deployments
        if d.get("total_lead_time_s") is not None and d["status"] != "failed"
    ]

    if lead_times:
        arr = np.array(lead_times, dtype=np.float64)
        avg_lt = float(np.mean(arr))
        p50_lt = float(np.percentile(arr, 50))
        p95_lt = float(np.percentile(arr, 95))
    else:
        avg_lt = p50_lt = p95_lt = 0

    cfr = (failed / total * 100) if total > 0 else 0

    mttr = _compute_mttr(deployments)

    return {
        "deployment_frequency": {
            "daily": round(daily, 2),
            "weekly": round(weekly, 2),
            "total": total,
        },
        "lead_time": {
            "avg_s": round(avg_lt, 1),
            "p50_s": round(p50_lt, 1),
            "p95_s": round(p95_lt, 1),
        },
        "change_failure_rate": round(cfr, 1),
        "mttr_s": round(mttr, 1),
    }


def _compute_mttr(deployments: list[dict]) -> float:
    """Mean time to recovery: avg time from failure to next success per project."""
    by_project: dict[str, list[dict]] = defaultdict(list)
    for d in deployments:
        by_project[d["project_id"]].append(d)

    recovery_times = []
    for project_deploys in by_project.values():
        sorted_deps = sorted(project_deploys, key=lambda x: x["created_at"])
        last_failure: datetime | None = None
        for dep in sorted_deps:
            if dep["status"] == "failed":
                last_failure = dep["created_at"]
            elif last_failure and dep["status"] in ("running", "healthy"):
                delta = (dep["created_at"] - last_failure).total_seconds()
                recovery_times.append(delta)
                last_failure = None

    return float(np.mean(recovery_times)) if recovery_times else 0


def compute_per_project(deployments: list[dict], days: int = 30) -> list[dict]:
    """DORA metrics broken down per project."""
    by_project: dict[str, list[dict]] = defaultdict(list)
    for d in deployments:
        by_project[d.get("project_name", "unknown")].append(d)

    results = []
    for name, deps in sorted(by_project.items()):
        dora = compute_dora(deps, days)
        dora["project"] = name
        results.append(dora)
    return results


def analyze_health(health_data: list[dict]) -> dict:
    """Analyze health check data: trends, anomalies, summary stats."""
    if not health_data:
        return {"summary": {}, "anomalies": [], "by_project": {}}

    response_times = [h["response_time_ms"]
                      for h in health_data if h.get("response_time_ms")]
    cpu_values = [h["cpu_percent"]
                  for h in health_data if h.get("cpu_percent") is not None]
    mem_values = [h["memory_percent"]
                  for h in health_data if h.get("memory_percent") is not None]
    unhealthy_count = sum(
        1 for h in health_data if h.get("status") == "unhealthy")

    summary = {
        "total_checks": len(health_data),
        "unhealthy_count": unhealthy_count,
        "unhealthy_rate": round(unhealthy_count / len(health_data) * 100, 1) if health_data else 0,
    }

    if response_times:
        rt = np.array(response_times, dtype=np.float64)
        summary["response_time"] = {
            "avg_ms": round(float(np.mean(rt)), 1),
            "p50_ms": round(float(np.percentile(rt, 50)), 1),
            "p95_ms": round(float(np.percentile(rt, 95)), 1),
            "max_ms": round(float(np.max(rt)), 1),
        }

    if cpu_values:
        cpu = np.array(cpu_values, dtype=np.float64)
        summary["cpu"] = {
            "avg_percent": round(float(np.mean(cpu)), 1),
            "max_percent": round(float(np.max(cpu)), 1),
        }

    if mem_values:
        mem = np.array(mem_values, dtype=np.float64)
        summary["memory"] = {
            "avg_percent": round(float(np.mean(mem)), 1),
            "max_percent": round(float(np.max(mem)), 1),
        }

    # Anomaly detection using z-scores
    anomalies = _detect_anomalies(health_data)

    # Per-project breakdown
    by_project = _health_by_project(health_data)

    return {"summary": summary, "anomalies": anomalies, "by_project": by_project}


def _detect_anomalies(health_data: list[dict], z_threshold: float = 2.5) -> list[dict]:
    """Flag data points where metrics deviate significantly from the mean."""
    anomalies = []

    for metric_key in ("response_time_ms", "cpu_percent", "memory_percent"):
        values = [
            (i, h[metric_key])
            for i, h in enumerate(health_data)
            if h.get(metric_key) is not None
        ]
        if len(values) < 5:
            continue

        arr = np.array([v[1] for v in values], dtype=np.float64)
        mean = np.mean(arr)
        std = np.std(arr)
        if std == 0:
            continue

        for idx, val in values:
            z = abs(val - mean) / std
            if z >= z_threshold:
                h = health_data[idx]
                anomalies.append({
                    "metric": metric_key,
                    "value": round(float(val), 2),
                    "z_score": round(float(z), 2),
                    "mean": round(float(mean), 2),
                    "deployment_id": str(h.get("deployment_id", "")),
                    "project": h.get("project_name", ""),
                    "checked_at": h["checked_at"].isoformat() if h.get("checked_at") else None,
                })

    return anomalies


def _health_by_project(health_data: list[dict]) -> dict:
    """Group health stats per project."""
    by_project: dict[str, list[dict]] = defaultdict(list)
    for h in health_data:
        by_project[h.get("project_name", "unknown")].append(h)

    result = {}
    for name, checks in by_project.items():
        unhealthy = sum(1 for c in checks if c.get("status") == "unhealthy")
        rts = [c["response_time_ms"]
               for c in checks if c.get("response_time_ms")]
        result[name] = {
            "total_checks": len(checks),
            "unhealthy_count": unhealthy,
            "avg_response_ms": round(float(np.mean(rts)), 1) if rts else 0,
        }

    return result
