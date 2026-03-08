#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
DB_URL="${BF_DATABASE_URL:-postgres://bifrost:localdev@localhost:5432/bifrost?sslmode=disable}"
API_URL="${BF_API_URL:-http://localhost:8080}"

usage() {
    echo "Usage: bifrost.sh {up|down|logs|migrate|test-api}"
    exit 1
}

cmd_up() {
    cd "$PROJECT_ROOT"
    docker compose up -d
    echo "Waiting for Postgres..."
    until docker compose exec postgres pg_isready -U bifrost > /dev/null 2>&1; do sleep 1; done
    echo "Environment ready."
}

cmd_down() {
    cd "$PROJECT_ROOT"
    docker compose down
}

cmd_logs() {
    cd "$PROJECT_ROOT"
    docker compose logs -f "${2:-}"
}

cmd_migrate() {
    echo "Running migrations..."
    for f in "$PROJECT_ROOT"/scripts/migrations/*.sql; do
        echo "  Applying $(basename "$f")..."
        psql "$DB_URL" -f "$f"
    done
    echo "Migrations complete."
}

cmd_test_api() {
    echo "=== Health Check ==="
    curl -s "$API_URL/health" | jq .

    echo ""
    echo "=== Create Project ==="
    curl -s -X POST "$API_URL/api/v1/project" \
        -H "Content-Type: application/json" \
        -d '{"name":"test-app","repo_url":"https://github.com/example/test-app"}' | jq .

    echo ""
    echo "=== List Projects ==="
    curl -s "$API_URL/api/v1/projects" | jq .

    echo ""
    echo "=== Trigger Deploy ==="
    PROJECT_ID=$(curl -s "$API_URL/api/v1/projects" | jq -r '.projects[0].id')
    if [ "$PROJECT_ID" != "null" ] && [ -n "$PROJECT_ID" ]; then
        curl -s -X POST "$API_URL/api/v1/projects/$PROJECT_ID/deploy" \
            -H "Content-Type: application/json" \
            -d '{"commit_sha":"abc1234","branch":"main"}' | jq .

        echo ""
        echo "=== List Deployments ==="
        curl -s "$API_URL/api/v1/projects/$PROJECT_ID/deployments" | jq .
    fi

    echo ""
    echo "=== Done ==="
}

case "${1:-}" in
    up)         cmd_up ;;
    down)       cmd_down ;;
    logs)       cmd_logs "$@" ;;
    migrate)    cmd_migrate ;;
    test-api)   cmd_test_api ;;
    *)          usage ;;
esac
