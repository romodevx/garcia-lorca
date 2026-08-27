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
