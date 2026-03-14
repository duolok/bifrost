.PHONY: dev down clean gateway-run gateway-build test-api migrate psql \
       infra-plan infra-apply cloud-up cloud-down cloud-status cloud-deploy cloud-psql

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
	cd infra && terraform plan -var="project_id=bifrost-platform" -var="db_password=BfrostPg2026x"

infra-apply:
	cd infra && terraform apply -var="project_id=bifrost-platform" -var="db_password=BfrostPg2026x"

GCP_PROJECT  := bifrost-platform
GCP_REGION   := europe-central2
REGISTRY     := $(GCP_REGION)-docker.pkg.dev/$(GCP_PROJECT)/bifrost-platform
GATEWAY_IMG  := $(REGISTRY)/gateway:latest
DB_IP        := $(shell terraform -chdir=infra output -raw db_ip 2>/dev/null)
CLOUD_RUN_URL = $(shell gcloud run services describe bifrost-gateway --region=$(GCP_REGION) --format='value(status.url)' 2>/dev/null)

cloud-up:
	cd infra && terraform apply -auto-approve -var="project_id=$(GCP_PROJECT)" -var="db_password=BfrostPg2026x"
	gcloud container clusters get-credentials bifrost-cluster --region $(GCP_REGION) --project $(GCP_PROJECT)
	kubectl create namespace bifrost-apps 2>/dev/null || true
	PGPASSWORD='BfrostPg2026x' psql -h $$(terraform -chdir=infra output -raw db_ip) -U bifrost -d bifrost -f scripts/migrations/001_init.sql 2>/dev/null || echo "Tables already exist"
	docker build -t $(GATEWAY_IMG) gateway/
	docker push $(GATEWAY_IMG)
	gcloud run deploy bifrost-gateway \
		--image $(GATEWAY_IMG) \
		--region $(GCP_REGION) \
		--platform managed \
		--set-env-vars "BF_ENV=prod,BF_GCP_PROJECT=$(GCP_PROJECT),BF_AR_REPO=$(GCP_REGION)-docker.pkg.dev/$(GCP_PROJECT)/bifrost-apps,BF_DATABASE_URL=host=$$(terraform -chdir=infra output -raw db_ip) user=bifrost password=BfrostPg2026x dbname=bifrost sslmode=disable" \
		--allow-unauthenticated \
		--min-instances 0 --max-instances 3 \
		--memory 256Mi --cpu 1 --timeout 300s
	@gcloud run services describe bifrost-gateway --region=$(GCP_REGION) --format='value(status.url)'

cloud-down:
	gcloud run services delete bifrost-gateway --region=$(GCP_REGION) --quiet 2>/dev/null || true
	cd infra && terraform destroy -auto-approve -var="project_id=$(GCP_PROJECT)" -var="db_password=BfrostPg2026x"

cloud-status:
	@echo "=== Cloud Run ==="
	@gcloud run services describe bifrost-gateway --region=$(GCP_REGION) --format='table(status.url,status.conditions[0].status)' 2>/dev/null || echo "Not deployed"
	@echo ""
	@echo "=== GKE Cluster ==="
	@gcloud container clusters describe bifrost-cluster --region=$(GCP_REGION) --format='table(status,currentNodeCount)' 2>/dev/null || echo "Not running"
	@echo ""
	@echo "=== Cloud SQL ==="
	@gcloud sql instances describe bifrost-db --format='table(state,ipAddresses[0].ipAddress)' 2>/dev/null || echo "Not running"
	@echo ""
	@echo "=== Health Check ==="
	@curl -s $(CLOUD_RUN_URL)/health 2>/dev/null | jq . || echo "Gateway not reachable"

cloud-deploy:
	docker build -t $(GATEWAY_IMG) gateway/
	docker push $(GATEWAY_IMG)
	gcloud run deploy bifrost-gateway \
		--image $(GATEWAY_IMG) \
		--region $(GCP_REGION) \
		--platform managed
	@echo "==> Deployed. URL:"
	@gcloud run services describe bifrost-gateway --region=$(GCP_REGION) --format='value(status.url)'

cloud-psql:
	PGPASSWORD='BfrostPg2026x' psql -h $$(terraform -chdir=infra output -raw db_ip) -U bifrost -d bifrost
