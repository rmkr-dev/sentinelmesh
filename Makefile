.DEFAULT_GOAL := help
PROFILE ?= azure-minimal
FAULT ?= payment-latency
SERVICE ?= orders
TEAM ?= checkout
ENVIRONMENT ?= local

.PHONY: help install lint test test-unit test-integration test-e2e security build \
	up down reset status logs demo fault-enable fault-disable \
	test-e2e-azure-replay test-e2e-kind \
	terraform-fmt terraform-validate azure-bootstrap azure-plan azure-apply azure-destroy \
	helm-lint docs onboard-service

help: ## Show targets
	@awk 'BEGIN {FS = ":.*##"; printf "Targets:\n"} /^[a-zA-Z0-9_-]+:.*?##/ {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install: ## Download Go modules
	go mod download

lint: ## Format check and go vet
	test -z "$$(gofmt -l .)"
	go vet ./...

test: test-unit ## Run the test suite

test-unit: ## Unit tests
	go test ./...

test-integration: ## Postgres integration tests. Skips when DATABASE_URL is empty.
	go test -tags=integration ./...

test-e2e-azure-replay: ## Azure and Kubernetes scenarios through the API and engine
	go test -count=1 ./internal/e2e/

test-e2e-kind: ## Kind cluster scenario. Requires kind and helm.
	./scripts/e2e-kind.sh

test-e2e: ## End-to-end demo against a running local stack
	./scripts/e2e.sh

security: ## Reachable vulnerability scan
	go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...

build: ## Build local binaries
	mkdir -p bin
	go build -o bin/platform ./cmd/platform
	go build -o bin/obsctl ./cmd/obsctl
	go build -o bin/demo ./cmd/demo
	go build -o bin/trafficgen ./cmd/trafficgen

up: ## Start the local platform
	docker compose up -d --build

down: ## Stop the local platform
	docker compose down

reset: ## Stop the local platform and delete volumes
	docker compose down -v

status: ## Compose status
	docker compose ps

logs: ## Follow compose logs
	docker compose logs -f --tail=100

demo: ## Run a scenario (SCENARIO=deployment-regression)
	SCENARIO=$${SCENARIO:-deployment-regression} ./scripts/e2e.sh

fault-enable: ## Enable a demo fault (FAULT=payment-latency)
	OBSCTL_API=$${OBSCTL_API:-http://localhost:8080} go run ./cmd/obsctl demo fault enable $(FAULT)

fault-disable: ## Disable a demo fault
	OBSCTL_API=$${OBSCTL_API:-http://localhost:8080} go run ./cmd/obsctl demo fault disable $(FAULT)

terraform-fmt: ## Format Terraform
	terraform fmt -recursive infra/terraform

terraform-validate: ## Validate Terraform without applying
	cd infra/terraform && terraform init -backend=false && terraform validate

azure-bootstrap: ## Print the Azure bootstrap steps
	@echo "See docs/deployment/azure.md and docs/deployment/github-oidc.md"
	@echo "Then: make azure-plan PROFILE=$(PROFILE)"

azure-plan: ## Terraform plan for PROFILE. Set TF_VAR_subscription_id or ARM_SUBSCRIPTION_ID.
	@test -n "$${TF_VAR_subscription_id:-$$ARM_SUBSCRIPTION_ID}" || { echo "set TF_VAR_subscription_id or ARM_SUBSCRIPTION_ID"; exit 1; }
	cd infra/terraform && terraform init -backend=false && terraform plan -var-file=profiles/$(PROFILE).tfvars -var=subscription_id=$${TF_VAR_subscription_id:-$$ARM_SUBSCRIPTION_ID}

azure-apply: ## Terraform apply for PROFILE
	@test -n "$${TF_VAR_subscription_id:-$$ARM_SUBSCRIPTION_ID}" || { echo "set TF_VAR_subscription_id or ARM_SUBSCRIPTION_ID"; exit 1; }
	cd infra/terraform && terraform init -backend=false && terraform apply -var-file=profiles/$(PROFILE).tfvars -var=subscription_id=$${TF_VAR_subscription_id:-$$ARM_SUBSCRIPTION_ID}

azure-destroy: ## Terraform destroy for PROFILE
	@test -n "$${TF_VAR_subscription_id:-$$ARM_SUBSCRIPTION_ID}" || { echo "set TF_VAR_subscription_id or ARM_SUBSCRIPTION_ID"; exit 1; }
	cd infra/terraform && terraform init -backend=false && terraform destroy -var-file=profiles/$(PROFILE).tfvars -var=subscription_id=$${TF_VAR_subscription_id:-$$ARM_SUBSCRIPTION_ID}

helm-lint: ## Lint the Helm chart
	helm lint helm/platform
	helm template platform helm/platform --namespace observability >/dev/null

docs: ## Check documentation links
	python3 scripts/validate-docs.py

onboard-service: ## Generate onboarding files
	go run ./cmd/obsctl service onboard --name $(SERVICE) --team $(TEAM) --environment $(ENVIRONMENT)
