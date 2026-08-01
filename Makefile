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
	pnpm turbo run test

.PHONY: lint
lint: ## Lint proto, Go, and frontend
	buf lint
	go vet ./...
	pnpm turbo run lint

.PHONY: build
build: ## Build Go binaries and frontend artifacts
	go build ./...
	pnpm turbo run build

# ---- Local Docker stack ---------------------------------------------------
# Full backend + web on one network (11 Go services hot-reloaded with air,
# Postgres, Pub/Sub emulator, Next.js web). Mobile runs natively against it.
COMPOSE ?= docker compose

# Local dev creds for compose interpolation. Never committed (.env is gitignored);
# `make run` materialises it from .env.example on first use.
.env:
	@cp .env.example .env
	@echo "created .env from .env.example (edit for non-default local creds)"

# gen/go is gitignored (produced by `buf generate`); go.work needs the module to
# exist. Drop an offline stub so the stack builds without buf. `make gen` writes
# the real generated code over it.
gen/go/go.mod:
	@mkdir -p gen/go
	@printf 'module github.com/garcia-lorca/gen/go\n\ngo 1.24\n' > $@
	@echo "created gen/go/go.mod stub (run 'make gen' for real codegen)"

.PHONY: run
run: .env gen/go/go.mod ## Levanta TODO el stack (build + up en segundo plano)
	$(COMPOSE) up -d --build
	@echo "→ web http://localhost:3000 · gateway http://localhost:8080 · postgres :5432"

.PHONY: run-fg
run-fg: .env gen/go/go.mod ## Igual que run pero en primer plano (logs en vivo)
	$(COMPOSE) up --build

.PHONY: run-new
run-new: .env gen/go/go.mod ## Borra TODO (contenedores + volúmenes) y reconstruye desde cero
	$(COMPOSE) down -v --remove-orphans
	$(COMPOSE) build --no-cache
	$(COMPOSE) up -d
	@echo "→ stack recreado desde cero"

.PHONY: up
up: run ## Alias de run (compat)

.PHONY: stop
stop: ## Detiene todos los contenedores (conserva datos y volúmenes)
	$(COMPOSE) stop

.PHONY: down
down: ## Detiene y elimina contenedores (conserva volúmenes/datos)
	$(COMPOSE) down --remove-orphans

.PHONY: restart
restart: ## Reinicia todo el stack
	$(COMPOSE) restart

.PHONY: ps
ps: ## Estado de los servicios
	$(COMPOSE) ps

.PHONY: logs
logs: ## Logs de todo el stack (follow)
	$(COMPOSE) logs -f --tail=120

.PHONY: gen
gen: ## buf generate dentro de un contenedor (no requiere buf en el host)
	docker run --rm -v "$(CURDIR)":/work -w /work bufbuild/buf:latest generate

.PHONY: psql
psql: ## Abre psql en la base local (rol app)
	$(COMPOSE) exec postgres psql -U app -d garcia_lorca

.PHONY: migrate
migrate: ## Aplica migrations/**/*.sql en la base local (orden léxico)
	$(COMPOSE) run --rm migrate

.PHONY: clean
clean: ## down -v + borra imágenes locales del proyecto
	$(COMPOSE) down -v --remove-orphans --rmi local

# ---- Per-service targets (usa _ : make start_iam, restart_gateway, logs_web) -
.PHONY: start_% stop_% restart_% rebuild_% logs_% sh_%
start_%: .env gen/go/go.mod ## Levanta un servicio: make start_iam
	$(COMPOSE) up -d $*

stop_%: ## Detiene un servicio: make stop_iam
	$(COMPOSE) stop $*

restart_%: ## Reinicia un servicio: make restart_gateway
	$(COMPOSE) restart $*

rebuild_%: .env gen/go/go.mod ## Reconstruye y relevanta un servicio: make rebuild_web
	$(COMPOSE) up -d --build --force-recreate --no-deps $*

logs_%: ## Logs de un servicio (follow): make logs_gateway
	$(COMPOSE) logs -f --tail=120 $*

sh_%: ## Shell dentro de un servicio: make sh_iam
	$(COMPOSE) exec $* bash

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
