<!-- /autoplan restore point: /Users/andresromo/.gstack/projects/romodevx-garcia-lorca/dev-autoplan-restore-20260827-141455.md -->

# PLAN — Deploy `dev` a GCP (proyecto `garcia-lorca`), costo ~cero en reposo

Rama: `dev` · Proyecto destino: `garcia-lorca` (número `230382563795`) · Región: `us-central1`
Objetivo del dueño: **demo funcional end-to-end que no cobre nada mientras nadie la usa.**
No repetir el error de Heraldo (~700 MXN/mes).

---

## 0. Verdad de terreno (verificada, no supuesta)

Sondas ejecutadas con `gcloud` el 2026-08-27:

| Hecho | Evidencia |
|---|---|
| El proyecto `garcia-lorca` **ya existe** y está ACTIVE | `gcloud projects describe garcia-lorca` → `garcia-lorca 230382563795 ACTIVE` |
| Tiene billing habilitado, en la cuenta **Heraldo** `018CA3-FF15AB-FAA61D` | `gcloud beta billing projects describe garcia-lorca` → `billingEnabled: True` |
| La otra cuenta de facturación (`01483F-…`) está **CERRADA** (`OPEN=False`) | `gcloud beta billing accounts list` |
| El proyecto está **limpio**: sin Cloud Run, sin Cloud SQL, sin Artifact Registry | `gcloud services list --enabled` → solo BigQuery/Firebase/logging por defecto |
| **Heraldo tiene `minScale: 1` en sus DOS servicios** (`heraldo-api`, `heraldo-web`), cpu=1, 512Mi | `gcloud run services describe` (anotación `autoscaling.knative.dev/minScale: '1'`) |
| Heraldo **no tiene Cloud SQL** (`sqladmin` deshabilitado) → su DB es externa | `gcloud sql instances list` → API not enabled |
| No hay export de billing a BigQuery en ninguno de los dos proyectos | `bq ls` → vacío |

### La lección de Heraldo, con números

`minScale=1` significa que Google mantiene **una instancia viva 24/7** por servicio. Aunque
el CPU esté "throttled" en reposo, GCP cobra la tarifa *idle* de CPU y memoria todo el mes:

```
1 vCPU × 2,592,000 s/mes × $0.0000025/vCPU-s  ≈ $6.48
0.5 GiB × 2,592,000 s/mes × $0.00000025/GiB-s ≈ $0.32
                                        ─────────────
                             ≈ $6.80/mes POR SERVICIO
```

Dos servicios ≈ **$13.6/mes ≈ 250 MXN/mes que se pagan sin una sola visita.**
Eso explica una parte grande de los 700 MXN. El resto (Artifact Registry sin política de
limpieza, Cloud Build, logging, egress, la DB externa) **no lo puedo ver desde la CLI** —
se necesita Billing → Reports → agrupar por SKU en la consola. **Acción para el dueño:**
mirar ese reporte una vez, para cerrar el diagnóstico de Heraldo. Independientemente del
resultado, el diseño de abajo evita el driver que sí está confirmado.

### Consecuencia directa para García Lorca

La **capa gratuita de Cloud Run es por cuenta de facturación, no por proyecto**
(180k vCPU-s, 360k GiB-s, 2M requests al mes). Heraldo, con dos instancias siempre vivas,
ya se la come completa. **García Lorca arranca con cero colchón gratis.** No es un problema
—scale-to-zero cobra ~$0 igual— pero significa que *cada* request se factura desde el
primero, y que cualquier `min_instances=1` que se cuele aquí se paga íntegro.

---

## 1. Lo que ya está construido (no rehacer)

`deploy/terraform/**` está mucho más avanzado de lo que sugiere el nombre. Ya existe y está
verificado (`terraform validate` OK, mergeado a `dev` el 2026-08-24):

- Módulo `cloud_run` reutilizable con perfil de costo por variable.
- Los **8 servicios reales** instanciados: `gateway`, `iam`, `students`, `config`, `finance`,
  `grades`, `attendance`, `comms`. Los 4 stubs (`billing`, `scholarships`, `docs`, `uniforms`)
  deliberadamente **fuera**.
- Wiring completo gateway→internos **y** interno→interno (`*_BASE_URL` por servicio),
  con `roles/run.invoker` al runtime SA (sin conector VPC — más barato).
- Auth servicio-a-servicio por OIDC (`pkg/gcpauth`), DSN de Cloud SQL armado desde env
  partido (`pkg/dbdsn`).
- Pub/Sub declarativo: 3 topics + DLQ + suscripciones **push autenticadas** (`comms`,
  `students`, `finance`) — push, no pull, precisamente para que sobrevivan a scale-to-zero.
- Artifact Registry **con política de limpieza** (5 versiones, borra untagged >30d).
- Budget filtrado **por proyecto** (`budget_filter.projects`), así que la alerta de García
  Lorca no se contamina con el gasto de Heraldo aunque compartan cuenta.
- `environments/dev.tfvars.example` ya trae el preset barato completo.
- Cloud Build sin `machineType` (máquina default = elegible para los 120 min/día gratis).

**Esto no se replantea.** El plan de abajo son los huecos que faltan para que aplique y sirva.

---

## 2. Bloqueadores encontrados en esta revisión (nuevos)

### B1 — `terraform apply` va a FALLAR en Cloud SQL 🔴

`deploy/terraform/database.tf:33-37`:

```hcl
ip_configuration {
  ipv4_enabled = false
  # private_network = "..."   # <-- comentado
}
```

Cloud SQL exige **al menos una** de IP pública o red privada. Con las dos ausentes la API
rechaza la creación. Además, el conector nativo de Cloud Run (`/cloudsql` socket) usa la ruta
pública autenticada; sin IP pública y sin Direct VPC egress, ningún servicio conectaría.

**Fix:** `ipv4_enabled = true` **sin `authorized_networks`** (lista vacía) + `ssl_mode`
exigiendo TLS. Nadie entra desde internet (no hay red autorizada); Cloud Run y el proxy local
entran por IAM vía Cloud SQL Auth Proxy. La IP pública de Cloud SQL no se factura aparte.
Red privada quedaría para prod (necesita VPC + Direct VPC egress = más costo y complejidad).

### B2 — El frontend web no tiene ningún camino de deploy 🔴

`apps/web` (Next.js 16) **no tiene Dockerfile**, **no está en `services.tf`**, **no está en
`cloudbuild.yaml`**. Verificado: `find . -name "Dockerfile*"` no lista `apps/web`; `grep -rn
"web" deploy/terraform/` → 0 resultados.

Sin esto **no hay demo**: los 8 servicios backend quedarían corriendo sin nada que mirar.
`next.config.js` ya tiene `output: "standalone"` (imagen chica, arranque rápido) — la mitad
del trabajo está hecha, falta el Dockerfile, el módulo de TF y el paso de build.

### B3 — El login se rompe entre dos dominios `*.run.app` 🔴

`services/gateway/internal/handler/auth.go:93-104`: las cookies son
`HttpOnly + Secure + SameSite=Strict`, sin atributo `Domain` (host-only).
El navegador llama al gateway **directo** desde el cliente
(`apps/web/app/AppShell.tsx:38`: `NEXT_PUBLIC_API_URL`).

`run.app` está en la Public Suffix List: `web-xxx.run.app` y `gateway-yyy.run.app` son
**sitios distintos**. Con `SameSite=Strict` el navegador nunca manda la cookie. Login
aparentemente exitoso, `/me` siempre 401. Esto no es teoría: es el modo de falla exacto que
produce la combinación actual.

**Dos salidas:**

- **(a) Proxy same-origin en Next.js — $0, recomendada.** `rewrites()` mapea
  `/api/:path*` → `https://gateway-….run.app/api/:path*`, y `NEXT_PUBLIC_API_URL` pasa a ser
  relativo. El navegador solo habla con el origen del web; el `Set-Cookie` aterriza ahí como
  cookie de primera parte. Paths preservados (`/` para `gl_access`, `/api/v1/auth` para
  `gl_refresh`, ver `auth.go:64`). Sin dominio, sin DNS, sin costo. Cuesta un salto extra
  (dos cold starts encadenados en la primera visita del día).
- **(b) Dominio propio + domain mappings de Cloud Run.** `app.midominio.mx` y
  `api.midominio.mx` comparten `midominio.mx` → same-site. Más limpio y sin salto extra, pero
  cuesta el dominio (~$12/año), DNS, y los domain mappings de Cloud Run tienen disponibilidad
  irregular por región.

**Mina asociada a (a):** `apps/web/app/flags-provider.tsx:29` hace
`IS_DESIGN_DEMO = API_URL === ""`. Si se pone `NEXT_PUBLIC_API_URL=""` la app entra en **modo
mock** y la demo enseña datos falsos. Hay que usar un valor relativo no vacío y verificar
cómo concatenan las URLs los helpers (`fetchFeatures`, `createTransport`).

### B4 — Las migraciones no tienen camino en GCP 🟡

`docker/apply-migrations.sh` asume `psql` contra el Postgres de compose. En GCP no hay nada
que aplique `migrations/**` (11 dominios) ni `deploy/terraform/db/setup_app_role.sql`, ni que
siembre al dueño. Sin esto la base queda vacía y todo responde error.

**Fix barato:** Cloud SQL Auth Proxy desde la laptop (posible una vez resuelto B1) +
`apply-migrations.sh` apuntando a `127.0.0.1`. Cero infra nueva, cero costo. Un Cloud Run Job
sería más repetible, pero es infra extra para un paso que se corre dos veces al año.

### B5 — APIs que Terraform necesita y no declara 🟡

`main.tf` habilita 8 APIs. Faltan `cloudresourcemanager.googleapis.com` (lo usa
`data "google_project" "current"` en `pubsub.tf:78`) y `monitoring.googleapis.com` (canales de
notificación del budget). `monitoring` ya está activa en el proyecto por herencia de Firebase;
`cloudresourcemanager` **no aparece** en la lista de habilitadas. Agregarlas al `for_each`.

### B6 — `ENABLED_MODULES` está desactualizado 🟡

`services.tf:47` lista `iam,gateway,students,config,grades,finance,attendance,comms`.
`FLAGS.md` ya registra `module_teachers` y `module_tuition` (M14/M15, mergeados). Si el gateway
aplica default-deny, **Docentes y Colegiaturas quedan invisibles en la demo** — dos de los
módulos más nuevos y más vendibles.

---

## 3. Arquitectura destino y modelo de costo

```
                    Internet
                       │
                       ▼
        ┌──────────────────────────────┐
        │  web (Next.js standalone)    │  Cloud Run · min=0 · max=2 · cpu_idle
        │  rewrites /api/* ──────────┐ │  ← único origen que ve el navegador
        └────────────────────────────┼─┘
                                     ▼
        ┌──────────────────────────────┐
        │  gateway (Connect/HTTP+JSON) │  Cloud Run · min=0 · max=1 · público
        └──────────────┬───────────────┘  (max=1: el rate limiter es in-process)
                       │ OIDC (runtime SA)
      ┌────────┬───────┼────────┬─────────┬──────────┐
      ▼        ▼       ▼        ▼         ▼          ▼
    iam    students  config  grades   finance   attendance   comms
   ── todos: Cloud Run · min=0 · cpu_idle=true · ingress interno · max=3 ──
      │        │                          ▲          │          ▲
      └────────┴──── Cloud SQL ───────────┘          │          │
                  (socket /cloudsql)                 │          │
                                              Pub/Sub push (OIDC)
                                     payment-events · student-events · attendance-events
```

### Costo mensual estimado — dev, con la demo dormida

| Recurso | Config | Reposo | Notas |
|---|---|---|---|
| 9 servicios Cloud Run | `min=0`, `cpu_idle=true` | **$0.00** | No hay instancia viva. Cero. |
| Cloud SQL `db-f1-micro` | ZONAL, 10 GB SSD, PITR off, 3 backups | **~$9.70** | vCPU/RAM ~$7.67 + disco ~$1.70 + backups ~$0.30 |
| Artifact Registry | política: 5 versiones, borra untagged >30d | ~$0.20 | acotado por la política |
| Pub/Sub | 3 topics + DLQ, sin tráfico | $0.00 | 10 GiB/mes gratis |
| Secret Manager | 2 secretos | ~$0.12 | $0.06 por secreto activo |
| Cloud Build | máquina default | $0.00 | 120 min/día gratis (aún no consumidos) |
| Logging | retención 30 d | $0.00 | dentro de 50 GiB gratis |
| **Total en reposo** | | **≈ $10 USD ≈ 185 MXN/mes** | |

Con uso real de demo (unas cientos de requests al día): **+$0 a $3**.
Contra Heraldo: **185 MXN vs 700 MXN**, y el 100% del piso es la base de datos.

### El único gasto que no baja a cero: Cloud SQL

El dueño pidió literal *"no consuma nada de gasto mientras no se usa"*. **Cloud SQL no puede
apagarse solo.** Es un servidor: cobra estés o no. Ese ~$9.70 es el piso real, y es la
decisión abierta más importante (§5, D1). Alternativa a $0: **Neon free tier** — Postgres 17
real, auto-suspende a los 5 minutos sin conexiones y auto-despierta con la siguiente, RLS y
`SET LOCAL app.current_tenant` funcionan sin cambiar una línea. Se paga con salir de GCP y
conectar por TLS público.

---

## 4. Runbook de ejecución

### Fase A — Cerrar bloqueadores en el repo (todo en `dev`, TDD donde aplique)

| # | Tarea | Archivos |
|---|---|---|
| A1 | Arreglar `ip_configuration`: `ipv4_enabled = true`, `authorized_networks` vacío, `ssl_mode = "ENCRYPTED_ONLY"` | `deploy/terraform/database.tf` |
| A2 | `Dockerfile` para `apps/web` (multi-stage pnpm → `.next/standalone`, non-root, `PORT` de Cloud Run) | `apps/web/Dockerfile` |
| A3 | Módulo TF `web`: `min=0`, `max=2`, `cpu_idle=true`, `ingress=ALL`, `allow_unauthenticated=true`, sin Cloud SQL, sin secretos | `deploy/terraform/services.tf` |
| A4 | `rewrites()` `/api/:path*` → `GATEWAY_URL`, y `NEXT_PUBLIC_API_URL` relativo. **Test que falle primero** cubriendo el bug de `IS_DESIGN_DEMO` | `apps/web/next.config.js`, `apps/web/app/flags-provider.tsx` |
| A5 | Paso `image-web` + deploy de `web` en Cloud Build | `deploy/cloudbuild/cloudbuild.yaml` |
| A6 | Agregar `cloudresourcemanager` y `monitoring` al `for_each` de APIs | `deploy/terraform/main.tf` |
| A7 | Añadir `teachers,tuition` a `ENABLED_MODULES` (verificar antes cómo los nombra el gateway) | `deploy/terraform/services.tf` |
| A8 | `scripts/migrate-cloudsql.sh`: levanta Cloud SQL Auth Proxy, corre `setup_app_role.sql` + `migrations/**` + seed del dueño | `scripts/` |
| A9 | `dev.tfvars` real (gitignored) con `project_id = "garcia-lorca"` y el preset barato | `deploy/terraform/environments/` |

### Fase B — Aplicar la infraestructura

1. `gcloud config set project garcia-lorca`
2. Crear bucket de estado: `gs://garcia-lorca-tfstate` (misma región, versioning ON, uniform access)
3. `terraform init -backend-config=bucket=garcia-lorca-tfstate -backend-config=prefix=cloud-run/dev`
4. `terraform plan -var-file=environments/dev.tfvars` → **revisar a mano que ningún `min_instances` sea > 0**
5. `TF_VAR_owner_temp_password=… terraform apply -var-file=environments/dev.tfvars`

`dev.tfvars` (los valores que importan):

```hcl
project_id = "garcia-lorca"          # el proyecto YA EXISTE — no crear uno nuevo
env        = "dev"
region     = "us-central1"
billing_account_id = "018CA3-FF15AB-FAA61D"

db_tier                           = "db-f1-micro"
db_availability_type              = "ZONAL"
db_point_in_time_recovery_enabled = false
db_backup_retained_count          = 3

gateway_min_instances = 0    # ← el error de Heraldo, explícitamente evitado
gateway_max_instances = 1
gateway_cpu_idle      = true
gateway_startup_cpu_boost = false
iam_min_instances     = 0    # ← idem
iam_cpu_idle          = true
iam_startup_cpu_boost = false

monthly_budget_amount = 20
budget_alert_emails   = ["andres.roqa93@gmail.com"]
seed_owner_secret     = true
billing_schedule_enabled = false     # el job mensual queda APAGADO en demo
```

### Fase C — Datos

6. Escribir el password de la app en Secret Manager (lo crea Terraform, hay que darle valor)
7. `scripts/migrate-cloudsql.sh` → `setup_app_role.sql` + `migrations/**` + semilla del dueño
8. Sembrar datos de demo (`pnpm demo:ready` apuntado a Cloud SQL vía proxy)

### Fase D — Imágenes y verificación

9. Cloud Build con `_DEPLOY=true` (9 imágenes) — o `gcloud builds submit` a mano la primera vez
10. Verificación end-to-end en la URL del web: login del dueño → cambio de contraseña →
    ciclo/catálogo → alumnos → calificaciones → asistencia → adeudos → notificación
11. Confirmar en Cloud Run que **las 9 revisiones muestran `minScale` vacío o 0**
12. Cerrar el turno: `gcloud run services list` a las 24 h y confirmar `$0` de CPU idle

### Fase E — Interruptor de emergencia

Si algo se dispara, el corte total es un comando (la base sobrevive):

```bash
for s in web gateway iam students config finance grades attendance comms; do
  gcloud run services update $s --region=us-central1 --min-instances=0 --max-instances=0
done
gcloud sql instances patch garcia-lorca-dev --activation-policy=NEVER   # apaga la DB
```

`--activation-policy=NEVER` detiene la instancia: se sigue pagando el disco (~$1.70/mes) pero
no el cómputo. Volver: `--activation-policy=ALWAYS`.

---

## 5. Decisiones abiertas para el dueño

**D1 — La base de datos (la única que no baja a cero).**
Cloud SQL `db-f1-micro` ≈ $9.70/mes (185 MXN) con paridad dev/prod total, **vs** Neon free
tier = $0, auto-suspende, Postgres real, RLS intacta, pero fuera de GCP y por TLS público.
En la memoria del proyecto ya había una decisión previa de *quedarse con Cloud SQL por
paridad*; la instrucción nueva ("que no consuma nada en reposo") la contradice. Hay que
resolverla explícitamente.

**D2 — Dominio propio o proxy en Next.js** (§B3). El proxy es $0 y funciona hoy; el dominio
es más limpio y ~$12/año.

**D3 — Cuenta de facturación.** Hoy `garcia-lorca` cuelga de la cuenta **Heraldo**, y la otra
cuenta está cerrada. Funciona (el budget filtra por proyecto), pero mezcla el gasto personal
del dueño con el de Heraldo en la misma factura. ¿Se deja así o se abre una cuenta aparte?

**D4 — `master`/escuela.** Fuera del alcance de este plan. Mismo Terraform + `prod.tfvars` en
el proyecto de la escuela, estado aislado, REGIONAL, `min=1` en gateway+iam. Se hace cuando
la escuela contrate.

---

## 6. Fuera de alcance (explícito)

- Deploy de `master` a la cuenta de la escuela (D4).
- Los 4 servicios stub (`billing`, `scholarships`, `docs`, `uniforms`) — flags OFF, sin imagen.
- App móvil Expo (no consume infra de GCP; solo apunta al gateway).
- Load Balancer / CDN / Cloud Armor (~$18/mes de LB — no se justifica en demo).
- HA, réplicas de lectura, PITR (prod).
- El `billing_schedule` mensual de M15 (queda apagado; un job que factura el ciclo equivocado
  es peor que no tener job).

---
---

# REVISIÓN /autoplan — FASE 1: CEO (estrategia y alcance)

Modo: **SELECTIVE EXPANSION** (baseline = alcance actual; las expansiones se ofrecen una por una).
Voces duales: Claude subagente ✅ · Codex ❌ `[codex-unavailable: binary not found]` → **[subagent-only]**.

## Auditoría previa del sistema

- `git log -15`: la rama va de M14/M15 (Docentes, Colegiaturas) a fixes de datos de demo.
  El último trabajo real fue **hacer que la demo se vea bien**, no infraestructura.
- `git diff master --stat`: **`dev` está a 1 commit de `master`** (solo `CLAUDE.md`). No hay
  divergencia: desplegar `dev` es desplegar el estado actual del producto. Riesgo de deriva: cero.
- `git stash list`: vacío. Nada a medias.
- **No existe `TODOS.md`.** Todo lo que esta revisión difiera necesita un archivo donde vivir,
  o se pierde (Directiva 7: "lo diferido sin escribir es mentira").
- Archivos más tocados en 30 días: `services/gateway/cmd/main.go` (25),
  `apps/web/app/AppShell.tsx` (25). El gateway y el shell del web son las piezas calientes —
  y son exactamente las dos que este deploy toca.

### Chequeo retrospectivo

`deploy/COST-AND-DEPLOY.md:22` todavía dice *"solo gateway/iam tienen Dockerfiles"* y
*"config/finance/grades/attendance/comms NOT written (TODO comment)"*. **Ese texto está
obsoleto**: `services.tf` ya instancia los 8 y los 8 tienen Dockerfile. El documento de costos
quedó desfasado de su propio Terraform. Un documento de costos que miente sobre el inventario
es cómo se cuelan sorpresas en la factura. **Corregirlo es parte de este trabajo.**

### Calibración de gusto (referencias del propio repo)

Bien diseñado, imitar: `deploy/terraform/modules/cloud_run` (un módulo, perfil de costo por
variable — por eso agregar `web` es copiar un bloque, no diseñar nada); `pubsub.tf` (topic +
DLQ + suscripción push, repetido idéntico 3 veces, con el *porqué* escrito en el comentario).
Antipatrón a no repetir: `COST-AND-DEPLOY.md` describiendo un estado que ya no existe.

### Chequeo de panorama (tres capas)

- **[Capa 1 — lo probado]** Demo SaaS = un contenedor + una Postgres gestionada. Render,
  Fly.io, Railway lo hacen en minutos.
- **[Capa 2 — lo popular]** "Scale-to-zero" se vende como gratis; casi nunca lo es, porque la
  base de datos no baja a cero. Es el error que casi todos cometen.
- **[Capa 3 — primeros principios]** El costo de este sistema **no es proporcional al número
  de servicios, sino al número de cosas que no pueden apagarse.** Nueve servicios Cloud Run a
  `min=0` cuestan lo mismo que cero servicios: **nada.** Una sola Cloud SQL cuesta el 100% del
  piso. La intuición de "son muchos microservicios, va a salir caro" es **falsa** aquí, y la
  de "es solo una base chiquita, no importa" es la cara.

> **Eureka:** en una arquitectura scale-to-zero el costo no se cuenta en servicios, se cuenta
> en cosas que no se apagan. García Lorca tiene 9 servicios y **exactamente un** costo: la DB.

---

## 0A. Desafío de premisas

| # | Premisa | ¿Declarada o asumida? | Veredicto |
|---|---|---|---|
| P1 | El dueño quiere **una demo**, no producción | Asumida (dicha en el mensaje: *"para un deploy demo"*) | ✅ Válida |
| P2 | El error de Heraldo fue Cloud Run siempre caliente | **Verificada**: `minScale: '1'` en los dos servicios | ✅ Válida, con evidencia |
| P3 | Cloud SQL es el único piso que no baja a cero | **Verificada** (es un servidor gestionado) | ✅ Válida |
| P4 | Desplegar los 9 servicios cuesta lo mismo que desplegar 1 | Válida: Cloud Run cobra por request/CPU-s, no por servicio | ✅ Válida |
| P5 | El proyecto `garcia-lorca` está listo para recibir | **Verificada**: existe, ACTIVE, con billing, vacío de infra | ✅ Válida |
| P6 | *"La IaC está lista, solo falta aplicar"* (memoria del proyecto) | **Asumida — y FALSA** | ❌ Refutada: §B1 rompe el apply, §B2 no despliega el front, §B3 rompe el login |
| P7 | El backend desplegado = demo funcional | **Asumida — y falsa** | ❌ Sin `web` no hay nada que enseñar |
| P8 | 700 MXN de Heraldo = solo Cloud Run | Parcial | ⚠️ `min=1` explica ~250 MXN. El resto **no es visible por CLI**; falta mirar Billing→Reports por SKU |

**Las tres preguntas:**

1. **¿Es el problema correcto?** Sí, con una corrección de encuadre. El problema no es
   *"desplegar el backend en GCP"* — es **"que el director de la escuela abra una liga, entre,
   y vea su escuela funcionando, sin que eso me cueste dinero mientras nadie mira."** Con ese
   encuadre, `web` deja de ser un pendiente y se vuelve **la mitad del entregable**, y el login
   entre dominios (§B3) deja de ser un detalle técnico y se vuelve **el bug que mata la demo**.
2. **¿Cuál es el resultado real?** Que la escuela firme. La demo es un instrumento de venta.
   Todo lo que no se ve en pantalla no cuenta.
3. **¿Y si no hacemos nada?** El sistema queda en `localhost`. La escuela nunca lo toca sola,
   solo lo ve por videollamada. Dolor real, no hipotético.

---

## 0B. Mapa de código existente (qué ya está resuelto)

| Sub-problema | ¿Ya existe? | Dónde |
|---|---|---|
| Definir 8 servicios en Cloud Run con perfil de costo | ✅ Completo | `services.tf`, `modules/cloud_run` |
| Escalado a cero por variable | ✅ Completo | `variables.tf:88-133`, `environments/dev.tfvars.example` |
| Auth servicio-a-servicio (OIDC) | ✅ Completo | `pkg/gcpauth`, `services.tf:internal_invoker` |
| DSN de Cloud SQL desde env partido | ✅ Completo | `pkg/dbdsn` |
| Eventos que sobreviven a scale-to-zero | ✅ Completo (push + DLQ + OIDC) | `pubsub.tf` |
| Limpieza de imágenes | ✅ Completo | `main.tf:49-66` |
| Alerta de presupuesto por proyecto | ✅ Completo | `budget.tf` |
| Build multi-servicio + gates | ✅ Completo (8 servicios) | `cloudbuild.yaml` |
| Imagen del web | ❌ **No existe** | — |
| Servicio `web` en Terraform | ❌ **No existe** | — |
| Origen compartido web↔gateway | ❌ **No existe** | — |
| Migraciones contra Cloud SQL | ❌ **No existe** (solo compose) | `docker/apply-migrations.sh` |
| IP de Cloud SQL | ❌ **Rota** | `database.tf:33-37` |

**¿Se está reconstruyendo algo?** No. Los 4 huecos son huecos reales, no duplicados. El plan
**suma** al Terraform existente; no lo reescribe. Correcto.

---

## 0C. Estado soñado

```
  HOY                          ESTE PLAN                    IDEAL A 12 MESES
  ─────────────────────        ────────────────────         ─────────────────────
  Solo corre en localhost      Una URL pública que          Dos entornos: demo del
  vía docker-compose.          la escuela abre sola.        dueño + producción de la
                                                            escuela, mismo Terraform.
  IaC escrita pero nunca       9 servicios a min=0 +        Prod: gateway+iam calientes,
  aplicada; 3 bugs que         1 Cloud SQL. ~185 MXN/mes    DB REGIONAL, PITR on.
  nadie descubrió porque       en reposo, verificado.       Demo: sigue en ~0.
  nunca se aplicó.
                                                            CI que promueve
  Sin camino de despliegue     Cloud Build construye 9      branch→dev→master con
  para el front.               imágenes; el web es una      un apply por rama.
                               más.
```

**Delta:** el plan mueve **hacia** el ideal. El Terraform multi-entorno ya soporta la
promoción a la escuela (`project_id`/`billing_account_id`/estado por entorno,
`deletion_protection` automático cuando `env==prod`). Nada de lo que se hace aquí hay que
deshacerlo después. **Única deuda que sí crea:** si D1 resuelve "Neon", el entorno de la
escuela usará Cloud SQL y la demo no — se pierde paridad y hay que mantener dos rutas de DSN.

---

## 0C-bis. Alternativas de implementación (obligatorio)

```
ENFOQUE A: "Aplicar el Terraform que ya existe, tapando solo lo que impide aplicar"
  Resumen: arreglar B1 (IP de Cloud SQL) y aplicar. Backend vivo, sin front.
  Esfuerzo: S   (humano ~2 h / CC ~10 min)
  Riesgo:   Alto
  Pros:     - El camino más corto a "hay algo en GCP".
            - Toca un solo archivo.
  Contras:  - NO HAY DEMO. Nueve servicios internos y ninguna pantalla.
            - El login sigue roto (§B3), así que ni con curl se prueba el flujo real.
            - Se descubre lo que falta el día de la junta con la escuela.
  Reusa:    todo el Terraform actual.

ENFOQUE B: "Deploy demo completo — front incluido, mismo origen, migraciones desde la laptop"
  Resumen: B1+B2+B3+B4+B5+B6. Dockerfile del web, módulo TF `web`, proxy same-origin en
           Next.js, script de migración por Cloud SQL Auth Proxy, APIs faltantes,
           `teachers` en ENABLED_MODULES. Todo a min=0.
  Esfuerzo: M   (humano ~3 días / CC ~2-3 h)
  Riesgo:   Bajo
  Pros:     - Es lo único que produce una demo que la escuela puede abrir sola.
            - Cero infra nueva de pago: sin Load Balancer, sin dominio, sin conector VPC.
            - Cada pieza es copiar un patrón que ya existe en el repo.
            - Deja el camino a producción intacto (mismo TF + prod.tfvars).
  Contras:  - Un salto extra de red (web → gateway) en la primera visita del día.
            - Toca ~8 archivos en vez de 1.
  Reusa:    modules/cloud_run, cloudbuild.yaml, apply-migrations.sh, output:standalone.

ENFOQUE C: "Salirse de GCP — Render/Fly.io + Neon, un contenedor por servicio"
  Resumen: abandonar el Terraform y desplegar en una PaaS con free tier.
  Esfuerzo: L   (humano ~1 semana / CC ~1 día)
  Riesgo:   Alto
  Pros:     - Podría costar $0 real, incluida la base.
            - Menos ceremonia que Terraform + Cloud Build.
  Contras:  - Tira a la basura un Terraform funcional y ya revisado.
            - Rompe el modelo de dos cuentas: la escuela SÍ va a querer GCP o similar,
              y habría que construir el camino a producción dos veces.
            - Los free tiers de PaaS duermen agresivo: la demo tarda 30-60 s en despertar,
              peor que un cold start de Cloud Run.
  Reusa:    los Dockerfiles de los servicios. Nada más.
```

**RECOMENDACIÓN: ENFOQUE B.** Es el único que cumple la definición real de "hecho" (la escuela
abre una liga y entra), reusa todo lo construido, y no compra ni un recurso que cobre en reposo.
A es una demo que no se puede demostrar; C paga una semana por ahorrar ~$10 al mes y hace el
trabajo de producción dos veces.

**Decisión automática (P1 completitud + P2 boil the ocean):** ENFOQUE B.
No es cerrada — A y B difieren en si hay demo o no, no en gusto. **Mecánica, no de gusto.**

---

## 0D. Análisis del modo (SELECTIVE EXPANSION)

**1. Chequeo de complejidad.** El plan toca ~9 archivos y agrega **un** servicio Cloud Run
nuevo (`web`) y **un** script. Bajo el umbral de 8 archivos por poco, pero cada archivo es una
pieza distinta y necesaria: quitar cualquiera rompe la demo. Sin humo.

**2. Mínimo que logra el objetivo.** B1 + B2 + B3 + B4 + B6. Es decir: que aplique, que haya
front, que el login funcione, que haya datos, y que Docentes no esté ausente.
**B5 (APIs faltantes)** es barato y evita un apply fallido a mitad de camino → se queda.

**3. Escaneo de expansiones (candidatas, no aceptadas todavía):**

- **E1 — Apagado nocturno automático de Cloud SQL.** Cloud Scheduler apaga la instancia a las
  22:00 y la enciende a las 7:00. Baja el piso de ~$9.70 a ~$4/mes. (S / riesgo medio: si la
  escuela abre la demo a las 23:00, ve un error.)
- **E2 — Un `make demo-up` / `make demo-down` de un comando.** El interruptor de emergencia de
  §E, pero como target versionado en vez de un bloque de bash en un `.md`. (S / riesgo bajo)
- **E3 — Semilla automática al desplegar.** Que la demo siempre tenga la escuela completa
  (400 alumnos, 20 docentes), reconstruible con un comando. (M / riesgo bajo)
- **E4 — Página de "despertando…" durante el cold start.** El primer request del día tarda
  2-5 s encadenando dos servicios fríos. Sin retroalimentación, el director cree que está roto.
  (S / riesgo bajo, **impacto alto en la venta**)
- **E5 — Alerta de presupuesto a los $5, no solo a los $20.** El umbral del 50% sobre $20 son
  $10 — ya es el doble de lo esperado. Un budget de $15 avisa antes. (XS / riesgo nulo)
- **E6 — Corregir `COST-AND-DEPLOY.md`,** que describe un inventario que ya no existe. (XS)

**Decisiones automáticas de la ceremonia de cherry-pick** (P2: dentro del radio de impacto y
<1 día de CC → aprobar; fuera → diferir):

| # | Expansión | Decisión | Principio | Razón |
|---|---|---|---|---|
| E1 | Apagado nocturno de la DB | **DIFERIR** | P3 pragmático | Ahorra ~$5/mes y arriesga que la demo esté muerta cuando la escuela la abra. La venta vale más que $5. Reconsiderar si la demo dura meses. |
| E2 | `make demo-up` / `demo-down` | **ACEPTAR** | P2 radio de impacto | 1 archivo, ya existe `Makefile`. El interruptor de pánico tiene que ser un comando, no un bloque de bash copiado de un `.md` a las 11 de la noche. |
| E3 | Semilla automática | **ACEPTAR** | P1 completitud | Ya existe `pnpm demo:ready`. Solo hay que apuntarlo a Cloud SQL. Una demo vacía no vende. |
| E4 | Estado de "despertando" en el primer request | **ACEPTAR** | P1 + venta | El cold start es consecuencia directa de la decisión de costo. Si se elige min=0, hay que diseñar la espera. Sin esto el ahorro se paga con la impresión del cliente. |
| E5 | Budget a $15 | **ACEPTAR** | P2, costo cero | Cambiar un número en `dev.tfvars`. |
| E6 | Corregir el doc de costos | **ACEPTAR** | P4 DRY | Un documento de costos desactualizado es cómo vuelve el problema. |

**E4 se marca TASTE DECISION**: la alternativa razonable es dejar el cold start desnudo y
aceptar 2-5 s en blanco. Gente sensata discreparía sobre si vale el trabajo.

---

## 0E. Interrogatorio temporal

| Momento | Lo que hay que decidir AHORA, no durante |
|---|---|
| **Hora 1 — cimientos** | ¿`project_id = "garcia-lorca"` (existente) o un `garcia-lorca-dev` nuevo? → **el existente**: ya tiene billing y está vacío. ¿Bucket de estado en el mismo proyecto? → sí, `gs://garcia-lorca-tfstate`, versioning ON. |
| **Horas 2-3 — núcleo** | El Dockerfile del web tiene que compilar un **monorepo pnpm con workspaces** (`@garcia-lorca/ui`, `domain`, `i18n`, `api-client`). Un `COPY apps/web` pelado no compila. Y `NEXT_PUBLIC_API_URL` se **hornea en build**: cambiarlo obliga a reconstruir la imagen, no basta editar el env de Cloud Run. |
| **Horas 4-5 — integración** | La sorpresa: `flags-provider.tsx:29` (`IS_DESIGN_DEMO = API_URL === ""`). Poner la URL relativa mal manda la demo a **modo mock** y la escuela ve datos falsos con cara de reales. Es el fallo más caro del plan y es silencioso. Segunda sorpresa: **el orden importa** — `pnpm demo:verify` **vacía la base** (`.claude/docs/DEMO.md`); correrlo después de sembrar borra la demo. |
| **Hora 6+ — pulido** | Se va a desear haber tenido: un comando para apagar todo, la semilla reproducible, y saber **cuánto tarda de verdad el primer request** (medirlo, no estimarlo). |

---

## 0F. Confirmación de modo

**SELECTIVE EXPANSION** con **ENFOQUE B**, más E2/E3/E4/E5/E6 aceptadas y E1 diferida.

---

## Sección 1 — Arquitectura

### Grafo de dependencias, antes y después

```
ANTES (solo local)                        DESPUÉS (GCP dev)
──────────────────                        ─────────────────
navegador                                 navegador
   │ localhost:3002                          │ https://web-….run.app
   ▼                                         ▼
 web (pnpm dev)                          ┌────────────────┐
   │ localhost:8080  (mismo sitio        │ web (Cloud Run)│ ← COMPONENTE NUEVO
   ▼                  por ser localhost) │  Route Handler │
 gateway                                 │  /api/[...] ───┼──┐
   │                                     └────────────────┘  │ HTTPS, público
   ▼                                                          ▼
 7 servicios ── postgres (compose)        ┌──────────────────────┐
                                          │ gateway (Cloud Run)  │
                                          └──────────┬───────────┘
                                                     │ OIDC (runtime SA)
                          ┌──────────┬───────┬───────┼────────┬──────────┬────────┐
                          ▼          ▼       ▼       ▼        ▼          ▼        ▼
                        iam      students  config  grades  finance  attendance  comms
                          └──────────┴───────┴───────┴────────┴──────────┴────────┘
                                              │ socket /cloudsql
                                              ▼
                                       Cloud SQL (garcia-lorca-dev)
                                              ▲
                                     Pub/Sub push (OIDC) ──┘
                                     payment · student · attendance events
```

**Acoplamiento nuevo:** exactamente uno, `web → gateway`. Justificado: es la única forma de
que las cookies `SameSite=Strict` vivan en un solo origen sin comprar un dominio ni un
balanceador. No introduce ciclos: el grafo de Terraform sigue siendo un DAG
(`config` y `iam` son hojas; `web` es la nueva raíz).

### Hallazgo 1.1 — 🔴 CRÍTICO: el proxy no puede ser un `rewrite`

Un `rewrites()` de Next.js se resuelve en **build** y se congela en `routes-manifest.json`.
La URL del gateway solo existe **después** del `terraform apply`. Eso crea un huevo-y-gallina:
no se puede construir la imagen del web hasta que exista el gateway, pero Terraform quiere
la imagen para crear el servicio.

**Decisión (P5 explícito sobre listo):** un **Route Handler catch-all**
`apps/web/app/api/[...path]/route.ts` que lee `process.env.GATEWAY_URL` **en cada request**.
Elimina el orden de despliegue por completo: cambiar el gateway es
`gcloud run services update web --set-env-vars GATEWAY_URL=…`, no reconstruir la imagen.
~40 líneas. Rechazadas: el apply en dos fases (frágil, `-target` es una trampa) y confiar en
que Cloud Run dé una URL predecible (**Heraldo demuestra que no**:
`heraldo-api-ugvhpcz6ja-uc.a.run.app` trae hash aleatorio).

### Hallazgo 1.2 — 🔴 CRÍTICO: el proxy pierde una de las dos cookies

El gateway manda **dos** cabeceras `Set-Cookie` (`auth.go:88-105`): `gl_access` en `Path=/`
y `gl_refresh` en `Path=/api/v1/auth`. Un proxy escrito de la manera obvia
(`res.headers.get("set-cookie")`) **une las dos en un solo string** y el navegador guarda una
sola cookie malformada. Resultado: el login parece funcionar y la sesión **muere a los ~15
minutos** sin poder renovarse. Falla silenciosa, tardía, y clavada justo a la mitad de una
demostración.

**Decisión (P1 completitud):** el handler usa `res.headers.getSetCookie()` (array) y reemite
cada cookie por separado. **Test obligatorio primero**: una respuesta upstream con dos
`Set-Cookie` debe salir con dos.

### Las cuatro rutas del flujo de datos nuevo (navegador → web → gateway)

```
  FELIZ:   browser → /api/v1/auth/login → Route Handler → gateway → iam
           ← 200 + 2 Set-Cookie ← reemitidas por separado ← cookies en el origen del web ✅

  NULA:    NEXT_PUBLIC_API_URL sin definir en el build
           → AppShell.tsx:38 `?? "http://localhost:8080"`
           → el navegador del director marca a SU propio localhost → connection refused
           → 🔴 APP MUERTA, sin un solo log del lado servidor

  VACÍA:   NEXT_PUBLIC_API_URL = "" (que es lo que QUEREMOS para el mismo origen)
           → flags-provider.tsx:29 `IS_DESIGN_DEMO = true`
           → nunca llama a /features, usa DEMO_FLAGS
           → 🔴 LA ESCUELA VE DATOS FALSOS creyendo que son suyos

  ERROR:   GATEWAY_URL apunta a un gateway frío o caído
           → fetch del Route Handler falla
           → hoy: 500 sin cuerpo. La UI muestra pantalla en blanco.
           → 🟡 GAP: debe devolver 502 con JSON y que la UI lo distinga de "no autenticado"
```

**Las rutas NULA y VACÍA son las dos peores del plan porque ninguna hace ruido.** Violan la
Directiva 1 (cero fallas silenciosas). Ambas se arreglan con el mismo cambio: dejar de
usar el string vacío como interruptor de modo.

### Hallazgo 1.3 — 🔴 CRÍTICO: `IS_DESIGN_DEMO` sobrecarga el string vacío

`flags-provider.tsx:27-29` decide "¿hay gateway?" preguntando `API_URL === ""`. Pero `""` es
exactamente el valor que necesitamos para hacer llamadas relativas al mismo origen
(`auth/api.ts:28` hace `baseURL.replace(/\/+$/,"") + path`, así que `"" + "/api/v1/me"` da
justo la URL relativa correcta). Un mismo valor significa dos cosas opuestas.

**Decisión (P5 explícito):** un flag propio, `NEXT_PUBLIC_DESIGN_DEMO === "true"`.
`NEXT_PUBLIC_API_URL=""` pasa a significar solo "mismo origen". Cambio de 2 líneas.
**Test primero:** con `API_URL=""` y sin el flag de demo, el provider DEBE llamar a
`/features`.

### Máquina de estados: ciclo de vida de una instancia Cloud Run (min=0)

```
        ┌──────────────────────────────────────────────────┐
        │                                                  │
        ▼                                                  │
   ┌─────────┐  request  ┌──────────┐  listo  ┌─────────┐  │ ~15 min
   │  CERO   │ ────────▶ │ ARRANQUE │ ──────▶ │ SIRVE   │  │ sin tráfico
   │ ($0.00) │           │ 2-5 s    │         │         │ ─┘
   └─────────┘           └──────────┘         └─────────┘
        ▲                      │                    │
        │                      │ la imagen          │ 5xx
        │                      │ no arranca         ▼
        │                      ▼               ┌─────────┐
        └───────────────  ┌──────────┐         │ Cloud   │
                          │  FALLA   │         │ Run     │
                          │ 503 al   │         │ reinicia│
                          │ usuario  │         └─────────┘
                          └──────────┘

  Transición imposible: CERO ──▶ SIRVE directo. Cloud Run ENCOLA el request
  durante ARRANQUE (no lo pierde). Por eso Pub/Sub push funciona con min=0:
  el POST despierta el servicio y el ack_deadline de 30 s cubre los 2-5 s de arranque.
```

### Escalado, puntos únicos de falla, seguridad

- **Qué truena primero a 10x:** nada de cómputo. `gateway_max_instances = 1` en dev es un
  techo duro: al pasar de ~200 requests concurrentes (`concurrency = 200`) los siguientes se
  encolan. Para una demo de una escuela es holgadísimo. A 100x, el techo es
  `db-f1-micro` (~25 conexiones), no Cloud Run.
- **SPOF:** Cloud SQL (una zona). Aceptado para dev; prod usa REGIONAL. Segundo SPOF nuevo:
  **el servicio `web`** — si no arranca, no hay demo aunque los 8 backends estén sanos.
- **Superficie de ataque nueva:** exactamente un endpoint público nuevo, el catch-all
  `/api/[...path]` del web. **Es un proxy: hay que acotarlo a la lista de rutas del gateway,
  no reenviar cualquier cosa.** Ver Sección 3.

### Postura de rollback

| Qué falló | Cómo se revierte | Cuánto tarda |
|---|---|---|
| Una imagen mala | `gcloud run services update-traffic <svc> --to-revisions=<previa>=100` | ~30 s |
| Terraform mal aplicado | `terraform apply` con el tfvars anterior | ~5 min |
| Migración mala | ❌ **No hay rollback.** Ver Hallazgo 9.1 | — |
| El gasto se dispara | `make demo-down` (E2) | ~1 min |

**Belleza / potencial de plataforma:** el Route Handler proxy no es solo un parche de cookies
— es **la frontera de origen único** de la aplicación. Cuando llegue el dominio propio de la
escuela, o un segundo cliente web, la frontera ya existe y solo cambia `GATEWAY_URL`.

---

## Sección 2 — Mapa de errores y rescates

### Registro de errores y rescates

| Ruta de código | Qué puede salir mal | Clase de error |
|---|---|---|
| `app/api/[...path]/route.ts` (nuevo) | `GATEWAY_URL` sin definir | `TypeError: Invalid URL` |
| | gateway frío, tarda >10 s | `fetch` timeout / `AbortError` |
| | gateway devuelve 5xx | (no lanza; `res.ok === false`) |
| | ruta fuera de la lista permitida | — (debe rechazarse) |
| | dos `Set-Cookie` unidas | (silencioso, ver 1.2) |
| `fetchFeatures` (`features.ts:14`) | gateway inalcanzable | `TypeError: Failed to fetch` |
| | HTTP no-2xx | `Error("features: HTTP nnn")` |
| `login` (`auth/api.ts:32`) | HTTP no-2xx | `Error("login: HTTP nnn")` |
| `me` (`auth/api.ts:51`) | 401 sin sesión | `Error("me: HTTP 401")` |
| `scripts/migrate-cloudsql.sh` (nuevo) | proxy de Cloud SQL no levanta | exit ≠ 0 |
| | un `.sql` falla a mitad | `psql` `ON_ERROR_STOP=1` |
| Arranque de cualquier servicio Go | `DB_PASSWORD` no está en Secret Manager | pánico al boot |
| Push de Pub/Sub | el consumidor no arranca | reintento → DLQ tras 5 |

| Clase de error | ¿Rescatado? | Acción de rescate | Qué ve el usuario |
|---|---|---|---|
| `Invalid URL` (GATEWAY_URL vacío) | **N ← GAP** | — | 500 en blanco |
| `AbortError` (gateway frío) | **N ← GAP** | — | Pantalla en blanco durante 2-5 s |
| gateway 5xx | **N ← GAP** | — | Se propaga el body crudo |
| ruta no permitida | **N ← GAP** | — | Se reenvía al gateway |
| `Set-Cookie` unidas | **N ← GAP CRÍTICO** | — | **Nada. La sesión muere a los 15 min.** |
| `features: HTTP nnn` | **Y** | `.catch(() => {})`, conserva DEMO_FLAGS | Módulos apagados (default-deny) |
| `login: HTTP nnn` | **Y** | `AppShell.tsx` muestra el error | Mensaje de error en el login |
| `me: HTTP 401` | **Y** | intenta `refresh`, luego pantalla de login | Pantalla de login |
| migración fallida | **Y** (`ON_ERROR_STOP=1`) | corta al primer error | Salida en la terminal |
| `DB_PASSWORD` ausente | **Y** (falla al arrancar) | Cloud Run no promueve la revisión | 503 |
| consumidor caído | **Y** | backoff → DLQ tras 5 intentos | La notificación no llega |

**5 GAPs, todos en el Route Handler nuevo — el único código nuevo del plan.** Es coherente:
lo ya construido tiene su manejo de errores; lo que falta es lo que aún no se escribe.

### Decisiones automáticas (P1 completitud — se cierran los 5 GAPs)

| Gap | Rescate decidido | Qué verá el usuario |
|---|---|---|
| `GATEWAY_URL` vacío | Validar al arrancar el módulo y **fallar el boot** con mensaje claro | La revisión nunca se promueve (mejor que una demo rota) |
| gateway frío | `AbortSignal.timeout(30_000)` + reintento 1 vez | La pantalla de "despertando" de E4 |
| gateway 5xx | Reenviar el status, envolver en `{"error":"gateway_unavailable"}` | Mensaje distinguible de "no autenticado" |
| ruta no permitida | Prefijo permitido `/api/v1/` únicamente; el resto → 404 | 404 |
| `Set-Cookie` unidas | `getSetCookie()` + reemitir una por una, **con test primero** | Sesión que sí dura |

Nada de `catch` genérico. Cada error tiene nombre, rescate y consecuencia visible.

---

## Sección 3 — Seguridad y modelo de amenazas

| # | Amenaza | Probab. | Impacto | ¿El plan la mitiga? |
|---|---|---|---|---|
| S1 | **El Route Handler es un proxy abierto**: sin restricción reenvía cualquier ruta al gateway, y la petición sale **desde dentro de la red de Cloud Run** | Media | **Alta** | ❌ → mitigado por decisión: solo prefijo `/api/v1/`, y `GATEWAY_URL` fijo por env (nunca tomado del request) |
| S2 | Cloud SQL con IP pública (fix de B1) expuesta a internet | Baja | Alta | ✅ `authorized_networks` vacío + `ssl_mode` obligatorio; solo entra IAM vía proxy |
| S3 | `OWNER_TEMP_PASSWORD` filtrado en el historial de shell o en tfvars | Media | **Alta** | ✅ `TF_VAR_owner_temp_password` en la invocación, `seed_owner_secret`; rotación forzada al primer login (`mustChangePassword`) |
| S4 | Bucket de estado de Terraform con secretos legible | Baja | Alta | ⚠️ **GAP**: el plan no exige acceso uniforme + versioning + sin acceso público. Añadir al runbook. |
| S5 | El proxy reenvía la cabecera `Host` del cliente y el gateway confía en ella | Baja | Media | ⚠️ **GAP**: no reenviar `host`; dejar que `fetch` ponga el del destino |
| S6 | Falsificación de push de Pub/Sub | Baja | Alta | ✅ Ya cerrado: OIDC + el handler exige `email == PUBSUB_PUSH_SA_EMAIL` |
| S7 | Escalada de privilegios entre servicios | Baja | Alta | ✅ `run.invoker` por servicio, no a nivel proyecto; SA de push separada del SA de runtime |
| S8 | Datos reales de menores en una demo pública | **Media** | **Alta** | ❌ **GAP NO CUBIERTO** — ver abajo |
| S9 | La URL de la demo se indexa en buscadores | Media | Media | ❌ **GAP**: sin `robots.txt` / `X-Robots-Tag` |
| S10 | Fuerza bruta al login desde internet | Media | Media | ⚠️ Hay rate limiting **in-process**; con `max_instances=1` es correcto, pero el proxy hace que **todo el tráfico llegue con la IP del servicio web**, no la del cliente → el limitador pierde su discriminante |

### Hallazgo 3.1 — 🔴 S8: la demo va a tener nombres de niños reales

`pnpm demo:ready` siembra 400 alumnos y 20 docentes. Si esos nombres son reales, quedan en
una base en la nube, en una URL pública, en la cuenta personal del dueño, sin contrato de
tratamiento de datos con la escuela. Eso es un problema legal antes que técnico.

**Decisión (P1 + seguridad no es opcional):** la semilla de demo usa **nombres sintéticos
verosímiles**, nunca el padrón real. Verificar qué genera hoy `scripts/` de demo. Si genera
nombres reales, es bloqueante. Se documenta explícitamente en el runbook.

### Hallazgo 3.2 — 🟡 S10: el proxy ciega al limitador de intentos de login

Con el Route Handler en medio, el gateway ve todos los logins llegando del mismo origen. Si
el limitador discrimina por IP, deja de servir; si discrimina por email, sigue bien.
**Decisión (P3 pragmático):** verificar cuál usa `services/gateway` durante la
implementación. Si es por IP, reenviar `X-Forwarded-For` desde el Route Handler. Una línea.

**Validación de entradas:** el plan no agrega ningún campo de usuario nuevo. El único input
nuevo es el **path** del proxy, y se acota por allowlist de prefijo (S1). Sin inyección SQL
nueva (no hay consultas nuevas). Sin dependencias npm/Go nuevas: el Route Handler usa `fetch`
nativo. **Riesgo de dependencias: cero.**

**Bitácora de auditoría:** el proxy debe registrar método, path, status y duración por
request — hoy no existe nada. Ver Sección 8.

---

## Sección 4 — Flujo de datos y casos borde de interacción

```
  ENTRADA ────▶ VALIDACIÓN ────▶ REENVÍO ────▶ RESPUESTA ────▶ COOKIES
     │              │                │              │              │
     ▼              ▼                ▼              ▼              ▼
  [¿path?]     [¿prefijo         [¿GATEWAY_URL   [¿5xx?]      [¿2 Set-Cookie?]
  [¿body?]      /api/v1/?]        vacío?]        [¿body       [¿Path correcto?]
  [¿método?]   [¿método            [¿timeout?]    vacío?]      [¿Secure?]
                permitido?]        [¿DNS?]       [¿stream?]
```

| Interacción | Caso borde | ¿Cubierto? | Cómo |
|---|---|---|---|
| Primer login del día | Web y gateway **ambos fríos**: 2 arranques encadenados, 4-8 s | ❌ → **E4** | Estado "despertando…" tras 800 ms |
| Login | Doble clic en Entrar | ⚠️ | Deshabilitar el botón mientras la petición vuela (verificar `AppShell.tsx`) |
| Login | Correcto pero la sesión muere a los 15 min | ❌ → **1.2** | `getSetCookie()` |
| Restaurar sesión al abrir | `me()` 401 → `refresh()` → `me()` | ✅ | `AppShell.tsx:122-134` |
| Navegar a un módulo | El módulo está OFF en `ENABLED_MODULES` | ✅ | `ModulePlaceholder` amable |
| Ir a **Docentes** | `teachers` **no está** en `ENABLED_MODULES` | ❌ → **B6** | Módulo ausente del menú |
| Listado de alumnos | 400 resultados | ✅ | Lectura acotada (tope 500) |
| Listado de alumnos | Cero resultados (base sin sembrar) | ❌ → **E3** | Sembrar siempre al desplegar |
| Cualquier pantalla | El gateway se durmió mientras el director leía | ❌ → **E4** | Reintento + aviso |
| Corrida de facturación | Se dispara dos veces | ✅ | Idempotente por `external_ref` |
| Push de Pub/Sub | Llega mientras `comms` arranca | ✅ | Encolado + ack 30 s |
| Migraciones | Se corren dos veces | ✅ | Ledger `schema_migrations` |
| Migraciones | `demo:verify` **después** de sembrar | ❌ → **GAP** | **Vacía la base.** El script debe negarse a correr contra Cloud SQL |

### Hallazgo 4.1 — 🔴 `demo:verify` puede vaciar la base de la demo

`.claude/docs/DEMO.md` advierte que **`demo:verify` vacía la base**. Ese comando hoy solo
conoce el Postgres local. Cuando exista una demo en la nube, un `pnpm demo:verify` distraído
con el env apuntando a Cloud SQL **borra la escuela** — probablemente la noche anterior a la
junta. Que la advertencia viva en un `.md` no es una salvaguarda.

**Decisión (P1 completitud):** el script se niega a correr si el host de la base no es
local, salvo que se pase `--i-know-this-wipes-prod`. Es la barandilla más barata del plan.

---

## Sección 5 — Calidad de código

- **DRY:** `const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080"` está
  repetido **al menos 3 veces** (`AppShell.tsx:38`, `_screens/Dashboard.tsx:12`,
  `_screens/attendance/usePassList.ts:17`) y una cuarta con default distinto
  (`flags-provider.tsx:27` usa `?? ""`). **Cuatro copias, dos defaults distintos.** Justo el
  tipo de duplicación que produce el bug 1.3.
  **Decisión (P4 DRY):** un solo `apps/web/app/api-url.ts` que exporte el valor resuelto, y
  los cuatro sitios lo importan. Cambio mecánico, hace imposible que los defaults diverjan.
- **Nombres:** `IS_DESIGN_DEMO` nombra el *qué* pero se deriva del *cómo* (string vacío).
  Renombrar la fuente, no la constante (ver 1.3).
- **Sobre-ingeniería:** ninguna. El Route Handler es la pieza mínima; la alternativa
  (balanceador + dominio) es más infraestructura por el mismo resultado.
- **Sub-ingeniería:** el Route Handler tal como se describiría "obvio" es frágil en cookies,
  timeouts y allowlist. Los tres se cierran arriba.
- **Complejidad ciclomática:** el handler ramifica ~4 veces (método, allowlist, timeout,
  set-cookie). Bajo el umbral de 5. Sin refactor.
- **Ajuste al patrón existente:** el módulo `web` en Terraform copia el bloque `students`
  cambiando el nombre y quitando Cloud SQL. Exactamente lo que el comentario de
  `services.tf:12-14` dice que hay que hacer.

---

## Sección 6 — Pruebas

```
  FLUJOS DE UX NUEVOS
    - Primera visita del día con web y gateway fríos (E4)
    - Login a través del proxy de mismo origen

  FLUJOS DE DATOS NUEVOS
    - navegador → Route Handler → gateway → servicio (ida)
    - gateway → Route Handler → navegador, con DOS Set-Cookie (vuelta)

  RUTAS DE CÓDIGO NUEVAS
    - route.ts: allowlist de prefijo / método / timeout / reemisión de cookies
    - api-url.ts: resolución única de la URL base
    - flags-provider: IS_DESIGN_DEMO desde flag propio, no desde string vacío

  TRABAJO ASÍNCRONO NUEVO
    - ninguno (Pub/Sub ya existía y ya está probado)

  INTEGRACIONES NUEVAS
    - web → gateway por HTTPS
    - laptop → Cloud SQL por Auth Proxy (migraciones)

  RUTAS DE ERROR NUEVAS
    - los 5 GAPs de la Sección 2
```

| Qué | Tipo | ¿Existe? | Feliz | Falla | Borde |
|---|---|---|---|---|---|
| `route.ts` reemite 2 cookies | Unit (Vitest) | ❌ **escribir primero** | upstream 2 `Set-Cookie` → salida 2 | upstream 0 cookies → salida 0 | cookie con `;` y `=` en el valor |
| `route.ts` allowlist | Unit | ❌ escribir primero | `/api/v1/me` → 200 | `/admin` → 404 | `/api/v1/../../x` (traversal) |
| `route.ts` timeout | Unit | ❌ escribir primero | responde <30 s | upstream cuelga → 504 | reintento exactamente 1 vez |
| `api-url.ts` | Unit | ❌ escribir primero | `""` → relativo | sin definir → **falla ruidosa**, no localhost | `"https://x/"` → sin barra doble |
| `flags-provider` con `API_URL=""` | Unit | ⚠️ existe, **hay que invertirlo** | llama a `/features` | gateway caído → default-deny | flag de demo puesto → no llama |
| Login E2E contra la demo | E2E (Playwright) | ❌ escribir | login → dashboard | credencial mala → error | recarga tras 20 min → sigue con sesión |
| Docentes visible | E2E | ❌ escribir | menú muestra Docentes | — | — |

**La prueba que da confianza para desplegar un viernes a las 2 am:** un E2E que abre la
demo **fría** (sin tráfico previo), hace login, espera 20 minutos, recarga, y sigue
autenticado. Cubre a la vez el arranque en frío, el proxy, y el bug de las dos cookies —
los tres riesgos nuevos del plan, en un solo test.

**Lo que escribiría un QA hostil:** mandar `POST /api/v1/../../../secret` por el proxy y ver
si sale de la allowlist.

**Prueba de caos:** matar el gateway a media sesión y confirmar que la UI dice "servicio no
disponible" y no una pantalla en blanco.

**Pirámide:** 4-5 unitarias, 0 de integración nuevas, 2 E2E. Correcta (base ancha).
**Riesgo de flakiness:** el E2E de arranque en frío **depende del tiempo por diseño**. Se
acota con un timeout generoso y se marca como no-paralelizable; no se le pone un umbral de
latencia estricto que lo vuelva intermitente.

---

## Sección 7 — Rendimiento

- **N+1 / índices:** el plan **no agrega ni una consulta**. No aplica. Sin cambios de esquema.
- **La ruta lenta nueva, y es la que ve el cliente:**

```
  Primera visita del día (todo en cero):
    DNS + TLS                     ~200 ms
    arranque del web (standalone) ~1.5-2.5 s
    arranque del gateway          ~1-2 s      ← encadenado, no en paralelo
    arranque de iam (login)       ~1-2 s      ← encadenado
    consulta a Cloud SQL          ~50 ms
                                  ─────────────
    p99 del primer login          ~4-7 s
    Visitas siguientes (calientes) ~150-300 ms
```

  **Este es el precio exacto de los $0 en reposo, y es un precio de venta, no técnico.**
- **Presión sobre el pool de conexiones:** 7 servicios × su pool contra `db-f1-micro`
  (~25 conexiones). Con `max_instances=3` por servicio el techo teórico supera el límite.
  🟡 **GAP:** hay que fijar un pool chico y explícito por servicio (p. ej. `MaxOpenConns=2`).
  Con min=0 casi nunca hay 7 servicios despiertos a la vez, pero "casi nunca" no es una
  garantía y el modo de falla (`too many connections`) es feo y aleatorio.
- **Caché:** el Route Handler **no debe cachear nada** — es tráfico autenticado.
  Poner `export const dynamic = "force-dynamic"` explícito. Que Next cachee una respuesta
  de `/api/v1/me` y se la sirva a otro visitante sería una fuga de sesión.
- **Memoria:** el proxy debe hacer *stream* del cuerpo, no bufferearlo. Las respuestas son
  JSON chico, pero bufferear es un hábito que se paga cuando aparezcan PDFs de boletas.

### Hallazgo 7.1 — 🟡 Fijar el pool de conexiones antes de aplicar

**Decisión (P2, dentro del radio de impacto):** `MaxOpenConns` explícito y bajo en los 7
servicios. Barato ahora, doloroso de diagnosticar después.

---

## Sección 8 — Observabilidad

| Necesidad | ¿Existe? | Decisión |
|---|---|---|
| Log estructurado del proxy (método, path, status, ms) | ❌ | **Agregar.** Es el único componente sin dueño en los logs |
| Correlación web ↔ gateway | ❌ | Propagar `X-Request-Id` desde el Route Handler |
| Métrica "¿está viva la demo?" | ❌ | Cloud Monitoring uptime check contra `/` del web |
| Métrica "¿me está costando?" | ⚠️ parcial | El budget avisa a $7.50/$12/$15 (E5). **Es la métrica que le importa al dueño.** |
| Alerta de arranques fallidos | ❌ | Alerta por log: revisión que no promueve |
| Alerta de la DLQ | ❌ | 🟡 **GAP**: existe la DLQ pero nada avisa si le cae algo |
| Tablero día 1 | ❌ | 4 paneles: requests, latencia p95, errores 5xx, **gasto del mes** |
| Runbook | ⚠️ | §E es el interruptor; falta "la demo no responde: qué mirar y en qué orden" |

**Decisión (P1, y es barato):** log estructurado en el proxy + uptime check + alerta de DLQ.
Los tres son configuración, no código.

**Depurabilidad a 3 semanas:** hoy, con un solo log de Cloud Run por servicio y sin id de
correlación, reconstruir "qué pasó el martes a las 10" significa leer 9 flujos a mano. El
`X-Request-Id` propagado es lo que convierte eso en una consulta.

**Uptime check — ojo:** un chequeo cada minuto **despierta el servicio 1440 veces al día** y
mata el scale-to-zero. Configurarlo a **15 minutos**, o mejor, dejarlo apagado en la demo y
depender del budget. 🟡 **Contradicción real entre observabilidad y costo, y el costo gana
aquí.** → uptime check **cada 15 min**, no cada minuto.

---

## Sección 9 — Despliegue y salida a producción

### Secuencia

```
  1. Habilitar APIs ────▶ 2. Bucket de estado ────▶ 3. terraform apply
                                                          │
        ┌─────────────────────────────────────────────────┘
        ▼
  4. Secret: password de la app
        │
        ▼
  5. setup_app_role.sql (como superusuario)  ← NOBYPASSRLS
        │
        ▼
  6. migrations/**  (11 dominios, orden lexicográfico)
        │
        ▼
  7. Semilla del dueño ────▶ 8. Semilla de demo (400 alumnos)
        │
        ▼
  9. Cloud Build ×9 imágenes ────▶ 10. gcloud run deploy
        │
        ▼
 11. Verificación E2E ────▶ 12. Confirmar min=0 en las 9 revisiones
```

**Orden crítico:** las migraciones van **antes** del primer despliegue con tráfico. Pero
Terraform crea los servicios en el paso 3, apuntando a una imagen que quizá no existe todavía.

### Hallazgo 9.1 — 🔴 El primer apply falla porque las imágenes no existen

`image_tag = "latest"` y el Artifact Registry está **vacío** en un proyecto nuevo. Cloud Run
no puede crear una revisión de una imagen inexistente: `terraform apply` falla en los 9
servicios. El runbook actual (§4 Fase B antes de Fase D) tiene el orden invertido.

**Decisión (P1 completitud):** orden correcto para un proyecto virgen:
1. `terraform apply -target=google_artifact_registry_repository.images -target=google_project_service.apis`
2. Cloud Build construye y sube las 9 imágenes con tag `latest`
3. `terraform apply` completo

Es el único uso de `-target` justificado: arrancar el huevo antes que la gallina, una sola vez.

### Hallazgo 9.2 — 🟡 Las migraciones no tienen vuelta atrás

`migrations/**` solo tiene `.sql` de ida; `schema_migrations` registra qué se aplicó pero no
sabe deshacerlo. En una demo el rollback real es **recrear la base y volver a sembrar**, lo
cual es aceptable *si está escrito*. **Decisión (P3 pragmático):** no se construyen
migraciones `down`. Se documenta que el rollback de datos en dev es
`terraform destroy` de la instancia + apply + migrar + sembrar (~15 min), y **eso es
justamente por lo que la demo no debe tener datos que importen.**

### Otros

- **Feature flags:** ya existen y son env. `ENABLED_MODULES` decide qué ve la escuela. **Es
  la palanca de "modo presentación" y hay que usarla a conciencia** — incluyendo `teachers`.
- **Ventana de riesgo (código viejo + nuevo a la vez):** Cloud Run desplaza el tráfico al
  100% a la revisión nueva. Con una sola instancia, la ventana es de segundos. Sin bases de
  datos compartidas entre versiones incompatibles porque no hay migraciones nuevas.
- **Paridad de entornos:** ❌ **no hay staging.** Dev *es* el staging. Aceptado para una demo.
- **Verificación post-deploy (primeros 5 min):** los 12 pasos de arriba, más:
  `gcloud run services list --format="value(name,spec.template.metadata.annotations)"` y
  confirmar `minScale` vacío en las 9.
- **Primera hora:** abrir la demo, hacer login, tocar cada módulo, cerrar. Volver a las 2 h
  y confirmar que todo volvió a cero (`gcloud run revisions list` sin instancias activas).

---

## Sección 10 — Trayectoria a largo plazo

- **Deuda técnica introducida:**
  - *Código:* el Route Handler es un proxy propio. Deuda pequeña y contenida, pero real: si
    mañana hay dominio propio, el proxy sobra y hay que acordarse de quitarlo.
  - *Operativa:* migraciones aplicadas **a mano desde la laptop**. Funciona para dos veces al
    año; no escala a la escuela. Deuda consciente.
  - *Pruebas:* sin staging, la primera vez que este camino se ejerce es contra la demo real.
  - *Documentación:* `COST-AND-DEPLOY.md` ya está desfasado. Se corrige (E6).
- **Dependencia de camino:** baja. Todo lo que se agrega es aditivo. El único punto que
  condiciona el futuro es **D1**: si dev usa Neon y prod usa Cloud SQL, se mantienen dos
  rutas de conexión para siempre.
- **Reversibilidad: 5/5.** `terraform destroy` borra el entorno completo. La única puerta de
  un solo sentido sería sembrar datos reales de alumnos (Hallazgo 3.1) — un dato personal
  publicado no se despublica. **Por eso 3.1 es bloqueante y no una recomendación.**
- **Concentración de conocimiento:** el runbook queda en el repo, no en la cabeza del dueño.
  Un ingeniero nuevo en 12 meses lee `PLAN-deploy-gcp-dev.md` + `COST-AND-DEPLOY.md`
  corregido y puede repetirlo.
- **La pregunta del año:** ¿por qué hay un Route Handler que proxea a un servicio hermano?
  **Hay que escribir el porqué en el archivo**, no en este plan: "las cookies son
  `SameSite=Strict` y `run.app` está en la Public Suffix List". Sin ese comentario, en 12
  meses alguien lo borra por redundante y rompe el login.
- **Qué viene después:** promoción a la escuela (mismo TF + `prod.tfvars`), gateway+iam
  calientes, DB REGIONAL, dominio propio (el proxy se retira), y CI que aplique por rama.
  **La arquitectura soporta esa trayectoria sin reescrituras.**
- **Retrospectiva de los cherry-picks:** E4 (estado de "despertando") resultó **cargante de
  peso**, no cosmético: es la mitigación de la ruta lenta de la Sección 7, que a su vez es
  consecuencia directa de la decisión de costo. Aceptarlo fue correcto. E1 (apagar la DB de
  noche) quedó bien diferido: habría empeorado la misma latencia que E4 intenta tapar.

## Sección 11 — Diseño y UX

**OMITIDA — sin alcance de UI detectado** (0 coincidencias de términos de interfaz en el
plan). El plan es de infraestructura. La única superficie visible que toca es la pantalla de
"despertando" (E4), y su diseño se resuelve con el sistema de diseño existente
(`.claude/docs/design/DESIGN-SYSTEM.md`), no requiere revisión de diseño propia.

---

## Paso 0.5 — Voces duales (CEO)

**CODEX SAYS (CEO — strategy challenge):** `[codex-unavailable: binary not found]`.
Instalar con `npm install -g @openai/codex` para cobertura cross-model.

### CLAUDE SUBAGENT (CEO — independencia estratégica)

Contexto fresco, sin ver esta revisión. **17 hallazgos.** Verifiqué uno por uno los de mayor
riesgo antes de absorberlos; **todos los que verifiqué resultaron ciertos**, incluidos tres
que refutan cosas que yo escribí.

#### 🔴 F3 — CRÍTICO, y es el peor hallazgo de toda la revisión: `IAM_JWT_SECRET`

`services/iam/cmd/main.go:106-116`:

```go
func jwtSecret(log *slog.Logger) []byte {
	if s := os.Getenv("IAM_JWT_SECRET"); s != "" { return []byte(s) }
	b := make([]byte, 32); _, _ = rand.Read(b)
	log.Warn("IAM_JWT_SECRET not set; generated an ephemeral signing key (dev only)")
	return []byte(hex.EncodeToString(b))
}
```

**Verificado:** `grep -rn "IAM_JWT_SECRET" deploy/` → **cero resultados.** No está en
`common_env`, ni en `common_secret_env`, ni en `secrets.tf`.

Con `iam_min_instances = 0`, `iam` se duerme a los ~15 minutos. El siguiente request la
despierta **con una llave de firma nueva**, y todos los tokens emitidos antes dejan de
validar. Traducido a la junta: *el director entra, navega, se queda leyendo una pantalla
cinco minutos, hace clic — y lo saca al login.* Sin error entendible. Y si `max_instances`
levanta dos instancias a la vez, cada una firma con una llave distinta: 401 intermitentes,
imposibles de reproducir.

**Es invisible en local**, porque en `docker-compose` `iam` nunca se reinicia. Por eso nadie
lo vio. Es exactamente la clase de falla que este plan existe para atrapar: **silenciosa,
diferida, y solo aparece bajo scale-to-zero — o sea, solo aparece por culpa de la decisión de
costo.**

**Decisión (P1 completitud, no negociable):** secreto `garcia-lorca-iam-jwt-secret-${env}` en
`secrets.tf`, inyectado por `common_secret_env`, **más** un fallo duro al arrancar en
`jwtSecret()` cuando `APP_ENV != "local"`. El fallback aleatorio nunca debe poder correr en
la nube. **Bloquea el deploy.**

#### 🔴 F5 — CRÍTICO: `uniforms` NO es un stub. Me equivoqué.

Escribí (siguiendo a `COST-AND-DEPLOY.md:52`) que los stubs son
`billing, scholarships, docs, uniforms`. **Falso, y verificable en tres lugares:**

- `services/uniforms/`: **35 archivos Go**, `go.mod` propio, `Dockerfile`, `cmd/`, `internal/`
- `migrations/uniforms/`: **4 migraciones**
- `FLAGS.md:31`: *"Backed by a real Clean-Architecture service since M12 (ADR 0007)"* —
  con ADRs 0007/0008/0009/0010
- El `CLAUDE.md` de este repo **lo dice explícitamente**: uniforms nació clasificado como thin
  y **se movió** a Clean Architecture completa
- `apps/web/app/_design/data.ts:64`: el mosaico **"Uniformes — Inventario y mostrador"** ya
  está en el menú

Caí en el antipatrón que yo mismo señalé en el chequeo retrospectivo: **creerle a un documento
de costos desactualizado en vez de mirar el árbol.** Es una lección, no solo un bug.

Costo de negocio: inventario y venta en mostrador es de lo **más tangible** que se le puede
enseñar a una administradora escolar — se ve, se toca, se entiende sin explicación. Y de
pilón, su ausencia **tumba la semilla** (F4) y deja un mosaico muerto en el menú (F16).

**Decisión (P1 + P2):** desplegar `uniforms` como **noveno servicio backend**. Ya tiene
Dockerfile. Faltan ~30 líneas: bloque `module "uniforms"`, paso de imagen en `cloudbuild.yaml`,
`UNIFORMS_BASE_URL` en `gateway_env`, y `uniforms` en `ENABLED_MODULES`.
**Los stubs reales son tres: `billing`, `scholarships`, `docs`.**

#### 🔴 F4 — CRÍTICO: la Fase C paso 8 es imposible tal como la escribí

Escribí *"sembrar con `pnpm demo:ready` apuntado a Cloud SQL"*. **Se rompe por tres lados
independientes**, los tres verificados:

1. `package.json:17` — `demo:ready` = `demo:verify && demo:seed && demo:smoke`, y
   `demo:verify` corre la suite Playwright desde cero, que **vacía la base**. Mi propio
   `CLAUDE.md` advierte esto y aun así lo escribí mal. (Confirma el Hallazgo 4.1 desde otro
   ángulo, y lo empeora: no es un riesgo futuro, **está en el runbook.**)
2. `scripts/reset-demo-db.sh:41` — llama `docker exec -i "$CONTAINER" psql`
   incondicionalmente. **No puede apuntar a Cloud SQL de ninguna manera.** El comentario de
   cabecera que dice que funciona "anywhere else" es mentira; hay que borrarlo.
3. `scripts/seed-demo.mjs:434-469` — la semilla llama `/api/v1/uniforms/…`. Sin uniforms
   desplegado, 404 → la semilla revienta → **escuela a medio sembrar.**

**Decisión (P1):** `scripts/seed-cloud.mjs`, **solo siembra**, contra el proxy de Cloud SQL, y
se **ensaya contra la base real 48 h antes de la junta**. Corregir el comentario falso.

#### 🟡 F6 — `module_tuition` no existe. Me equivoqué otra vez.

Mi B6 decía agregar `teachers,tuition`. **`FLAGS.md:24` dice lo contrario, con todas sus
letras:** *"M15 (colegiaturas) rides on THIS flag, not one of its own — the plan reserved
`module_tuition`, and building it showed the split had no reader."* Y `grep 'Protect("'` en el
gateway confirma que no hay guard de `tuition`. Colegiaturas ya funciona bajo `module_finance`.

**Correcto:** `ENABLED_MODULES = "iam,gateway,students,config,grades,finance,attendance,comms,teachers,uniforms"`

#### 🟡 F8 — Dos variables que el gateway lee y Terraform no le da

Verificado: `services/gateway/cmd/main.go:71` lee `UNIFORMS_BASE_URL`; `:118` lee `WEB_ORIGIN`
(la allowlist de CORS). Ninguna está en `local.gateway_env`. Con el proxy de mismo origen CORS
es irrelevante — pero **la app móvil sí pega directo al gateway**, y sin `WEB_ORIGIN` su
login falla con un síntoma idéntico a B3. **Decisión:** agregar ambas.

#### 🔴 F9 — Nada acota el pool de conexiones. Confirma y agrava el Hallazgo 7.1.

`grep -rn "SetMaxOpenConns\|SetMaxIdleConns" services pkg` → **cero resultados.**
`database/sql` de Go abre conexiones **sin límite** por defecto. 9 servicios × hasta 3
instancias × pool ilimitado, contra las ~25 conexiones de `db-f1-micro`.
`FATAL: remaining connection slots are reserved`, intermitente, primero bajo la semilla
(que corre con 8 en paralelo) y después con el director haciendo clic.
**Decisión (P2, un solo cambio en `pkg/dbdsn`):** `SetMaxOpenConns(3)`, `SetMaxIdleConns(1)`,
`SetConnMaxIdleTime(1m)`.

#### 🔴 F10 — El guardia contra `min_instances` es un ojo humano

Mi Fase B paso 4 decía *"revisar a mano que ningún `min_instances` sea > 0"*.
**Ese es exactamente el control que falló en Heraldo.** Una revisión manual no es un control;
es una intención.

**Decisión (P1 — es la pieza que hace que "esto no puede repetirse" sea verdad y no un deseo):**
un `lifecycle.precondition` en el módulo `cloud_run` que **falle el apply** si
`var.env != "prod"` y `min_instances > 0`, más un gate de `grep` en CI. Gratis, permanente.

#### 🟡 F11 — El presupuesto solo avisa, y avisar ya falló una vez

`budget.tf` no tiene `pubsub_topic`. Un correo a las 3 a.m. no detiene una tormenta de
reintentos de Pub/Sub ni un Cloud Build desbocado. **Al dueño lo quemó una cuenta silenciosa;
las alertas son precisamente el control que no lo salvó.** → decisión de gusto, va a la puerta.

#### 🔴 F14 — El paso de deploy de Cloud Build no tiene permisos

Verificado: en `deploy/terraform/*.tf` solo aparece `cloudbuild.googleapis.com` como API.
**No hay ni un `roles/run.admin` ni un `roles/iam.serviceAccountUser`** para la cuenta de
servicio de Cloud Build. El paso `_DEPLOY=true` (`cloudbuild.yaml`, step `deploy`) va a fallar
con permiso denegado en la primera corrida. Además `timeout: 1800s` para lint + `go test -race`
de todo el workspace + `govulncheck` + `pnpm turbo build` + **9** builds de Docker en una
e2-medium es optimista. **Decisión:** otorgar ambos roles, subir el timeout a `3600s`, y hacer
de `gcloud builds submit` a mano el camino **por defecto** en la primera corrida.

#### 🔴 F15 — El arranque en frío es el riesgo real de la junta, y es barato de matar

Con `min=0`, `cpu_idle=true` y **`startup_cpu_boost = false`**, el primer clic encadena:
web frío → gateway frío → iam frío → primera conexión a Cloud SQL. Y **cada** primer toque de
cada módulo es otro arranque en frío independiente. Realista: 5-15 s el login, 3-8 s por
módulo nuevo. Una directora de colegio privado lee eso como *"no está terminado"*.

**El arreglo cuesta unos seis centavos de dólar:**

```
scripts/demo-warm.sh   # min-instances=1 en web,gateway,iam + un curl a cada módulo
scripts/demo-cold.sh   # min-instances=0 en todo   (correr al terminar la junta)
```

Tres servicios calientes durante una ventana de 3 horas ≈ **$0.06**. Se corre `demo-warm.sh`
15 minutos antes, se pasea por cada módulo una vez, y `demo-cold.sh` al terminar.
**Elimina el riesgo entero sin tocar el costo en reposo.** Y `startup_cpu_boost = true` en
web/gateway/iam: con `min=0` **solo se cobra durante el arranque**, así que en reposo no cuesta
nada y acorta justo el camino que el director sí experimenta.

Esto **supera a mi E4**: E4 tapaba la espera con una pantalla; F15 **la elimina**. Se aceptan
las dos — E4 para el visitante casual que abra la liga un martes cualquiera, F15 para la junta.

#### 🟡 F16 — Mosaicos muertos en el menú

`apps/web/app/_design/data.ts` renderiza **Becas** (56), **Facturación/CFDI** (65) y
**Reportes** (66) con sus flags apagadas → los tres caen en `ModulePlaceholder`. Solo
`documents` está marcado `hidden: true` (58). El director va a hacer clic en al menos uno.
**Decisión (P1, es la cara del producto):** desplegar uniforms (F5) y marcar
`hidden: true` en becas/facturación/reportes para el build de demo.

#### ✅ F17 — El idioma no es riesgo (verificado limpio)

`packages/i18n/src/locales.ts:21`: `DEFAULT_LOCALE = "es"`, mapeado a `es-MX`. Un navegador
nuevo en la escuela abre en español. Sin acción.

#### F2 — La aritmética de Heraldo: el subagente ve una contradicción; en realidad son dos configuraciones distintas

Señala que mis **$6.80/servicio** y los **$95-100** de `COST-AND-DEPLOY.md:20-21` difieren 7x
sin reconciliar. Tiene razón en que **no estaba reconciliado**. Pero no es un error de
aritmética — **son escenarios distintos, y reconciliarlos mejora el argumento**:

| Escenario | `cpu_idle` | Tarifa de CPU | Costo/servicio/mes |
|---|---|---|---|
| **Heraldo hoy** (medido: sin anotación `cpu-throttling` ⇒ throttling ON) | `true` | tarifa *idle* | **~$6.80** |
| **García Lorca con el TF tal como está** (`services.tf:47-54`, `cpu_idle = false`) | `false` | tarifa **activa 24/7** | **~$62** |

**La conclusión que se desprende es más fuerte que la que yo escribí:** si se aplicara el
Terraform **tal como está hoy**, García Lorca costaría **~$124/mes en gateway+iam solos —
más del doble que Heraldo.** El `dev.tfvars` no es una optimización opcional; es lo único que
separa este proyecto de un problema **peor** que el que se quiere evitar.

**Decisión (P1, acepta el fondo del hallazgo):** el reporte de facturación de Heraldo por SKU
**sube a Fase 0 y es bloqueante** — 5 minutos, gratis, y cierra el único supuesto que sigue
abierto. Y `COST-AND-DEPLOY.md` se corrige para que las dos cifras convivan explicadas (E6).

#### F1 — Desafío al vehículo: ¿GCP es siquiera el medio correcto para una demo?

Propone dos caminos nunca analizados: (a) la demo de `docker compose` en la laptop, que **ya
existe y está verificada con Playwright**; (b) una VM `e2-micro` de la capa siempre-gratis
corriendo el mismo `docker-compose.yml`, con URL pública a **$0/mes** — por debajo del piso de
$10 de este plan.

**Es la objeción más fuerte de la revisión y merece ir a la puerta final.** Mi lectura honesta,
para que decidas con los dos lados:

- **La `e2-micro` es más frágil de lo que suena.** Tiene **1 GB de RAM**. Nueve servicios Go
  (~400 MB) + Postgres (~200 MB) + Next.js (~150 MB) ≈ 750 MB antes de atender un solo
  request. Va a estar al borde del OOM. Y desde 2024 GCP **cobra la IP externa IPv4** (~$3.60/mes),
  así que el "$0" tampoco es exacto.
- **Tira el camino a producción.** La escuela (D4) va a querer una nube de verdad. Ese trabajo
  se haría dos veces.
- **Pero la demo en laptop sí es una jugada legítima — como red, no como reemplazo.** Para la
  junta en sí, tiene cero modos de falla. Nada impide tenerla lista *y* desplegar en GCP.

---

## TABLA DE CONSENSO — VOCES DUALES (CEO)

```
═══════════════════════════════════════════════════════════════════════
  Dimensión                              Claude   Codex   Consenso
  ─────────────────────────────────────  ───────  ──────  ────────────
  1. ¿Premisas válidas?                  NO       N/A     ⚠️ 1 voz: 3 premisas
                                                           mías refutadas (F4/F5/F6)
  2. ¿Es el problema correcto?           PARCIAL  N/A     ⚠️ 1 voz: vehículo
                                                           cuestionado (F1) → puerta
  3. ¿Alcance bien calibrado?            NO       N/A     ⚠️ 1 voz: faltan 6 tareas
                                                           (F3/F5/F9/F10/F14/F15)
  4. ¿Alternativas exploradas?           NO       N/A     ⚠️ 1 voz: e2-micro y
                                                           laptop no analizadas
  5. ¿Riesgos de negocio cubiertos?      NO       N/A     ⚠️ 1 voz: arranque en frío
                                                           y mosaicos muertos (F15/F16)
  6. ¿Trayectoria a 6 meses sana?        PARCIAL  N/A     ⚠️ 1 voz: sin ruta de datos
                                                           demo→prod (F12)
═══════════════════════════════════════════════════════════════════════
CONFIRMADO = ambas coinciden. Codex ausente ⇒ N/A, nunca CONFIRMADO.
Hallazgo crítico de una sola voz = se marca igual. **F3 y F5 son de ese tipo,
y los dos resultaron ciertos al verificarlos contra el árbol.**
Modo: [subagent-only] — cobertura cross-model NO obtenida en esta corrida.
```

**Lectura honesta de esta tabla:** con una sola voz externa no hay consenso que reportar, y
el valor del ejercicio se sostiene en que **verifiqué cada hallazgo contra el código** en vez
de creerle. Los seis renglones en ⚠️ significan que la voz externa encontró algo real en las
seis dimensiones — no que haya acuerdo entre modelos.
