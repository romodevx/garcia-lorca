# García Lorca monorepo — thin task wrappers. Real logic lives in the tools
# (buf, go, turbo, terraform, gcloud); this file just gives memorable entry
# points used locally and mirrored by CI (deploy/cloudbuild/cloudbuild.yaml).

# Overridable on the command line: make tf-plan ENV=staging
REGION  ?= us-central1
AR_REPO ?= garcia-lorca
ENV     ?= dev
# Had NO default while REGION/AR_REPO/ENV did, so image-% and deploy-% silently
# built refs like us-central1-docker.pkg.dev//garcia-lorca/gateway:local — a
# double slash and no error.
PROJECT_ID ?= garcia-lorca
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
	@set -a; . ./.env 2>/dev/null; set +a; echo "→ web http://localhost:$${WEB_HOST_PORT:-3000} · gateway http://localhost:8080 · postgres :$${POSTGRES_HOST_PORT:-5432}"

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
.PHONY: tf-init
tf-init: ## terraform init for ENV (default dev) — pass BUCKET=gs-bucket-name
	@test -n "$(BUCKET)" || { echo "BUCKET is required, e.g. make tf-init BUCKET=garcia-lorca-tfstate"; exit 1; }
	terraform -chdir=$(TF_DIR) init \
		-backend-config=bucket=$(BUCKET) \
		-backend-config=prefix=cloud-run/$(ENV)

.PHONY: tf-plan
tf-plan: ## terraform plan for ENV (default dev)
	terraform -chdir=$(TF_DIR) plan -var-file=environments/$(ENV).tfvars

.PHONY: tf-apply
tf-apply: ## terraform apply for ENV (default dev)
	terraform -chdir=$(TF_DIR) apply -var-file=environments/$(ENV).tfvars

# ---- Deployed environment -------------------------------------------------
# Deliberately prefixed `cloud-` and NOT `up`/`down`: those already mean the
# LOCAL docker-compose stack in this file, and confusing `make down` (stops your
# laptop) with a command that takes the customer-facing demo offline is not a
# mistake worth leaving available at 11pm. Every underlying script passes
# --project and --region explicitly rather than inheriting gcloud config.
.PHONY: cloud-warm
cloud-warm: ## Warm web+gateway+iam before a demo (~$0.10 for a 3h window)
	PROJECT_ID=$(PROJECT_ID) REGION=$(REGION) sh scripts/cloud-warm.sh

.PHONY: cloud-cool
cloud-cool: ## Return every service to scale-to-zero after a demo
	PROJECT_ID=$(PROJECT_ID) REGION=$(REGION) sh scripts/cloud-cool.sh

.PHONY: cloud-stop
cloud-stop: ## PANIC: take the demo offline and cap instances (deletes nothing)
	PROJECT_ID=$(PROJECT_ID) REGION=$(REGION) sh scripts/cloud-stop.sh

.PHONY: cloud-start
cloud-start: ## Undo cloud-stop: put the demo back online
	PROJECT_ID=$(PROJECT_ID) REGION=$(REGION) sh scripts/cloud-start.sh

.PHONY: cloud-status
cloud-status: ## Show every service's min/max instances — is anything warm?
	@gcloud run services list --project=$(PROJECT_ID) --region=$(REGION) \
		--format='table(metadata.name,status.url,spec.template.metadata.annotations["autoscaling.knative.dev/minScale"]:label=MIN,spec.template.metadata.annotations["autoscaling.knative.dev/maxScale"]:label=MAX)'
