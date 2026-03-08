.PHONY: dev down clean gateway-run gateway-build test-api migrate psql infra-plan infra-apply

dev:
	docker compose up -d
	@echo "Waiting for Postgres..."
	@until docker compose exec postgres pg_isready -U bifrost > /dev/null 2>&1; do sleep 1; done
	@echo "Postgres ready."

down:
	docker compose down

clean:
	docker compose down -v

gateway-build:
	cd gateway && go build -o gateway cmd/main.go

gateway-run:
	cd gateway && go run cmd/main.go

test-api:
	@./scripts/bifrost.sh test-api

migrate:
	@./scripts/bifrost.sh migrate

psql:
	docker compose exec postgres psql -U bifrost -d bifrost

infra-plan:
	cd infra && terraform plan

infra-apply:
	cd infra && terraform apply
