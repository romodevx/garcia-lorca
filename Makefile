# García Lorca monorepo — thin task wrappers. Real logic lives in the tools
# (buf, go, turbo, terraform, gcloud); this file just gives memorable entry
# points used locally and mirrored by CI (deploy/cloudbuild/cloudbuild.yaml).

# Overridable on the command line: make tf-plan ENV=staging
REGION  ?= us-central1
AR_REPO ?= garcia-lorca
ENV     ?= dev
TF_DIR  := deploy/terraform
TF_VARS := $(TF_DIR)/environments/$(ENV).tfvars

.DEFAULT_GOAL := help

.PHONY: help
help: ## List available targets
	@grep -hE '^[a-zA-Z_%-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

# ---- Codegen --------------------------------------------------------------
.PHONY: generate
generate: ## Regenerate gen/ from proto/ (buf)
	buf lint
	buf generate

# ---- Quality --------------------------------------------------------------
.PHONY: test
test: ## Run Go + frontend tests
	go work sync
	go test ./... -race -count=1
	npx turbo run test

.PHONY: lint
lint: ## Lint proto, Go, and frontend
	buf lint
	go vet ./...
	npx turbo run lint

.PHONY: build
build: ## Build Go binaries and frontend artifacts
	go build ./...
	npx turbo run build

# ---- Local ----------------------------------------------------------------
.PHONY: up
up: ## Start the local dev stack (docker compose)
	docker compose up --build

# ---- Container images -----------------------------------------------------
# Build a single service image from the repo root. Usage: make image-gateway
.PHONY: image-%
image-%: ## Build a service image, e.g. make image-gateway
	docker build -f services/$*/Dockerfile -t $(REGION)-docker.pkg.dev/$(PROJECT_ID)/$(AR_REPO)/$*:local .

# ---- Deploy ---------------------------------------------------------------
# Deploy one service's latest local image. Usage: make deploy-gateway ENV=dev
.PHONY: deploy-%
deploy-%: ## Deploy a service to Cloud Run, e.g. make deploy-gateway
	gcloud run deploy $* \
		--project=$(PROJECT_ID) \
		--region=$(REGION) \
		--image=$(REGION)-docker.pkg.dev/$(PROJECT_ID)/$(AR_REPO)/$*:$(or $(TAG),local)

# ---- Terraform ------------------------------------------------------------
.PHONY: tf-plan
tf-plan: ## terraform plan for ENV (default dev)
	terraform -chdir=$(TF_DIR) plan -var-file=environments/$(ENV).tfvars

.PHONY: tf-apply
tf-apply: ## terraform apply for ENV (default dev)
	terraform -chdir=$(TF_DIR) apply -var-file=environments/$(ENV).tfvars
