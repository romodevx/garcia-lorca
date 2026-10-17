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
# buf from the host when it is installed, otherwise the same PINNED image CI
# uses. `make ci` has to work on the owner's laptop, where buf is not installed
# — a local gate nobody can run is not a gate.
#
# BUF_VERSION is kept EQUAL to the one in .github/workflows/ci.yml. The `gen`
# target below used to pull bufbuild/buf:latest, which is the exact shape of
# dependency that broke the deploy on 2026-09-10 (govulncheck@latest) and that
# security.md forbids.
BUF_VERSION ?= 1.50.0
BUF_DOCKER   = docker run --rm -v "$(CURDIR)":/work -w /work bufbuild/buf:$(BUF_VERSION)
BUF         ?= $(shell command -v buf >/dev/null 2>&1 && echo buf || echo '$(BUF_DOCKER)')

.PHONY: generate
generate: ## Regenerate gen/ from proto/ (buf; uses the pinned container if buf is not installed)
	$(BUF) lint
	$(BUF) generate

# ---- Quality --------------------------------------------------------------
# `./...` does NOT work in this repo. There is no root go.mod, so every Go
# command below used to die on its first line with:
#   pattern ./...: directory prefix . does not contain modules listed in go.work
# go.work is the single source of truth for which modules exist, so ask it
# rather than keeping a second list that drifts. Lazy (=, not :=) so `make help`
# does not shell out to go and jq.
GO_PKGS = $(shell go work edit -json | jq -r '.Use[].DiskPath + "/..."')

# Kept EQUAL to ACTIONLINT_VERSION in .github/workflows/ci.yml. Two linters at
# two versions is how local goes green and CI goes red for no visible reason.
ACTIONLINT_VERSION ?= v1.7.12

.PHONY: test
test: ## Run Go + frontend tests
	go work sync
	go test $(GO_PKGS) -race -count=1
	pnpm turbo run test

.PHONY: lint
lint: ## Lint proto, Go, and frontend
	$(BUF) lint
	go vet $(GO_PKGS)
	pnpm turbo run lint

.PHONY: build
build: ## Build Go binaries and frontend artifacts
	go build $(GO_PKGS)
	pnpm turbo run build

.PHONY: actionlint
actionlint: ## Lint the GitHub Actions workflows (pinned, installed on demand)
	@command -v actionlint >/dev/null 2>&1 \
		|| test -x "$$(go env GOPATH)/bin/actionlint" \
		|| go install github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)
	@if command -v actionlint >/dev/null 2>&1; then actionlint -color; \
	else "$$(go env GOPATH)/bin/actionlint" -color; fi

# The feedback loop this exists to close: 21 PR runs at ~48 billed min each,
# because the only way to find out CI is red was to push and wait. Runs the
# gates that need no Docker daemon and no network, so it stays under 5 minutes
# on a WARM machine (a cold Go build cache compiles the world and will not).
.PHONY: ci
ci: ## Fast local gate before pushing — OMITS integration tests, govulncheck, the source-map gate, buf breaking and the web production build (all still run in CI)
	@missing=""; \
	 command -v go   >/dev/null 2>&1 || missing="$$missing go"; \
	 command -v jq   >/dev/null 2>&1 || missing="$$missing jq"; \
	 command -v pnpm >/dev/null 2>&1 || missing="$$missing pnpm"; \
	 command -v buf  >/dev/null 2>&1 || docker info >/dev/null 2>&1 \
	   || missing="$$missing buf-or-a-running-docker"; \
	 if [ -n "$$missing" ]; then \
	   echo "missing:$$missing"; \
	   echo "  go   -> https://go.dev/dl/"; \
	   echo "  jq   -> brew install jq"; \
	   echo "  pnpm -> corepack enable pnpm"; \
	   echo "  buf  -> brew install bufbuild/buf/buf, or just start Docker (the"; \
	   echo "          pinned bufbuild/buf:$(BUF_VERSION) container is used instead)"; \
	   exit 1; \
	 fi
	@echo "==> generate"; $(MAKE) --no-print-directory generate
	@echo "==> actionlint"; $(MAKE) --no-print-directory actionlint
	@echo "==> lint"; $(MAKE) --no-print-directory lint
	@echo "==> test"; $(MAKE) --no-print-directory test
	@echo
	@echo "OK — the local gate passed. It did NOT run, and CI still will:"
	@echo "  - go test -tags=integration   (needs Docker + testcontainers)"
	@echo "  - govulncheck                 (needs the vuln database)"
	@echo "  - buf breaking                (needs the PR base revision)"
	@echo "  - turbo build + source-map gate (production artifacts)"

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

# ---- Modo producción local (deploy/PROD-LOCAL.md) --------------------------
# Las MISMAS imágenes de Cloud Run contra la Postgres local. Detiene los
# contenedores de la app dev mientras corre y los vuelve a arrancar en prod-down.
.PHONY: prod-build prod-up prod-status prod-logs prod-down prod-apk
prod-build: .env ## Construye las 10 imágenes de producción, una por una
	scripts/prod-local.sh build

prod-up: .env ## Levanta el stack en modo producción (web :13000, gateway :18080)
	scripts/prod-local.sh up

prod-status: ## Sondea cada puerto del stack en modo producción
	scripts/prod-local.sh status

prod-logs: ## Logs del stack en modo producción (follow)
	scripts/prod-local.sh logs

prod-down: ## Baja el modo producción y regresa los contenedores dev
	scripts/prod-local.sh down

prod-apk: ## APK release de la app de familias apuntando al gateway local
	scripts/mobile-release-apk.sh

.PHONY: gen
gen: ## buf generate dentro de un contenedor (no requiere buf en el host)
	$(BUF_DOCKER) generate

.PHONY: psql
psql: ## Abre psql en la base local (rol app)
	$(COMPOSE) exec postgres psql -U app -d garcia_lorca

.PHONY: migrate
migrate: ## Aplica migrations/**/*.sql en la base local (orden léxico)
	$(COMPOSE) run --rm migrate

# T11: lo que Cloud Scheduler hace cada mañana a las 7:00, a mano y en LOCAL.
# Sin DATE felicita a quien cumple HOY (hora de México); con DATE=2026-10-01
# repite ese día. Es idempotente: correrlo dos veces no felicita dos veces.
COMMS_LOCAL_URL ?= http://localhost:8096
.PHONY: birthdays
birthdays: ## Corre la felicitación de cumpleaños en local — make birthdays [DATE=aaaa-mm-dd]
	curl -sS -X POST -H 'Content-Type: application/json' \
	  -d '{"date":"$(DATE)"}' \
	  $(COMMS_LOCAL_URL)/garcialorca.comms.v1.CommsService/RunBirthdays; echo

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
# The ten services the deploy rolls out, in the order deploy-dev.yml uses them.
CLOUD_RUN_SERVICES = gateway iam students config finance grades attendance comms uniforms web

# `services update`, NOT `run deploy`. They are not interchangeable: the pipeline
# has always used `services update`, and every copy of this that drifted to
# `run deploy` was a second rollout implementation waiting to behave differently
# on the day somebody used it. One command, everywhere.
deploy-%: ## Deploy a service to Cloud Run, e.g. make deploy-gateway TAG=<sha>
	gcloud run services update $* \
		--project=$(PROJECT_ID) \
		--region=$(REGION) --quiet \
		--image=$(REGION)-docker.pkg.dev/$(PROJECT_ID)/$(AR_REPO)/$*:$(or $(TAG),local)

# Rehearsed 2026-09-19: 146 s to roll back, 157 s to roll forward, ten services
# each way, dev answering in both states, and ZERO billed Actions minutes.
#
# This is the rollback, not the deploy workflow. Re-running the workflow on an
# older commit does NOT work: workflow_dispatch takes the workflow file from the
# ref it runs on and only accepts a branch or a tag, never a bare SHA — so an
# old ref would run the OLD workflow. The images for any previously deployed
# commit are already in Artifact Registry; rolling back is pointing at them.
#
# Idempotent, which is why "run it again" is the answer to BOTH a rollback and a
# rollout that died halfway.
.PHONY: rollback-dev
rollback-dev: ## EMERGENCY: point dev at a previously deployed SHA — make rollback-dev TAG=<sha>
	@test -n "$(TAG)" || { echo "TAG is required, e.g. make rollback-dev TAG=8534a22b…"; \
	  echo "  candidates: gcloud artifacts docker tags list $(REGION)-docker.pkg.dev/$(PROJECT_ID)/$(AR_REPO)/web --project=$(PROJECT_ID)"; exit 1; }
	@echo "==> checking every image exists for $(TAG) before touching anything"
	@for svc in $(CLOUD_RUN_SERVICES); do \
	  gcloud artifacts docker images describe \
	    "$(REGION)-docker.pkg.dev/$(PROJECT_ID)/$(AR_REPO)/$$svc:$(TAG)" \
	    --project=$(PROJECT_ID) >/dev/null 2>&1 \
	    || { echo "  MISSING: $$svc:$(TAG) — half a rollback is worse than none"; exit 1; }; \
	done
	@echo "==> rolling back"
	@for svc in $(CLOUD_RUN_SERVICES); do \
	  printf '  %-11s ' "$$svc"; \
	  gcloud run services update "$$svc" --project=$(PROJECT_ID) --region=$(REGION) --quiet \
	    --image="$(REGION)-docker.pkg.dev/$(PROJECT_ID)/$(AR_REPO)/$$svc:$(TAG)" >/dev/null && echo ok; \
	done
	@echo "==> smoke (the same two checks the deploy workflow runs)"
	@WEB=$$(gcloud run services describe web --project=$(PROJECT_ID) --region=$(REGION) --format='value(status.url)'); \
	 GW=$$(gcloud run services describe gateway --project=$(PROJECT_ID) --region=$(REGION) --format='value(status.url)'); \
	 fail=0; \
	 for u in "$$WEB/login" "$$GW/readyz"; do \
	   code=$$(curl -s -o /dev/null -w '%{http_code}' --max-time 60 "$$u" || echo 000); \
	   printf '  %-52s -> %s\n' "$$u" "$$code"; \
	   [ "$$code" = "200" ] || fail=1; \
	 done; \
	 [ "$$fail" = "0" ] && echo "dev responde en $(TAG)" || { echo "::SMOKE FAILED::"; exit 1; }

# ---- Terraform ------------------------------------------------------------
.PHONY: deploy-preflight
deploy-preflight: ## Check every prerequisite the deploy assumes (read-only)
	PROJECT_ID=$(PROJECT_ID) REGION=$(REGION) sh scripts/deploy-preflight.sh

.PHONY: ci-minutes
ci-minutes: ## What CI and the deploy cost in billed Actions minutes (read-only)
	sh scripts/ci-minutes.sh

.PHONY: migrate-cloud
migrate-cloud: ## Apply migrations to a deployed env (needs DATABASE_URL)
	sh scripts/migrate-cloud.sh

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
