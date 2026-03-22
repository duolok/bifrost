.PHONY: dev down clean gateway-run gateway-build test-api migrate psql \
       infra-plan infra-apply cloud-up cloud-down cloud-status cloud-deploy cloud-psql \
       validator-build validator-run validator-deploy \
       realtime-build realtime-run realtime-deploy

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

validator-build:
	cd validator && opam exec -- dune build

validator-run:
	cd validator && opam exec -- dune exec bin/main.exe

validator-deploy:
	@echo "==> Building and pushing validator..."
	docker build -t $(VALIDATOR_IMG) validator/
	docker push $(VALIDATOR_IMG)
	@echo "==> Deploying validator to Cloud Run..."
	gcloud run deploy bifrost-validator \
		--image=$(VALIDATOR_IMG) \
		--region=$(GCP_REGION) \
		--project=$(GCP_PROJECT) \
		--port=8090 \
		--allow-unauthenticated \
		--min-instances=0 \
		--max-instances=3 \
		--memory=256Mi \
		--cpu=1
	@echo "==> Validator URL:"
	@gcloud run services describe bifrost-validator --region=$(GCP_REGION) --project=$(GCP_PROJECT) --format='value(status.url)'

realtime-run:
	cd realtime && mix phx.server

realtime-deploy:
	@echo "==> Building and pushing realtime..."
	docker build -t $(REALTIME_IMG) realtime/
	docker push $(REALTIME_IMG)
	@echo "==> Deploying realtime to GKE..."
	kubectl apply -f realtime/k8s/deployment.yaml
	kubectl apply -f realtime/k8s/service.yaml
	kubectl rollout status deployment/bifrost-realtime -n bifrost-apps --timeout=120s
	@echo "==> Realtime deployed."

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

# --- Cloud variables ---

GCP_PROJECT  := bifrost-platform
GCP_REGION   := europe-central2
REGISTRY     := $(GCP_REGION)-docker.pkg.dev/$(GCP_PROJECT)/bifrost-platform
GATEWAY_IMG    := $(REGISTRY)/gateway:latest
BUILDER_IMG    := $(REGISTRY)/builder:latest
VALIDATOR_IMG  := $(REGISTRY)/validator:latest
REALTIME_IMG   := $(REGISTRY)/realtime:latest
VALIDATOR_URL   = $(shell gcloud run services describe bifrost-validator --region=$(GCP_REGION) --project=$(GCP_PROJECT) --format='value(status.url)' 2>/dev/null)
GATEWAY_URL   = $(shell kubectl get svc bifrost-gateway -n bifrost-apps -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null)

# Spin up everything: infra → migrate → build → deploy to GKE
cloud-up:
	@echo "==> Applying Terraform..."
	cd infra && terraform apply -auto-approve -var="project_id=$(GCP_PROJECT)" -var="db_password=BfrostPg2026x"
	@echo "==> Getting kubectl credentials..."
	gcloud container clusters get-credentials bifrost-cluster --region $(GCP_REGION) --project $(GCP_PROJECT)
	kubectl create namespace bifrost-apps 2>/dev/null || true
	@echo "==> Running migrations..."
	PGPASSWORD='BfrostPg2026x' psql -h $$(terraform -chdir=infra output -raw db_ip) -U bifrost -d bifrost -f scripts/migrations/001_init.sql 2>/dev/null || echo "Tables already exist"
	@echo "==> Building and pushing gateway..."
	docker build -t $(GATEWAY_IMG) gateway/
	docker push $(GATEWAY_IMG)
	@echo "==> Building and pushing builder..."
	docker build -t $(BUILDER_IMG) builder/
	docker push $(BUILDER_IMG)
	@echo "==> Building and pushing validator..."
	docker build -t $(VALIDATOR_IMG) validator/
	docker push $(VALIDATOR_IMG)
	@echo "==> Deploying validator to Cloud Run..."
	gcloud run deploy bifrost-validator \
		--image=$(VALIDATOR_IMG) \
		--region=$(GCP_REGION) \
		--project=$(GCP_PROJECT) \
		--port=8090 \
		--allow-unauthenticated \
		--min-instances=0 \
		--max-instances=3 \
		--memory=256Mi \
		--cpu=1
	@echo "==> Building and pushing realtime..."
	docker build -t $(REALTIME_IMG) realtime/
	docker push $(REALTIME_IMG)
	@echo "==> Deploying to GKE..."
	kubectl apply -f gateway/k8s/sa.yaml
	kubectl apply -f gateway/k8s/rbac.yaml
	kubectl apply -f gateway/k8s/deployment.yaml
	kubectl apply -f gateway/k8s/service.yaml
	kubectl apply -f builder/k8s/sa.yaml
	kubectl apply -f builder/k8s/deployment.yaml
	kubectl apply -f realtime/k8s/deployment.yaml
	kubectl apply -f realtime/k8s/service.yaml
	@echo "==> Waiting for gateway external IP..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12; do \
		IP=$$(kubectl get svc bifrost-gateway -n bifrost-apps -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null); \
		if [ -n "$$IP" ]; then echo "==> Gateway: http://$$IP/health"; exit 0; fi; \
		sleep 10; \
	done; echo "==> IP not assigned yet, check: kubectl get svc -n bifrost-apps"

# Tear down everything
cloud-down:
	@echo "==> Deleting GKE workloads..."
	kubectl delete -f realtime/k8s/service.yaml 2>/dev/null || true
	kubectl delete -f realtime/k8s/deployment.yaml 2>/dev/null || true
	kubectl delete -f builder/k8s/deployment.yaml 2>/dev/null || true
	kubectl delete -f gateway/k8s/deployment.yaml 2>/dev/null || true
	kubectl delete -f gateway/k8s/service.yaml 2>/dev/null || true
	kubectl delete -f gateway/k8s/rbac.yaml 2>/dev/null || true
	kubectl delete -f gateway/k8s/sa.yaml 2>/dev/null || true
	kubectl delete -f builder/k8s/sa.yaml 2>/dev/null || true
	@echo "==> Removing GKE cluster from Terraform state (deletion_protection workaround)..."
	cd infra && terraform state rm google_container_cluster.primary 2>/dev/null || true
	@echo "==> Deleting GKE cluster directly..."
	gcloud container clusters delete bifrost-cluster --region $(GCP_REGION) --quiet 2>/dev/null || true
	@echo "==> Destroying remaining Terraform resources..."
	cd infra && terraform destroy -auto-approve -var="project_id=$(GCP_PROJECT)" -var="db_password=BfrostPg2026x"
	@echo "==> Done. All cloud resources destroyed."

cloud-status:
	@echo "=== GKE Pods ==="
	@kubectl get pods -n bifrost-apps 2>/dev/null || echo "Not available"
	@echo ""
	@echo "=== GKE Services ==="
	@kubectl get svc -n bifrost-apps 2>/dev/null || echo "Not available"
	@echo ""
	@echo "=== Cloud SQL ==="
	@gcloud sql instances describe bifrost-db --format='table(state,ipAddresses[0].ipAddress)' 2>/dev/null || echo "Not running"
	@echo ""
	@echo "=== Health Check ==="
	@curl -s http://$(GATEWAY_URL)/health 2>/dev/null | jq . || echo "Gateway not reachable"

# Build and deploy all services (no infra changes)
cloud-deploy:
	docker build -t $(GATEWAY_IMG) gateway/
	docker push $(GATEWAY_IMG)
	docker build -t $(BUILDER_IMG) builder/
	docker push $(BUILDER_IMG)
	docker build -t $(VALIDATOR_IMG) validator/
	docker push $(VALIDATOR_IMG)
	gcloud run deploy bifrost-validator \
		--image=$(VALIDATOR_IMG) \
		--region=$(GCP_REGION) \
		--project=$(GCP_PROJECT) \
		--port=8090 \
		--allow-unauthenticated \
		--min-instances=0 \
		--max-instances=3 \
		--memory=256Mi \
		--cpu=1
	docker build -t $(REALTIME_IMG) realtime/
	docker push $(REALTIME_IMG)
	kubectl rollout restart deployment/bifrost-gateway -n bifrost-apps
	kubectl rollout restart deployment/bifrost-builder -n bifrost-apps
	kubectl rollout restart deployment/bifrost-realtime -n bifrost-apps
	@echo "==> Deployed. Waiting for rollout..."
	kubectl rollout status deployment/bifrost-gateway -n bifrost-apps --timeout=120s
	kubectl rollout status deployment/bifrost-builder -n bifrost-apps --timeout=120s
	kubectl rollout status deployment/bifrost-realtime -n bifrost-apps --timeout=120s

cloud-psql:
	PGPASSWORD='BfrostPg2026x' psql -h $$(terraform -chdir=infra output -raw db_ip) -U bifrost -d bifrost
