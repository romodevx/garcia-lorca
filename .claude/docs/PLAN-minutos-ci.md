<!-- /autoplan restore point: "/Users/andresromo/.gstack/projects/romodevx-garcia-lorca/feature-optimizacion-minutos-ci-autoplan-restore-20260918-150710.md" -->
## Implementation plan
# PLAN — Bajar el consumo de minutos de GitHub Actions

Estado: **APROBADO** — /autoplan completo (CEO + DX + Eng), 2026-09-18
Premisa bloqueante RESUELTA: el dueño confirma que el límite de gasto de Actions
está deliberadamente en $0. Quedarse sin minutos es quedarse sin CI y sin deploy,
así que el plan completo procede; no se compran minutos.
Rama: `feature/optimizacion-minutos-ci`
Fecha: 2026-09-18

## Problema

GitHub reporta **2,700 min usados de 3,000 incluidos (90%)** en el ciclo actual.

**Cuánto dinero es, dicho explícitamente:** el excedente en repo privado cuesta
**$0.008 USD/min**. Hoy el excedente es cero. Aun duplicando el consumo a 5,500
min/ciclo, la factura sería **~$20 USD/mes (~370 MXN)**. Así que este plan NO se
justifica por dinero.

Se justifica por una sola premisa, que hay que confirmar antes de empezar:
**el límite de gasto de Actions está en $0**, igual que el corte duro de
facturación que el dueño eligió para GCP. Con el límite en $0, agotar la cuota
apaga CI y apaga el deploy — con demos **diarias** para el colegio y entrega el
30 nov 2026, eso es quedarse sin poder desplegar a media semana. Con un límite
distinto de $0, el problema real no es la cuota sino el tiempo de pared, y
siete de las diez tareas de aquí sobran.

> **Bloqueante — cómo se resuelve, no solo dónde mirar:**
> ```sh
> gh auth refresh -h github.com -s user          # el endpoint pide este scope
> gh api /users/romodevx/settings/billing/actions
> ```
> O en la UI: Settings → Billing → Spending limits → Actions.
> **Regla de bifurcación:** si el límite es $0 → Etapa 1 completa. Si NO es $0 →
> solo T1, T2 y T7 (reparación de defectos, no cuestan cobertura); el resto no
> se justifica a ~$20 USD/mes.

## Medición (no estimación)

Medido con la API de Actions, 94 runs entre 2026-08-19 y 2026-09-18, sumando
`ceil(duración_del_job / 60s)` por job — que es exactamente como factura GitHub.
(El endpoint `/timing` devuelve `total_ms: 0` en este repo y no sirve.)

**Total medido: 2,739 min** — coincide con los 2,700 de la factura (1.4% de error).

| Bucket | Minutos | % | Runs |
| --- | ---: | ---: | ---: |
| `Deploy dev` → job `build images and roll out` | 789 | 29% | 35 |
| `CI` → matriz `go` (14 módulos de `go.work`) | 1,619 | 59% | 50 |
| `CI` → job `web` | 150 | 5.5% | 58 |
| `CI` → `proto` + `openapi` + `ci` (3 jobs triviales) | 173 | 6.3% | 58 c/u |

| Evento | Minutos | Runs | Promedio |
| --- | ---: | ---: | ---: |
| `CI` / pull_request | 1,015 | 21 | **48.3 min/run** |
| `CI` / push a `dev` | 935 | 37 | **25.3 min/run** |
| `Deploy dev` / workflow_run | 708 | 25 | 28.3 (58 min los exitosos) |
| `Deploy dev` / dispatch + push | 81 | 11 | — |

**Advertencia sobre la línea base:** estos 2,739 min se midieron en un mes en el
que hubo commits retenidos sin empujar justamente para ahorrar minutos. Comparar
un total contra un mes sin esa restricción mide dos cargas de trabajo distintas.
Por eso el criterio de aceptación es una **tasa**, no un total (ver §Verificación).

## Causas raíz (verificadas en logs, no inferidas)

### C1 — El caché de Go es una lápida de 7.6 KB
`actions/setup-go@v6` cachea `GOCACHE` + `GOMODCACHE` con una llave derivada de
`hashFiles('**/go.sum')`. Las 14 patas de la matriz calculan **la misma llave**.
Las entradas de caché de Actions son inmutables: la primera pata que ganó la
carrera guardó un caché casi vacío y quedó congelado ahí para siempre.

Log del run 35396038508, pata `go (./services/iam)`:

```
Cache hit for: setup-go-Linux-x64-ubuntu24-go-1.25.14-f4c882bf...
Cache Size: ~0 MB (7599 B)
Cache restored successfully
go: downloading cloud.google.com/go/firestore v1.22.0
...
Cache hit occurred on the primary key ..., not saving cache.
```

Reporta "cache hit", restaura 7.6 KB, y acto seguido **descarga todos los
módulos y compila el grafo completo desde cero** — en cada pata, en cada run,
desde que existe el workflow. Como la llave ya existe, nunca vuelve a guardar:
el caché no se puede auto-reparar.

Costo por pata en ese run: `go vet` 109 s + `go test -race` 112 s +
`integration` 43 s + `govulncheck` 27 s = **307 s**, casi todo compilación fría.

### C2 — El mismo código se compila cuatro veces por pata
`go vet ./...` compila el grafo entero (109 s). `go test -race` lo recompila con
instrumentación (112 s). Integración lo compila otra vez con `-tags`.
`govulncheck` carga el grafo una cuarta vez. `go test` **ya corre el
subconjunto de vet de alta confianza**.

**Evidencia de que `go vet` no compra nada:** en 58 runs de CI, `go vet` y
`go test -race` fallaron **cero** veces. Los únicos pasos de Go que se pusieron
en rojo fueron `govulncheck` (9 patas, run 35394352750) y
`go test -tags=integration` (students, runs 35394352750 y 35395204982). Esos dos
se quedan en la ruta del PR: son todo el valor demostrado de la matriz.

### C3 — Se paga un runner de GitHub por mirar un build de Google
El job `build images and roll out` dura **~3,500 s (58 min) en los deploys
exitosos**. Hace `gcloud builds submit`, que **transmite logs de forma síncrona**
mientras Cloud Build construye las diez imágenes en infraestructura de Google.
El runner no compila nada: espera, y factura mientras espera. ~10 deploys
exitosos × 58 min = **~580 min** de cuota gastados en un `tail -f`.

### C4 — Impuesto de redondeo por job
GitHub factura cada job redondeado al minuto completo:

- `proto`: 17 s de trabajo → 1 min × 58 runs = **58 min**
- `openapi (lint)`: 9 s (3 de ellos bajando `@redocly/cli@latest`) → **57 min**
- `ci` (un `if` de cinco líneas) → **58 min**
- matriz `go`: 153 s reales → 3 min facturados en PR; 81 s → 2 min en push.
  Solo el redondeo, sobre 700 jobs de matriz, son **~400 min**.

### C5 — La matriz no mira el diff
Cambiar un archivo en `apps/web` corre las 14 patas de Go. `paths-ignore` solo
excluye markdown, `docs/` y `.claude/`.

### C6 — Los dos pipelines corren la MISMA suite de compuertas
`cloudbuild.yaml` vuelve a correr, sobre el mismo commit que CI acaba de
aprobar: `buf lint`, `go vet` (todos los módulos), `go test -race` (todos los
módulos), `govulncheck`, `web build` y la compuerta de source maps. Eso es la
mayor parte de los 58 min de build. CI es la compuerta; Cloud Build debería
construir imágenes y desplegar, nada más.

## Objetivo

Que un deploy cueste **≤ 2 min facturados** y un run de CI baje por etapas, sin
quitar ninguna compuerta que haya atrapado algo real.

**El piso aritmético manda sobre el deseo.** GitHub cobra cada job redondeado al
minuto, y un run de PR hoy tiene **18 jobs** (14 de matriz + `proto` + `web` +
`contract` + `ci`). Aunque cada uno terminara en 3 segundos, el run cuesta
**18 min**. Por eso:

- **Etapa 1: ≤ 18 min por run de CI** (hoy: 48 en PR, 25 en push).
- **Etapa 2 (con T3, que baja la matriz a 4 shards → 8 jobs): ≤ 8 min por run.**

Pedir ≤8 en la Etapa 1 es pedir algo imposible con 18 jobs; era un criterio que
el plan no podía cumplir ni haciendo todo bien.

## Plan de implementación

Dos etapas. La Etapa 1 lleva ~94% del ahorro con el riesgo bajo; la Etapa 2
lleva ~6% y concentra todo el riesgo, así que va después del 30 nov.

### ETAPA 1 — antes del 30 nov

#### T0 — Línea base medible antes de tocar nada
`scripts/ci-minutes.sh`, en el estilo de `scripts/deploy-preflight.sh`
(`/bin/sh`, solo lectura, helpers `ok/bad/warn`, exit 1): consulta la API de
Actions, suma `ceil(job_duration/60)` y reporta **min por run de CI** y **min
por deploy**, no solo el total. Es la prueba que falla primero: hoy imprime
~48 min/run de PR y ~58 min/deploy.

#### T1 — Reparar el caché de Go (la palanca más grande de CI)
El diseño con `github.run_id` en la llave **no sirve**: medido en esta máquina,
`GOMODCACHE` son **6.0 GB** y `GOCACHE` **1.0 GB**, contra un presupuesto LRU de
**10 GB por repo**. Rotar la llave cada run llenaría y desalojaría el caché en
menos de un día, y los PR restaurarían basura o nada. La lápida se arregla
haciendo la llave **por shard**, no rotándola.

- Paso 1: **medir**. Un run instrumentado que imprima el tamaño real de
  `GOCACHE` y `GOMODCACHE` **por módulo** en Linux. El número decide el resto.
- `cache: false` en `actions/setup-go` (deja de emitir la llave compartida).
- **`GOMODCACHE`**: UNA entrada compartida, llave
  `gomod-${{ hashFiles('**/go.sum') }}`, sin rotación (cambia cuando cambian las
  dependencias, que es lo correcto). La guarda un job dedicado.
- **`GOCACHE`**: una entrada **por módulo de la matriz**, llave
  `gocache-${{ matrix.module }}-${{ hashFiles('**/go.sum') }}` — **sin
  `github.sha`**. Poner el SHA rota la llave en cada push igual que `run_id`:
  la llave primaria nunca acierta, cada push sube 14 entradas nuevas que no
  reemplazan a las viejas, y el LRU desaloja justo la generación recién subida.
  Para invalidar a mano existe una variable `CACHE_EPOCH` que se sube a mano.
  **En la Etapa 1 la unidad es `matrix.module` (14 patas), no "shard":** los
  shards no existen hasta T3, que es Etapa 2. Escribir la llave en términos de
  shards deja la tarea sin poder implementarse.
- **Guardar solo en push a `dev`** (`if: github.event_name == 'push'`). Los PR
  restauran y no escriben: así no se desaloja nada y se cierra de paso el vector
  de envenenamiento de caché desde una rama.
  **Consecuencia que va escrita en el comentario del workflow:** una rama que
  toca `go.sum` compila en frío en cada run de PR hasta que se mergea. Sin ese
  comentario, el siguiente que lo vea concluye "el caché volvió a romperse".
- **Si no cabe en 10 GB, el plan B es de Etapa 1, no T3:** cachear `GOCACHE`
  solo en los módulos caros (iam, students, finance, comms, uniforms, grades,
  attendance) y dejar que los baratos compilen en frío. Bajar el número de
  shards NO es una salida disponible antes del 30 nov.
- **Observabilidad del caché (si no, esto se repite).** La lápida sobrevivió
  meses porque un caché vacío y uno lleno se ven igual en el log. Paso posterior
  al restore que imprima el tamaño restaurado y avise — y falle en push a
  `dev` — si está por debajo de un umbral.
  **La aserción se salta cuando `cache-hit != 'true'`.** En el primer push
  después del cambio todavía no hay caché: sin esa condición el guardia se pone
  rojo por construcción y lo primero que hace cualquiera es borrarlo. Lo que se
  afirma es "un acierto no es una lápida", no "siempre hay caché".
- **El presupuesto de 10 GB no está vacío.** `ci.yml` ya guarda el store de pnpm
  y un caché de turbo con llave `turbo-${{ runner.os }}-${{ github.sha }}` —
  otra entrada nueva por run, compitiendo por la misma cuota y desalojando
  entradas de Go. La llave de turbo se arregla en la misma tarea.
- **Restaurar 6 GB también cuesta minutos**, catorce veces. El run instrumentado
  mide el tiempo de restore, no solo el tamaño: si restaurar cuesta más de lo
  que ahorra en un módulo barato, ese módulo no se cachea.

#### T2 — Dejar de compilar lo mismo cuatro veces
- Borrar el paso `go vet` por módulo (cero hallazgos en 58 runs, 109 s × 14 × 50).
- `go vet` **una vez** sobre todo el workspace, en un job con caché caliente.
  **El comando NO es `go vet ./...`.** No hay `go.mod` en la raíz, solo
  `go.work`, así que desde la raíz falla con
  `pattern ./...: directory prefix . does not contain modules listed in go.work`
  (verificado ejecutándolo). `cloudbuild.yaml` ya documenta exactamente esto en
  el comentario del paso `go-vet`. La forma que sí corre:
  `go vet $(go work edit -json | jq -r '.Use[].DiskPath+"/..."')`.
- `govulncheck`: instalar el binario **una vez** y compartirlo por caché, en vez
  de `go install` en cada pata. El paso se queda en la ruta del PR: es uno de los
  dos únicos que han atrapado algo.

#### T5 — Fundir `openapi (lint)` y `actionlint` dentro del job `proto`
Mismo checkout, mismo runner, cero minutos extra. Pinear `@redocly/cli` a una
versión en vez de `@latest` — mismo error que tumbó el deploy el 2026-09-10 con
`govulncheck@latest`. `actionlint` entra aquí (el job ya tiene Go): la
verificación del plan lo daba por instalado y no lo está.
**`actionlint` se pinea igual**, con su variable `ACTIONLINT_VERSION`. Meterlo
con `@latest` en el mismo párrafo que pinea redocly sería repetir el error que
se está corrigiendo.
**Fundirlo obliga a editar el job `ci`.** `ci.yml:319` declara
`needs: [proto, go, web, contract]` y `ci.yml:325` lee
`${{ needs.contract.result }}`. Si `contract` desaparece sin tocar eso, el
workflow ya no parsea y **todos** los runs fallan, incluido el disparador del
deploy. T5 edita `needs`, la cadena `results`, y revisa el nombre del required
check en la protección de rama.
Nota sobre los 9 rojos de `openapi (lint)`: no son contratos rotos que la
compuerta atrapó, son deriva de `@redocly/cli@latest`. El pineo los elimina; la
compuerta se queda por el canon spec-first, no por ese número.

#### T7 — Enviar el build y esperar el veredicto, no los logs
- `gcloud builds submit --async`, capturar el BUILD_ID, y **sondear el estado
  terminal** con `gcloud builds describe` cada ~30 s. Costo: **~2-3 min
  facturados** en vez de 58.
- **El sondeo lleva fecha límite.** Sin tope, un build colgado factura el job de
  GitHub hasta el límite de 6 h — justo la falla que este plan existe para
  evitar. Deadline explícito (p. ej. 75 min) que falla el job con el BUILD_ID.
- **Y lleva trampa de cancelación.** `cancel-in-progress` cancela el *runner*,
  no el build ya enviado: el build viejo sigue vivo y llega igual a
  `gcloud run services update`. Un `trap` que corra
  `gcloud builds cancel $BUILD_ID` antes de salir es lo que de verdad cierra la
  carrera; el sondeo solo no basta.
- **Qué imprime cuando falla.** Hoy el runner transmite los logs, así que un
  build roto se diagnostica en la misma pantalla. Cambiarlo por un estado y un
  BUILD_ID es una **regresión** de diagnóstico. En cualquier estado terminal
  distinto de SUCCESS, el paso escribe al resumen del job: el paso que falló,
  las últimas ~100 líneas (`gcloud builds log $BUILD_ID`) y la URL de consola.
- **No es fire-and-forget, y eso es deliberado.** Un envío sin espera rompe en
  silencio `concurrency: deploy-dev, cancel-in-progress`: hoy ese candado
  funciona solo porque el runner sostiene el build entero. Sin la espera, dos
  pushes seguidos lanzan dos builds que se carreran a
  `gcloud run services update` y **el más viejo puede aterrizar al final**. Esa
  es exactamente la falla de "la demo del 12 de noviembre corrió el commit
  equivocado dos días". El sondeo conserva el candado y conserva la X roja.
- La X roja importa el doble aquí: la mitigación que este plan proponía en su
  primera versión (alerta por correo) **está rota y documentado que lo está** —
  el correo saliente de este proyecto cae en spam y quedó bloqueado en una
  credencial SMTP.
- Migraciones, rollout y smoke test se mueven a pasos de `cloudbuild.yaml` con
  `_DEPLOY=true`, conservando el orden: **esquema antes que código**; si la
  migración falla no se despliega nada.
- **Ojo con "el orden ya probado": ese camino nunca se ha ejecutado.** El paso
  `deploy` de `cloudbuild.yaml` está detrás de `_DEPLOY=true` y `deploy-dev.yml`
  manda `_DEPLOY=false` en cada run, así que lo probado son los pasos de GitHub,
  no los de Cloud Build. Además el paso de cloudbuild usa `gcloud run deploy`
  mientras dev siempre se ha desplegado con `gcloud run services update`. Se
  portan los pasos de GitHub **literalmente**, no se confía en los que ya están.
- **Dos pasos se quedan en el runner, después del sondeo, y hay que decirlo:**
  `Confirm nothing was left warm` (el guardia de costo de min-instances) y el
  resumen `if: always()` de qué commit corre dev. El plan no les asignaba dueño
  y son justo lo que evita una factura sorpresa y un "¿la demo está caída?".
- **El rollout va al final del build y es idempotente.** El `trap` de
  cancelación puede caer entre el servicio 5 y el 6 y dejar dev partido en dos
  SHAs; un runner cancelado tampoco corre el resumen `if: always()`, así que
  nadie sabría qué mitad aterrizó. El trap imprime el estado de imagen por
  servicio antes de salir.
- **No requiere cambios de Terraform.** `deploy/terraform/main.tf:183` ya le da
  al SA de build `roles/run.admin`, `roles/cloudsql.client`,
  `roles/iam.serviceAccountUser` y acceso al secreto
  `garcia-lorca-db-migrator-password-dev`, con un comentario que dice
  literalmente que es para que el deploy corra `scripts/migrate-cloud.sh` antes
  del rollout. La infraestructura lleva ahí desde el incidente del 2026-09-07.
- **Restricción de secuencia (bloqueante para probarlo):** un workflow
  `workflow_run` siempre ejecuta la copia de la **rama por defecto**, que aquí
  es `master`. Editar `deploy-dev.yml` en una rama **no tiene efecto** hasta que
  llega a `master`. T7 va al final de la etapa y no se puede validar en rama.

#### T8b — El contrato nunca se valida cuando cambia el contrato (defecto vivo)
No lo introduce este plan: ya pasa. `paths-ignore` excluye `docs/**` y el
contrato vive en `docs/api/openapi.yaml`, así que **un PR que solo toca el
contrato se salta CI entero** y `openapi (lint)` no corre justo cuando importa.
Contradice el canon spec-first (`api-standards.md`: el contrato se lintea ANTES
de implementar). `paths-ignore` no admite negaciones, así que la salida es
mover el contrato fuera de `docs/` o darle un disparador `paths:` propio.

#### T9 — Higiene de disparadores
- **NO por `paths-ignore`.** `paths-ignore` es una llave de `on:`, no de job:
  no existe forma de limitarla "a los jobs de Go". Agregar `apps/mobile/**`
  saltaría el workflow **completo** en un cambio solo-móvil, que es el único
  lugar donde el móvil se prueba (`pnpm turbo run test lint` corre todo el
  workspace), y dejaría el required check sin aparecer nunca: el PR se traba.
- La forma correcta es un filtro por archivos (el mismo script de T4, o
  `dorny/paths-filter`) que deje las patas de Go en no-op **mientras el workflow
  sigue reportando**. Hasta que exista ese filtro, T9 se limita a lo de abajo.
- `concurrency` de CI ya cancela runs superados; la llave `github.ref` es correcta.

#### T10 — Quitar de Cloud Build las compuertas que CI ya corrió (C6)
Borrar `go-vet`, `go-test`, `govulncheck`, `web-build` y `sourcemap-gate` de
`cloudbuild.yaml`; dejar `buf generate`, las diez imágenes, migraciones, rollout
y smoke. Esto es lo que hace que T7 **quepa** en los 120 min/día gratuitos de
e2-medium en vez de pasarse: sin esto, añadir migraciones y rollout a un build
que ya dura 58 min lo empuja arriba de 60, y dos deploys al día rozan el tope.
El tope de GCP es un corte duro que tira el ambiente de la demo; el de GitHub
solo frena CI.

**Guardia obligatoria que viene con T10.** Quitar las compuertas de Cloud Build
solo es correcto mientras TODO deploy venga después de un CI verde. Hoy no lo
es: `deploy-dev.yml:60` deja pasar `workflow_dispatch` a propósito, y un
`gcloud builds submit` a mano también. Sin guardia, un deploy manual publicaría
código sin `go test`, sin `govulncheck` y **sin la compuerta de source maps**,
que es regla del proyecto. Antes de enviar el build, el job verifica que el SHA
exacto tenga un check-run de CI en verde y falla nombrando el que falta;
saltarla exige una bandera explícita, no es el camino por defecto.
**Y necesita permiso.** `deploy-dev.yml` declara solo `contents: read` e
`id-token: write`, y su propio comentario dice que `checks: read` se quitó al
cambiar a `workflow_run`. Sin volver a agregarlo la guardia da 403 en **todos**
los deploys — y como es compuerta dura, los bloquea todos. T10 agrega
`checks: read` (y `actions: read` si consulta runs).

#### T11 — `make ci`: la misma compuerta, en la laptop, antes de empujar
La causa de raíz de 21 runs de PR a 48 min es que los fallos se descubren
**después** de empujar. El `Makefile` ya tiene `generate`, `lint`, `test` y
`build` con sus cadenas de ayuda `##`; falta el objetivo que los componga en
"lo que corre CI", más `actionlint`. No es un script nuevo: es un target que
reusa los que ya existen. `make ci-minutes` entra al lado de `deploy-preflight`,
con el mismo patrón.
**Objetivo honesto: verde en local en menos de 5 min *en caliente*, y el target
dice qué compuertas NO corre.** En una máquina fría son un `pnpm install`, la
compilación de 14 módulos de Go y testcontainers bajando Postgres — muy por
encima de 5 min según la propia cifra de 300 s por módulo en frío. Si el
criterio se escribe como "5 min en limpio", se va a cumplir quitándole
compuertas al target, que es justo lo contrario de lo que sirve.

#### T12 — Actualizar la documentación del pipeline
Cuatro cambios a "qué hace CI" (desaparece `go vet`, el deploy deja de
transmitir logs, Cloud Build deja de ser compuerta, aparece un script nuevo) y
cero tareas de documentación. `deploy/README.md` describe el pipeline actual y
quedaría mintiendo. Se actualiza: qué corre dónde, por qué `go vet` ya no está,
cómo se lee un deploy fallido, y cómo se corre `make ci` / `ci-minutes.sh`.
Documentación vieja aquí cuesta runs desperdiciados y hace que alguien vuelva a
proponer T8 dentro de tres meses.

### ETAPA 2 — después del 30 nov (no antes)

#### T3 — Colapsar la matriz de 14 patas a 4 shards
Agrupar los módulos por tiempo medido. Mata diez arranques y diez redondeos por
run. **Deuda que introduce:** la agrupación es una lista a mano que se desfasa
cuando se agrega un servicio; hoy `go.work` es la única fuente de verdad y esto
la rompe. Mitigación: un test que falle si un módulo de `go.work` no está en
ningún shard.

#### T4 — Correr solo los módulos que el diff puede afectar
Regla explícita, en un script con tests de tabla, no en YAML:
- Cambia `pkg/`, `gen/`, `proto/`, `go.work`, `go.work.sum` o el workflow →
  **todos** los shards.
- Si no, solo los shards con un `services/<x>` tocado.
- **La lista de archivos cambiados se lee por `env:`, nunca interpolando
  `${{ }}` dentro de un `run:`** — títulos de PR y nombres de rama son texto que
  un atacante controla, y `${{ }}` los mete como código shell.
- **Escotilla de escape, documentada junto a la regla:** un token `[ci full]` en
  el mensaje del commit (o una entrada de `workflow_dispatch`) fuerza las 14
  patas. Sin ella, cuando la heurística se equivoca la gente empuja commits
  vacíos para vencer el filtro, que es peor que no tener filtro.

#### T6 — Eliminar el job agregador `ci`
Con T3 los nombres son fijos y se registran como required checks directamente.

### RECHAZADO — T8 (no re-probar el push a `dev`)

**La premisa era falsa y la verifiqué.** El plan decía que el push a `dev`
re-verifica lo mismo que el PR. No:

1. `ci.yml:176-205` deja `-race`, integración y `govulncheck` en
   `if: github.event_name == 'pull_request'`. El push corre una compuerta
   **distinta y más débil**, no la misma.
2. Los merges a `dev` son **squash**: `git rev-list --parents -n1` devuelve un
   solo padre en `8534a22b`, `d2b40091`, `875978c5` y `bf86d107`. El commit que
   se despliega es un SHA que **ningún run de PR vio nunca**.
3. `check-runs` sobre `8534a22b` confirma que su CI verde viene del run de
   **push** (35397296141) — justamente el que T8 quería borrar.

T8 no eliminaba trabajo redundante: eliminaba **la única verificación que
existe sobre el árbol exacto que llega al colegio**. Los 935 min del push se
abaratan con T1 y T3; no se quitan.

## Ahorro proyectado

Revisado el 2026-09-18 contra el desglose real del build `667ce1bd`. La versión
anterior de esta tabla le daba 754 min a T7 asumiendo *enviar y salir*; sondear
—que es lo que se aprobó— deja al runner vivo durante el build, así que el
ahorro del deploy no sale de dejar de mirar, sale de que el build **dure menos**.

| Cambio | De | A | Ahorro |
| --- | ---: | ---: | ---: |
| T1+T2 — caché + compilaciones duplicadas | 1,619 | ~500 | ~1,120 |
| T5 — fundir openapi+actionlint en proto | 57 | 0 | ~57 |
| T10 — quitar del build las compuertas que CI ya corrió | — | — | ~255 (25.5 min × ~10 deploys) |
| T13 — `--cache-from` en las 10 imágenes | — | — | ~225 (estimado; se confirma al medir) |
| T7 — sondear con plazo, trap y diagnóstico | — | — | **0 min — es corrección, no ahorro** |
| **Etapa 1** | **2,739** | **~1,080** | **~1,660 (61%)** |
| T3+T4+T6 (Etapa 2) | ~1,080 | ~890 | ~190 (7%) |

T7 aparece con ahorro **cero** a propósito. Lo que compra es la X roja cuando un
build falla, el candado de concurrencia que impide que el commit viejo aterrice
al final, el `trap` que no deja builds huérfanos con autoridad para cambiar lo
que ve el colegio, y el paso que falló impreso en el run. El canal de alerta por
correo que lo sustituiría **está roto y documentado que lo está**.

El ahorro de T13 es el único número estimado de la tabla: se sabe que las
imágenes son 27.9 min sin caché, no cuánto bajan con él. `make ci-minutes` lo
confirma o lo desmiente después del primer par de deploys.

`web` (150 min) y `proto` (58 min) se quedan como están: tras la Etapa 1 pasan a
ser ~23% del presupuesto, y son baratos y útiles. Aceptado explícitamente.

## Verificación

Criterio de aceptación **por tasa**, no por total — el total depende del volumen
de trabajo del mes, que va a subir en cuanto se pueda empujar sin miedo:

1. **≤ 18 min facturados por run de CI** en la Etapa 1; **≤ 8 min** solo
   después de T3 (ver §Objetivo: 18 jobs × 1 min de piso).
2. **≤ 12 min facturados por deploy** (hoy: 58). CORREGIDO el 2026-09-18, por
   la misma aritmética que obligó a corregir el criterio 1: **≤ 2 min era
   imposible por construcción**. Ese número asumía *enviar y salir*, y las tres
   revisiones exigieron **sondear** el estado terminal del build para conservar
   la X roja y el candado de concurrencia. Sondear deja al runner vivo mientras
   el build corre, así que se factura igual: el piso de este criterio es la
   duración del build, no la del envío.
   Medido sobre el build `667ce1bd` (54.6 min de reloj): las compuertas eran
   25.5 min de la ruta crítica —las quita T10— y las imágenes 27.9 min —las baja
   T13 con `--cache-from`, porque Cloud Build entrega un daemon de Docker frío
   en cada build y no había de dónde traer las capas. ≤ 12 min es el objetivo
   con caché caliente; un build tras cambiar `go.sum` falla todos los
   `--cache-from` y no cuenta.
3. Meta escalonada del total: **≤ 1,100 min/ciclo éxito, ≤ 900 objetivo**.
   También corregido: el ≤ 900/700 original se apoyaba en que T7 ahorrara 754
   min. Sondeando ahorra ~480 con T10 y T13 incluidos. Sigue siendo una caída
   del ~60% sobre los 2,739 medidos.
3b. ✅ **CUMPLIDO 2026-09-19.** Ensayo de rollback contra el dev vivo: 146 s
   para volver los diez servicios a `8534a22b`, `/login` y `/readyz` en 200,
   157 s para volver a `f1fd6b34`, 200 otra vez. Cero minutos de Actions.
   Radio cero: los dos commits tienen código de aplicación idéntico.
   Destapó dos defectos y los dos quedaron corregidos: `make deploy-<svc>`
   usaba `gcloud run deploy` donde el pipeline usa `services update` (el mismo
   tema de E10, en un tercer sitio), y el README prometía un rollback
   imposible — `workflow_dispatch` toma el archivo del ref y no acepta un SHA
   suelto. El procedimiento quedó codificado en `make rollback-dev TAG=<sha>`,
   que se niega a empezar si falta una sola de las diez imágenes.
4. `actionlint` verde sobre los dos workflows (job `proto`), versión pineada.
4b. `make ci` verde en local en menos de 5 min **en caliente**, con la lista de
   compuertas que omite escrita en su cadena de ayuda `##`.
4c. `scripts/ci-minutes.sh` corre a la primera con UN comando documentado
   (`make ci-minutes`) y ante un 403/404 imprime el comando que lo arregla.
   Ajustado: el remedio NO es `gh auth refresh -s user`. El script evita a
   propósito los endpoints de facturación —que sí piden el scope `user`, y por
   eso `gh api user/settings/billing/actions` da 404 en esta máquina— y deriva
   el costo de los tiempos de job, que sólo necesitan `repo`. Aconsejar `-s user`
   sería mandar al dueño a pedir un permiso que el script no usa.
4d. Un deploy manual (`workflow_dispatch`) sobre un SHA sin CI verde **falla**
   nombrando el check que falta.
4e. Un run cancelado a media carrera deja el build de Cloud Build cancelado
   también, no huérfano.
5. Tests de tabla para la regla de selección de shards (Etapa 2).
6. Un deploy real a dev: el commit desplegado es el que se envió, y un build
   fallado **pone en rojo** el run de GitHub. Esto se prueba a propósito
   rompiendo un build.
7. `scripts/ci-minutes.sh` a los 30 días.
8. `deploy/README.md` describe el pipeline que existe después del cambio.

## Lo que NO entra en este plan

- **Comprar minutos** (el excedente cuesta ~$20 USD/mes aun al doble del
  consumo). RECHAZADO en firme: el dueño confirmó el 2026-09-18 que el límite de
  gasto está deliberadamente en $0. La bifurcación "si no está en $0, reducir el
  plan a T1+T2+T7" queda cerrada.
- **Hacer el repo público** (Actions es gratis e ilimitado en repos públicos).
  Rechazado: es el sistema de gestión de un colegio que se entrega en noviembre;
  publicarlo publica el modelo de auth, las políticas RLS y el hueco conocido
  del limitador global de login, además de regalar el entregable del dueño.
- **Runners auto-hospedados.** El motivo real es que no hay apetito de agregar
  superficie operativa nueva a 73 días de la entrega — no que "el stack local se
  muere", que es una memoria sobre un binario bajo `air` en esta laptop, no sobre
  un runner administrado.
- Cambiar de proveedor de CI.
- Optimizar los Dockerfile (el build vive en el tramo gratuito de Cloud Build).

## Notas de contexto

- La memoria del proyecto dice que `Deploy dev` está bloqueado desde el 10-sep
  por `govulncheck@latest`. **Está desactualizada**: `cloudbuild.yaml` ya pinea
  `_GOVULNCHECK_VERSION: v1.7.0` y hubo un deploy exitoso el 2026-09-14.
- La memoria dice que hay 47 commits sin empujar. **También desactualizada**:
  `origin/dev == dev == 8534a22b`. Todo está empujado, y eso explica el 90%.


<!-- autoplan-accepted:ceo -->
- Confirmar el límite de gasto de Actions ($0 o no) ANTES de empezar; el plan entero depende de esa premisa. Verificación: captura de Settings → Billing → Spending limits.
- Nombrar el costo en dólares del excedente (~$20 USD/mes al doble de consumo) en `## Problema`.
- T8 se elimina del plan. Verificación: `git rev-list --parents -n1` da un padre en los merges a dev (squash) y `check-runs` sobre 8534a22b muestra que su CI verde viene del run de push.
- T1 se rediseña: llave por shard sin `github.run_id`, `GOMODCACHE` en una entrada compartida, guardado solo en push. Precedido por un run instrumentado que mida el tamaño real. Verificación: la medición se publica antes de fijar el número de shards.
- T2 conserva `govulncheck` e integración en la ruta del PR. Verificación: son los únicos pasos de Go rojos en 58 runs (35394352750, 35395204982).
- T7 sondea el estado terminal del build en vez de enviar y salir. Verificación: romper un build a propósito y confirmar que el run de GitHub queda en rojo.
- T7 documenta que no requiere Terraform (main.tf:183) y que un workflow_run corre la copia de master.
- Nuevo T10: quitar de cloudbuild.yaml las compuertas que CI ya corrió, para que T7 quepa en los 120 min/día gratuitos.
- T5 absorbe `actionlint` (no estaba instalado) y pinea `@redocly/cli`.
- T4 lee los archivos cambiados por `env:`, nunca por `${{ }}` dentro de un `run:`.
- T3/T4/T6 pasan a Etapa 2 (después del 30 nov). T3 lleva un test que falle si un módulo de go.work no está en ningún shard.
- El criterio de aceptación pasa a ser una tasa (≤8 min/run de CI, ≤2 min/deploy) con meta escalonada 900/700.
- Los motivos de rechazo de runners auto-hospedados, comprar minutos y repo público quedan escritos explícitamente.
<!-- /autoplan-accepted:ceo -->

<!-- autoplan-accepted:dx -->
- T1 usa `matrix.module` como unidad en la Etapa 1, no "shard": T3 es Etapa 2 y la tarea no se podia implementar como estaba escrita. Verificacion: la llave del cache no menciona shards antes del 30 nov.
- El plan B de T1 si no cabe en 10 GB es cachear solo los modulos caros, no reducir shards. Verificacion: el run de medicion decide y su salida queda en el PR.
- T1 anade un paso que imprime el tamano restaurado del cache y falla en push a `dev` bajo un umbral. Verificacion: un cache vaciado a proposito pone el run en rojo.
- T1 documenta en el comentario del workflow que una rama que toca `go.sum` compila en frio hasta el merge.
- T5 pinea `actionlint` con `ACTIONLINT_VERSION`, igual que `@redocly/cli`.
- T7 lleva deadline de sondeo explicito que falla el job con el BUILD_ID.
- T7 lleva `trap` con `gcloud builds cancel $BUILD_ID`: `cancel-in-progress` cancela el runner, no el build. Verificacion: cancelar un run a media carrera deja el build cancelado, no huerfano.
- T7 imprime, en cualquier estado terminal distinto de SUCCESS, el paso que fallo, las ultimas ~100 lineas del log y la URL de consola. Sin esto el cambio es una regresion de diagnostico.
- T10 viene con guardia: el job verifica un check-run de CI verde sobre el SHA exacto antes de enviar el build, y falla nombrando el que falta. Verificacion: un `workflow_dispatch` sobre un SHA sin CI verde falla.
- T4 (Etapa 2) lleva escotilla `[ci full]` documentada junto a la regla.
- Nuevo T11: `make ci` que compone los targets `generate`/`lint`/`test` que YA existen en el Makefile, mas actionlint; y `make ci-minutes` al lado de `deploy-preflight`. Verificacion: verde en local en menos de 5 min.
- Nuevo T12: actualizar `deploy/README.md` con el pipeline posterior al cambio.
- T0 documenta los scopes de `gh` e imprime el `gh auth refresh -s user` ante un 403.
- El bloqueante del limite de gasto trae comando exacto y regla de bifurcacion: si no es $0, solo T1+T2+T7.
<!-- /autoplan-accepted:dx -->

<!-- autoplan-accepted:eng -->
- T2 corrige el comando: `go vet ./...` NO existe aqui (no hay go.mod raiz; verificado: "pattern ./...: directory prefix . does not contain modules listed in go.work"). La forma correcta es `go vet $(go work edit -json | jq -r '.Use[].DiskPath+"/..."')`. REGRESION: el texto anterior borraba 14 vets que funcionan y los cambiaba por un comando que falla en la primera linea.
- T1 quita `github.sha` de la llave del cache: rotaba igual que `run_id` y volvia a producir la misma churn. Se usa `CACHE_EPOCH` manual para invalidar. Verificacion: la llave primaria acierta en el segundo push.
- T1 arregla tambien la llave de turbo (`turbo-...-${{ github.sha }}`), que compite por los mismos 10 GB.
- T1 mide tiempo de restore, no solo tamano: un modulo cuyo restore cuesta mas que su compilacion en frio no se cachea.
- T1 salta la asercion de tamano cuando `cache-hit != 'true'`, para que el primer push no se ponga rojo por construccion.
- T5 edita `needs` y la cadena `results` del job `ci` (ci.yml:319 y :325). REGRESION: borrar el job `contract` sin eso deja el workflow sin parsear y TODOS los runs fallan, incluido el disparador del deploy.
- T10 agrega `checks: read` (y `actions: read` si aplica) a `deploy-dev.yml`. REGRESION: hoy solo tiene `contents: read` e `id-token: write`, asi que la guardia daria 403 y bloquearia todos los deploys.
- T9 NO usa `paths-ignore` para el movil: es una llave de `on:`, no de job, y saltaria el workflow entero dejando el required check sin aparecer. Se usa un filtro por archivos que deja las patas en no-op y el workflow reportando.
- Nuevo T8b: el contrato nunca se valida cuando cambia el contrato — `docs/**` esta en `paths-ignore` y el contrato es `docs/api/openapi.yaml`. Defecto vivo, no introducido por este plan.
- T7 porta los pasos de GitHub literalmente: el camino `_DEPLOY=true` de cloudbuild NUNCA se ha ejecutado y usa `gcloud run deploy` donde dev siempre uso `services update`.
- T7 deja en el runner, despues del sondeo, el guardia de min-instances y el resumen `if: always()` de que commit corre dev.
- T7 pone el rollout al final y lo hace idempotente; el trap imprime el estado de imagen por servicio antes de salir, porque una cancelacion a media carrera puede dejar dev partido en dos SHAs.
- El criterio de CI pasa a ≤18 min/run en Etapa 1 y ≤8 min solo tras T3: con 18 jobs y cobro por minuto, el piso aritmetico es 18 min. Verificacion: el run 35396038508 tiene 18 jobs.
- T11 apunta a <5 min EN CALIENTE y su cadena `##` nombra las compuertas que omite.
- Se agrega un ensayo de rollback: volver dev al SHA anterior y confirmar que responde.
<!-- /autoplan-accepted:eng -->
## Review record

<!-- autoplan-baseline-edits:ceo {"sourceSha256":"5a2297fc1eb45cc81a83fb8cd1ae8e5662ffe9dca989037def954e2e64bc93f3","replacements":[{"oldText":"# PLAN — Bajar el consumo de minutos de GitHub Actions\n\nEstado: BORRADOR (entra a /autoplan)\nRama: `feature/optimizacion-minutos-ci`\nFecha: 2026-09-18\n\n## Problema\n\nGitHub reporta **2,700 min usados de 3,000 incluidos (90%)** en el ciclo de\nfacturación actual. El repo es privado, así que cada minuto se cobra contra la\ncuota. Al ritmo actual el mes se agota antes de terminar, y cuando se agota\n**no hay CI y no hay deploy** — con demos diarias para el colegio y entrega el\n30 nov 2026, quedarse sin minutos a media semana es quedarse sin poder desplegar.\n\n## Medición (no estimación)\n\nMedido con la API de Actions, 94 runs entre 2026-08-19 y 2026-09-18, sumando\n`ceil(duración_del_job / 60s)` por job — que es exactamente como factura GitHub.\n(El endpoint `/timing` devuelve `total_ms: 0` en este repo y no sirve.)\n\n**Total medido: 2,739 min** — coincide con los 2,700 que reporta la factura.\n\n| Bucket | Minutos | % | Runs |\n| --- | ---: | ---: | ---: |\n| `Deploy dev` → job `build images and roll out` | 789 | 29% | 35 |\n| `CI` → matriz `go` (14 módulos de `go.work`) | 1,619 | 59% | 50 |\n| `CI` → job `web` | 150 | 5.5% | 58 |\n| `CI` → `proto` + `openapi` + `ci` (3 jobs triviales) | 173 | 6.3% | 58 c/u |\n\nPor tipo de evento:\n\n| Evento | Minutos | Runs | Promedio |\n| --- | ---: | ---: | ---: |\n| `CI` / pull_request | 1,015 | 21 | **48.3 min/run** |\n| `CI` / push a `dev` | 935 | 37 | **25.3 min/run** |\n| `Deploy dev` / workflow_run | 708 | 25 | 28.3 min/run (58 min los exitosos) |\n| `Deploy dev` / workflow_dispatch + push | 81 | 11 | — |\n\n## Causas raíz (verificadas en logs, no inferidas)\n\n### C1 — El caché de Go es una lápida de 7.6 KB\n`actions/setup-go@v6` cachea `GOCACHE` + `GOMODCACHE` con una llave derivada de\n`hashFiles('**/go.sum')`. Las 14 patas de la matriz calculan **la misma llave**.\nLas entradas de caché de Actions son inmutables: la primera pata que ganó la\ncarrera guardó un caché casi vacío y quedó congelado ahí para siempre.\n\nLog real del run 35396038508, pata `go (./services/iam)`:\n\n```\nCache hit for: setup-go-Linux-x64-ubuntu24-go-1.25.14-f4c882bf...\nCache Size: ~0 MB (7599 B)\nCache restored successfully\ngo: downloading cloud.google.com/go/firestore v1.22.0\n...\nCache hit occurred on the primary key ..., not saving cache.\n```\n\nReporta \"cache hit\", restaura 7.6 KB, y acto seguido **descarga todos los\nmódulos y compila el grafo completo desde cero** — en cada pata, en cada run,\ndesde que existe el workflow. Y como la llave ya existe, nunca vuelve a\nguardar: el caché no se puede auto-reparar.\n\nCosto por pata en ese run: `go vet` 109 s + `go test -race` 112 s +\n`integration` 43 s + `govulncheck` 27 s = **307 s**, casi todo compilación fría.\n\n### C2 — El mismo código se compila cuatro veces por pata\n`go vet ./...` compila el grafo entero (109 s). `go test -race` lo vuelve a\ncompilar con instrumentación de carreras (112 s). El paso de integración lo\ncompila otra vez con `-tags=integration`. `govulncheck` carga el grafo una\ncuarta vez. `go test` **ya corre el subconjunto de vet de alta confianza**, así\nque el paso `go vet` separado es trabajo duplicado casi íntegro.\n\n### C3 — Se paga un runner de GitHub por mirar un build de Google\nEl job `build images and roll out` dura **~3,500 s (58 min) en los deploys\nexitosos**. Lo que hace es `gcloud builds submit --config=cloudbuild.yaml`, que\n**transmite logs de forma síncrona** mientras Cloud Build construye las diez\nimágenes en la infraestructura de Google. El runner de GitHub no compila nada:\nespera, y factura mientras espera. ~10 deploys exitosos × 58 min = **~580 min**\nde cuota gastados en un `tail -f`.\n\n### C4 — Impuesto de redondeo por job\nGitHub factura cada job redondeado al minuto completo. Medido:\n\n- `proto`: 17 s de trabajo → 1 min facturado × 58 runs = **58 min**\n- `openapi (lint)`: 9 s (y 3 de esos son `npx --yes @redocly/cli@latest`,\n  que baja el CLI entero cada vez) → 1 min × 58 runs = **57 min**\n- `ci` (un `if` de cinco líneas que agrega resultados) → 1 min × 58 = **58 min**\n- matriz `go`: promedio real 153 s en PR → 3 min facturados; 81 s en push →\n  2 min facturados. El redondeo solo, sobre 700 jobs de matriz, son **~400 min**.\n\nSon ~570 min/ciclo que no compran ninguna verificación.\n\n### C5 — Cada commit se prueba dos veces\nUn PR a `dev` corre la compuerta completa (48 min). Al hacer merge, el push a\n`dev` corre la compuerta rápida sobre **el mismo árbol** (25 min) — y esa es la\núnica razón por la que existe el push CI: `Deploy dev` se dispara con\n`workflow_run` de CI. 935 min del ciclo son re-verificación.\n\n### C6 — La matriz no mira el diff\nCambiar un archivo en `apps/web` corre las 14 patas de Go. Cambiar\n`services/finance` corre las otras trece. `paths-ignore` solo excluye markdown,\n`docs/` y `.claude/`.\n\n## Objetivo\n\nBajar de ~2,739 min/ciclo a **≤ 700 min/ciclo** sin debilitar ninguna compuerta\nque hoy atrapa defectos reales (RLS, carreras, vulnerabilidades, source maps,\ncontrato OpenAPI), y bajar el tiempo de pared de un deploy.\n\n## Plan de implementación\n\n### T0 — Línea base medible antes de tocar nada\nCommitear `scripts/ci-minutes.sh`: consulta la API de Actions, suma\n`ceil(job_duration/60)` y reporta por workflow, evento y nombre de job.\nEs la prueba que falla primero: hoy imprime ~2,739 min y el criterio de\naceptación de este plan es que el mismo script imprima ≤ 700 sobre los 30 días\nsiguientes al merge. Sin esto, \"mejoró\" es una opinión.\n\n### T1 — Reparar el caché de Go (la palanca más grande de CI)\n- `cache: false` en `actions/setup-go` (deja de emitir la llave compartida).\n- Paso `actions/cache@v5` explícito, con llave **por módulo** y sufijo rotatorio\n  para que siempre guarde una versión nueva:\n  - `key: go-${{ runner.os }}-${{ matrix.module }}-${{ hashFiles('**/go.sum') }}-${{ github.run_id }}`\n  - `restore-keys:` la misma llave sin `run_id`, y luego sin el hash.\n- Rutas: `~/go/pkg/mod` y `~/.cache/go-build`.\n- El caché se ceba en los push a `dev`; los PR que apuntan a `dev` heredan el\n  caché de la rama base (regla de alcance de caché de Actions).\n- Vigilar el límite de 10 GB por repo: `GOMODCACHE` es común a todos los\n  módulos, así que va en **una sola** entrada compartida (llave por `go.sum`,\n  guardada por un job dedicado); `GOCACHE` va por shard.\n\n### T2 — Dejar de compilar lo mismo cuatro veces\n- Borrar el paso `go vet` por módulo. `go test` ya corre el subconjunto de vet.\n- Correr `go vet ./...` **una vez** sobre todo el workspace, en un job propio y\n  con caché caliente, en lugar de catorce veces.\n- `govulncheck`: instalar el binario **una vez** y compartirlo por artefacto o\n  caché, en vez de `go install` en cada pata.\n\n### T3 — Colapsar la matriz de 14 patas a 4 shards\nAgrupar los módulos de `go.work` en cuatro grupos balanceados por tiempo medido\n(iam/students/finance/comms son los caros; gen/go, docs, billing, scholarships\nlos baratos). Mata diez rondas de arranque y diez impuestos de redondeo por run,\nconservando paralelismo. Los nombres de los shards son fijos, así que sirven\ncomo required checks.\n\n### T4 — Correr solo los módulos que el diff puede afectar\nRegla explícita, no heurística:\n- Si cambia `pkg/`, `gen/`, `proto/`, `go.work`, `go.work.sum` o el propio\n  workflow → **todos** los shards.\n- Si no, solo los shards que contienen un `services/<x>` tocado.\nLa regla vive en un script con tests de tabla, no en una expresión de YAML.\n\n### T5 — Fundir `openapi (lint)` dentro del job `proto`\nMismo checkout, mismo runner. Y pinear `@redocly/cli` a una versión en vez de\n`@latest` (baja el CLI entero cada run y es una dependencia que un tercero\npuede romper — mismo error que costó el deploy el 2026-09-10 con\n`govulncheck@latest`).\n\n### T6 — Eliminar el job agregador `ci`\nCon T3 los nombres de los jobs son fijos y se registran directamente como\nrequired checks en la protección de rama. El job `ci` existe solo porque la\nmatriz tenía nombres dinámicos.\n\n### T7 — Dejar de pagar por mirar Cloud Build\n- El job de GitHub envía el build con `gcloud builds submit --async`, imprime\n  el ID del build y **termina**. Costo: ~1 min en lugar de 58.\n- Migraciones, rollout y smoke test se mueven a pasos de `cloudbuild.yaml` con\n  `_DEPLOY=true`, manteniendo el orden que ya está documentado y probado:\n  **esquema antes que código**, y si la migración falla no se despliega nada.\n- El SA de Cloud Build necesita: acceso al secreto\n  `garcia-lorca-db-migrator-password-dev`, `roles/cloudsql.client` y\n  `roles/run.admin`. Se agrega en `deploy/terraform/cicd.tf`.\n- El resumen \"qué commit está corriendo dev\" se conserva: pasa a ser un paso\n  final de Cloud Build que escribe a Cloud Logging, más una notificación.\n- Riesgo aceptado y mitigado: un build asíncrono ya no tiñe de rojo el run de\n  GitHub. Mitigación: alerta de Cloud Build a correo/Pub-Sub en fallo, más el\n  paso de smoke test que ya existe.\n\n### T8 — Dejar de probar dos veces el mismo árbol\n`Deploy dev` deja de depender de `workflow_run` de CI y pasa a dispararse en\n`push` a `dev`, verificando que el SHA ya tenga un check verde de CI (el del\nPR que acaba de hacer merge). El push CI se reduce a nada cuando el SHA ya\nestá verificado. Si el SHA no tiene CI verde (push directo a `dev`), corre la\ncompuerta completa antes de desplegar.\n\n### T9 — Higiene de disparadores\n- `paths-ignore` gana `apps/mobile/**` para los jobs de Go (el móvil no toca Go).\n- `concurrency` en CI ya cancela runs superados; verificar que también aplique\n  a `pull_request` con el mismo grupo (hoy la llave es `github.ref`, correcto).\n\n## Ahorro proyectado\n\n| Cambio | De | A | Ahorro |\n| --- | ---: | ---: | ---: |\n| T1+T2+T3+T4 — matriz Go | 1,619 | ~350 | ~1,270 |\n| T5+T6 — jobs triviales | 115 | ~0 | ~115 |\n| T7 — deploy asíncrono | 789 | ~35 | ~754 |\n| T8 — no re-probar | (dentro de los 935 de push) | — | ~200 |\n| **Total** | **2,739** | **~600** | **~2,140 (78%)** |\n\nTiempo de pared del deploy: baja de ~58 min visibles en GitHub a ~2 min de\nenvío; el build en Cloud Build sigue tardando lo mismo pero deja de bloquear y\ndeja de costar cuota.\n\n## Cómo se verifica\n\n1. `actionlint` sobre los dos workflows (compuerta nueva en CI, barata).\n2. Tests de tabla en Go o en shell para la regla de selección de shards (T4):\n   entradas = listas de archivos cambiados, salidas = shards esperados.\n3. Un PR de prueba que toca solo `services/finance` debe correr **un** shard.\n4. Un PR que toca `pkg/` debe correr **los cuatro**.\n5. Un deploy real a dev, verificando que el commit desplegado es el correcto.\n6. `scripts/ci-minutes.sh` a los 30 días: ≤ 700 min.\n\n## Lo que NO entra en este plan\n\n- Runners auto-hospedados (elimina la factura pero introduce una máquina que\n  debe estar encendida; el historial de esta máquina dice que el stack local\n  muere en silencio).\n- Cambiar de proveedor de CI.\n- Tocar `deploy/terraform` más allá de los permisos que T7 necesita.\n- Optimizar los Dockerfile o el tiempo de build de Cloud Build (es gratis\n  dentro de los 120 min/día de e2-medium; no consume cuota de GitHub).\n\n## Notas de contexto\n\n- Una memoria del proyecto dice que `Deploy dev` está bloqueado desde el 10-sep\n  por `govulncheck@latest`. **Está desactualizada**: `cloudbuild.yaml` ya pinea\n  `_GOVULNCHECK_VERSION: v1.7.0` y hubo un deploy exitoso el 2026-09-14.\n- Hay 47 commits sin empujar por instrucción del dueño, precisamente para no\n  gastar minutos. Este plan es lo que permite volver a empujar sin miedo.\n","newText":"# PLAN — Bajar el consumo de minutos de GitHub Actions\n\nEstado: REVISADO (CEO) — entra a DX y Eng\nRama: `feature/optimizacion-minutos-ci`\nFecha: 2026-09-18\n\n## Problema\n\nGitHub reporta **2,700 min usados de 3,000 incluidos (90%)** en el ciclo actual.\n\n**Cuánto dinero es, dicho explícitamente:** el excedente en repo privado cuesta\n**$0.008 USD/min**. Hoy el excedente es cero. Aun duplicando el consumo a 5,500\nmin/ciclo, la factura sería **~$20 USD/mes (~370 MXN)**. Así que este plan NO se\njustifica por dinero.\n\nSe justifica por una sola premisa, que hay que confirmar antes de empezar:\n**el límite de gasto de Actions está en $0**, igual que el corte duro de\nfacturación que el dueño eligió para GCP. Con el límite en $0, agotar la cuota\napaga CI y apaga el deploy — con demos **diarias** para el colegio y entrega el\n30 nov 2026, eso es quedarse sin poder desplegar a media semana. Con un límite\ndistinto de $0, el problema real no es la cuota sino el tiempo de pared, y\nsiete de las diez tareas de aquí sobran.\n\n> **Bloqueante:** confirmar el límite en\n> Settings → Billing → Spending limits → Actions. No pude verificarlo: el\n> endpoint de facturación pide el scope `user`, que esta sesión no tiene.\n\n## Medición (no estimación)\n\nMedido con la API de Actions, 94 runs entre 2026-08-19 y 2026-09-18, sumando\n`ceil(duración_del_job / 60s)` por job — que es exactamente como factura GitHub.\n(El endpoint `/timing` devuelve `total_ms: 0` en este repo y no sirve.)\n\n**Total medido: 2,739 min** — coincide con los 2,700 de la factura (1.4% de error).\n\n| Bucket | Minutos | % | Runs |\n| --- | ---: | ---: | ---: |\n| `Deploy dev` → job `build images and roll out` | 789 | 29% | 35 |\n| `CI` → matriz `go` (14 módulos de `go.work`) | 1,619 | 59% | 50 |\n| `CI` → job `web` | 150 | 5.5% | 58 |\n| `CI` → `proto` + `openapi` + `ci` (3 jobs triviales) | 173 | 6.3% | 58 c/u |\n\n| Evento | Minutos | Runs | Promedio |\n| --- | ---: | ---: | ---: |\n| `CI` / pull_request | 1,015 | 21 | **48.3 min/run** |\n| `CI` / push a `dev` | 935 | 37 | **25.3 min/run** |\n| `Deploy dev` / workflow_run | 708 | 25 | 28.3 (58 min los exitosos) |\n| `Deploy dev` / dispatch + push | 81 | 11 | — |\n\n**Advertencia sobre la línea base:** estos 2,739 min se midieron en un mes en el\nque hubo commits retenidos sin empujar justamente para ahorrar minutos. Comparar\nun total contra un mes sin esa restricción mide dos cargas de trabajo distintas.\nPor eso el criterio de aceptación es una **tasa**, no un total (ver §Verificación).\n\n## Causas raíz (verificadas en logs, no inferidas)\n\n### C1 — El caché de Go es una lápida de 7.6 KB\n`actions/setup-go@v6` cachea `GOCACHE` + `GOMODCACHE` con una llave derivada de\n`hashFiles('**/go.sum')`. Las 14 patas de la matriz calculan **la misma llave**.\nLas entradas de caché de Actions son inmutables: la primera pata que ganó la\ncarrera guardó un caché casi vacío y quedó congelado ahí para siempre.\n\nLog del run 35396038508, pata `go (./services/iam)`:\n\n```\nCache hit for: setup-go-Linux-x64-ubuntu24-go-1.25.14-f4c882bf...\nCache Size: ~0 MB (7599 B)\nCache restored successfully\ngo: downloading cloud.google.com/go/firestore v1.22.0\n...\nCache hit occurred on the primary key ..., not saving cache.\n```\n\nReporta \"cache hit\", restaura 7.6 KB, y acto seguido **descarga todos los\nmódulos y compila el grafo completo desde cero** — en cada pata, en cada run,\ndesde que existe el workflow. Como la llave ya existe, nunca vuelve a guardar:\nel caché no se puede auto-reparar.\n\nCosto por pata en ese run: `go vet` 109 s + `go test -race` 112 s +\n`integration` 43 s + `govulncheck` 27 s = **307 s**, casi todo compilación fría.\n\n### C2 — El mismo código se compila cuatro veces por pata\n`go vet ./...` compila el grafo entero (109 s). `go test -race` lo recompila con\ninstrumentación (112 s). Integración lo compila otra vez con `-tags`.\n`govulncheck` carga el grafo una cuarta vez. `go test` **ya corre el\nsubconjunto de vet de alta confianza**.\n\n**Evidencia de que `go vet` no compra nada:** en 58 runs de CI, `go vet` y\n`go test -race` fallaron **cero** veces. Los únicos pasos de Go que se pusieron\nen rojo fueron `govulncheck` (9 patas, run 35394352750) y\n`go test -tags=integration` (students, runs 35394352750 y 35395204982). Esos dos\nse quedan en la ruta del PR: son todo el valor demostrado de la matriz.\n\n### C3 — Se paga un runner de GitHub por mirar un build de Google\nEl job `build images and roll out` dura **~3,500 s (58 min) en los deploys\nexitosos**. Hace `gcloud builds submit`, que **transmite logs de forma síncrona**\nmientras Cloud Build construye las diez imágenes en infraestructura de Google.\nEl runner no compila nada: espera, y factura mientras espera. ~10 deploys\nexitosos × 58 min = **~580 min** de cuota gastados en un `tail -f`.\n\n### C4 — Impuesto de redondeo por job\nGitHub factura cada job redondeado al minuto completo:\n\n- `proto`: 17 s de trabajo → 1 min × 58 runs = **58 min**\n- `openapi (lint)`: 9 s (3 de ellos bajando `@redocly/cli@latest`) → **57 min**\n- `ci` (un `if` de cinco líneas) → **58 min**\n- matriz `go`: 153 s reales → 3 min facturados en PR; 81 s → 2 min en push.\n  Solo el redondeo, sobre 700 jobs de matriz, son **~400 min**.\n\n### C5 — La matriz no mira el diff\nCambiar un archivo en `apps/web` corre las 14 patas de Go. `paths-ignore` solo\nexcluye markdown, `docs/` y `.claude/`.\n\n### C6 — Los dos pipelines corren la MISMA suite de compuertas\n`cloudbuild.yaml` vuelve a correr, sobre el mismo commit que CI acaba de\naprobar: `buf lint`, `go vet` (todos los módulos), `go test -race` (todos los\nmódulos), `govulncheck`, `web build` y la compuerta de source maps. Eso es la\nmayor parte de los 58 min de build. CI es la compuerta; Cloud Build debería\nconstruir imágenes y desplegar, nada más.\n\n## Objetivo\n\nQue un run de CI cueste **≤ 8 min facturados** y un deploy **≤ 2 min\nfacturados**, sin quitar ninguna compuerta que haya atrapado algo real.\n\n## Plan de implementación\n\nDos etapas. La Etapa 1 lleva ~94% del ahorro con el riesgo bajo; la Etapa 2\nlleva ~6% y concentra todo el riesgo, así que va después del 30 nov.\n\n### ETAPA 1 — antes del 30 nov\n\n#### T0 — Línea base medible antes de tocar nada\n`scripts/ci-minutes.sh`, en el estilo de `scripts/deploy-preflight.sh`\n(`/bin/sh`, solo lectura, helpers `ok/bad/warn`, exit 1): consulta la API de\nActions, suma `ceil(job_duration/60)` y reporta **min por run de CI** y **min\npor deploy**, no solo el total. Es la prueba que falla primero: hoy imprime\n~48 min/run de PR y ~58 min/deploy.\n\n#### T1 — Reparar el caché de Go (la palanca más grande de CI)\nEl diseño con `github.run_id` en la llave **no sirve**: medido en esta máquina,\n`GOMODCACHE` son **6.0 GB** y `GOCACHE` **1.0 GB**, contra un presupuesto LRU de\n**10 GB por repo**. Rotar la llave cada run llenaría y desalojaría el caché en\nmenos de un día, y los PR restaurarían basura o nada. La lápida se arregla\nhaciendo la llave **por shard**, no rotándola.\n\n- Paso 1: **medir**. Un run instrumentado que imprima el tamaño real de\n  `GOCACHE` y `GOMODCACHE` por shard en Linux. El número decide el resto.\n- `cache: false` en `actions/setup-go` (deja de emitir la llave compartida).\n- **`GOMODCACHE`**: UNA entrada compartida, llave\n  `gomod-${{ hashFiles('**/go.sum') }}`, sin rotación (cambia cuando cambian las\n  dependencias, que es lo correcto). La guarda un job dedicado.\n- **`GOCACHE`**: una entrada **por shard**, llave\n  `gocache-<shard>-${{ hashFiles('**/go.sum') }}-${{ github.sha }}` con\n  `restore-keys` que caen al prefijo del shard.\n- **Guardar solo en push a `dev`** (`if: github.event_name == 'push'`). Los PR\n  restauran y no escriben: así no se desaloja nada y se cierra de paso el vector\n  de envenenamiento de caché desde una rama.\n- Si la medición dice que no cabe, bajar a 2 shards antes que sacrificar el caché.\n\n#### T2 — Dejar de compilar lo mismo cuatro veces\n- Borrar el paso `go vet` por módulo (cero hallazgos en 58 runs, 109 s × 14 × 50).\n- `go vet ./...` **una vez** sobre todo el workspace, en un job con caché caliente.\n- `govulncheck`: instalar el binario **una vez** y compartirlo por caché, en vez\n  de `go install` en cada pata. El paso se queda en la ruta del PR: es uno de los\n  dos únicos que han atrapado algo.\n\n#### T5 — Fundir `openapi (lint)` y `actionlint` dentro del job `proto`\nMismo checkout, mismo runner, cero minutos extra. Pinear `@redocly/cli` a una\nversión en vez de `@latest` — mismo error que tumbó el deploy el 2026-09-10 con\n`govulncheck@latest`. `actionlint` entra aquí (el job ya tiene Go): la\nverificación del plan lo daba por instalado y no lo está.\nNota: `openapi (lint)` es el job que **más veces se ha puesto en rojo** (9 de 58\nruns), así que fundirlo no lo debilita — lo abarata.\n\n#### T7 — Enviar el build y esperar el veredicto, no los logs\n- `gcloud builds submit --async`, capturar el BUILD_ID, y **sondear el estado\n  terminal** con `gcloud builds describe` cada ~30 s. Costo: **~2-3 min\n  facturados** en vez de 58.\n- **No es fire-and-forget, y eso es deliberado.** Un envío sin espera rompe en\n  silencio `concurrency: deploy-dev, cancel-in-progress`: hoy ese candado\n  funciona solo porque el runner sostiene el build entero. Sin la espera, dos\n  pushes seguidos lanzan dos builds que se carreran a\n  `gcloud run services update` y **el más viejo puede aterrizar al final**. Esa\n  es exactamente la falla de \"la demo del 12 de noviembre corrió el commit\n  equivocado dos días\". El sondeo conserva el candado y conserva la X roja.\n- La X roja importa el doble aquí: la mitigación que este plan proponía en su\n  primera versión (alerta por correo) **está rota y documentado que lo está** —\n  el correo saliente de este proyecto cae en spam y quedó bloqueado en una\n  credencial SMTP.\n- Migraciones, rollout y smoke test se mueven a pasos de `cloudbuild.yaml` con\n  `_DEPLOY=true`, conservando el orden ya probado: **esquema antes que código**;\n  si la migración falla no se despliega nada.\n- **No requiere cambios de Terraform.** `deploy/terraform/main.tf:183` ya le da\n  al SA de build `roles/run.admin`, `roles/cloudsql.client`,\n  `roles/iam.serviceAccountUser` y acceso al secreto\n  `garcia-lorca-db-migrator-password-dev`, con un comentario que dice\n  literalmente que es para que el deploy corra `scripts/migrate-cloud.sh` antes\n  del rollout. La infraestructura lleva ahí desde el incidente del 2026-09-07.\n- **Restricción de secuencia (bloqueante para probarlo):** un workflow\n  `workflow_run` siempre ejecuta la copia de la **rama por defecto**, que aquí\n  es `master`. Editar `deploy-dev.yml` en una rama **no tiene efecto** hasta que\n  llega a `master`. T7 va al final de la etapa y no se puede validar en rama.\n\n#### T9 — Higiene de disparadores\n- `paths-ignore` gana `apps/mobile/**` para los jobs de Go (el móvil no toca Go).\n- `concurrency` de CI ya cancela runs superados; la llave `github.ref` es correcta.\n\n#### T10 — Quitar de Cloud Build las compuertas que CI ya corrió (C6)\nBorrar `go-vet`, `go-test`, `govulncheck`, `web-build` y `sourcemap-gate` de\n`cloudbuild.yaml`; dejar `buf generate`, las diez imágenes, migraciones, rollout\ny smoke. Esto es lo que hace que T7 **quepa** en los 120 min/día gratuitos de\ne2-medium en vez de pasarse: sin esto, añadir migraciones y rollout a un build\nque ya dura 58 min lo empuja arriba de 60, y dos deploys al día rozan el tope.\nEl tope de GCP es un corte duro que tira el ambiente de la demo; el de GitHub\nsolo frena CI.\n\n### ETAPA 2 — después del 30 nov (no antes)\n\n#### T3 — Colapsar la matriz de 14 patas a 4 shards\nAgrupar los módulos por tiempo medido. Mata diez arranques y diez redondeos por\nrun. **Deuda que introduce:** la agrupación es una lista a mano que se desfasa\ncuando se agrega un servicio; hoy `go.work` es la única fuente de verdad y esto\nla rompe. Mitigación: un test que falle si un módulo de `go.work` no está en\nningún shard.\n\n#### T4 — Correr solo los módulos que el diff puede afectar\nRegla explícita, en un script con tests de tabla, no en YAML:\n- Cambia `pkg/`, `gen/`, `proto/`, `go.work`, `go.work.sum` o el workflow →\n  **todos** los shards.\n- Si no, solo los shards con un `services/<x>` tocado.\n- **La lista de archivos cambiados se lee por `env:`, nunca interpolando\n  `${{ }}` dentro de un `run:`** — títulos de PR y nombres de rama son texto que\n  un atacante controla, y `${{ }}` los mete como código shell.\n\n#### T6 — Eliminar el job agregador `ci`\nCon T3 los nombres son fijos y se registran como required checks directamente.\n\n### RECHAZADO — T8 (no re-probar el push a `dev`)\n\n**La premisa era falsa y la verifiqué.** El plan decía que el push a `dev`\nre-verifica lo mismo que el PR. No:\n\n1. `ci.yml:176-205` deja `-race`, integración y `govulncheck` en\n   `if: github.event_name == 'pull_request'`. El push corre una compuerta\n   **distinta y más débil**, no la misma.\n2. Los merges a `dev` son **squash**: `git rev-list --parents -n1` devuelve un\n   solo padre en `8534a22b`, `d2b40091`, `875978c5` y `bf86d107`. El commit que\n   se despliega es un SHA que **ningún run de PR vio nunca**.\n3. `check-runs` sobre `8534a22b` confirma que su CI verde viene del run de\n   **push** (35397296141) — justamente el que T8 quería borrar.\n\nT8 no eliminaba trabajo redundante: eliminaba **la única verificación que\nexiste sobre el árbol exacto que llega al colegio**. Los 935 min del push se\nabaratan con T1 y T3; no se quitan.\n\n## Ahorro proyectado\n\n| Cambio | De | A | Ahorro |\n| --- | ---: | ---: | ---: |\n| T1+T2 — caché + compilaciones duplicadas | 1,619 | ~500 | ~1,120 |\n| T5 — fundir openapi+actionlint en proto | 57 | 0 | ~57 |\n| T7 — enviar y sondear en vez de mirar | 789 | ~35 | ~754 |\n| **Etapa 1** | **2,739** | **~890** | **~1,930 (70%)** |\n| T3+T4+T6 (Etapa 2) | ~890 | ~700 | ~190 (7%) |\n\n`web` (150 min) y `proto` (58 min) se quedan como están: tras la Etapa 1 pasan a\nser ~23% del presupuesto, y son baratos y útiles. Aceptado explícitamente.\n\n## Verificación\n\nCriterio de aceptación **por tasa**, no por total — el total depende del volumen\nde trabajo del mes, que va a subir en cuanto se pueda empujar sin miedo:\n\n1. **≤ 8 min facturados por run de CI** (hoy: 48 en PR, 25 en push).\n2. **≤ 2 min facturados por deploy** (hoy: 58).\n3. Meta escalonada del total: **≤ 900 min/ciclo éxito, ≤ 700 objetivo**.\n4. `actionlint` verde sobre los dos workflows (dentro del job `proto`).\n5. Tests de tabla para la regla de selección de shards (Etapa 2).\n6. Un deploy real a dev: el commit desplegado es el que se envió, y un build\n   fallado **pone en rojo** el run de GitHub. Esto se prueba a propósito\n   rompiendo un build.\n7. `scripts/ci-minutes.sh` a los 30 días.\n\n## Lo que NO entra en este plan\n\n- **Comprar minutos** (el excedente cuesta ~$20 USD/mes aun al doble del\n  consumo). Rechazado solo si el límite de gasto está deliberadamente en $0; si\n  no lo está, ésta es la opción correcta y este plan se reduce a T1+T2+T7.\n- **Hacer el repo público** (Actions es gratis e ilimitado en repos públicos).\n  Rechazado: es el sistema de gestión de un colegio que se entrega en noviembre;\n  publicarlo publica el modelo de auth, las políticas RLS y el hueco conocido\n  del limitador global de login, además de regalar el entregable del dueño.\n- **Runners auto-hospedados.** El motivo real es que no hay apetito de agregar\n  superficie operativa nueva a 73 días de la entrega — no que \"el stack local se\n  muere\", que es una memoria sobre un binario bajo `air` en esta laptop, no sobre\n  un runner administrado.\n- Cambiar de proveedor de CI.\n- Optimizar los Dockerfile (el build vive en el tramo gratuito de Cloud Build).\n\n## Notas de contexto\n\n- La memoria del proyecto dice que `Deploy dev` está bloqueado desde el 10-sep\n  por `govulncheck@latest`. **Está desactualizada**: `cloudbuild.yaml` ya pinea\n  `_GOVULNCHECK_VERSION: v1.7.0` y hubo un deploy exitoso el 2026-09-14.\n- La memoria dice que hay 47 commits sin empujar. **También desactualizada**:\n  `origin/dev == dev == 8534a22b`. Todo está empujado, y eso explica el 90%.\n\n"}]} -->

### Fase 1 — CEO (SELECTIVE EXPANSION)

Voces: subagente Claude completado; Codex **no instalado** → sin cobertura
externa, marcado `[subagent-only]`. Las seis celdas de consenso son N/A, nunca
CONFIRMED.

**Tensión resuelta con datos:** la voz externa (F9) pedía mover `govulncheck` e
integración a un run nocturno y borrar T4. Medido sobre 58 runs: son los
**únicos dos pasos de Go que han fallado alguna vez**. Rechazado por evidencia.
T4 se difiere por otra razón (6% del beneficio, todo el riesgo).

#### "Lo que ya existe" (0B)
| Sub-problema | Ya existe | Uso |
| --- | --- | --- |
| Medir gasto | nada (los 9 scripts son de GCP) | T0 es nuevo; copia el estilo de `deploy-preflight.sh` |
| Migrar en el pipeline | `scripts/migrate-cloud.sh` | reusado tal cual desde Cloud Build |
| Reintentos de red | `deploy/cloudbuild/retry.sh` | reusado por los pasos nuevos |
| Desplegar desde Cloud Build | paso `deploy` con `_DEPLOY=true` ya escrito | T7 lo activa, no lo construye |
| Permisos para T7 | `main.tf:183` ya los otorga | **cero cambios de Terraform** |

#### Registro de errores y rescates
| Codepath | Qué puede fallar | Clase | ¿Rescatado? | Acción | Se ve como |
| --- | --- | --- | --- | --- | --- |
| `gcloud builds submit --async` | credencial OIDC vencida | AuthError | S | falla el paso | X roja en el run |
| sondeo `builds describe` | build FAILURE | BuildFailed | S | exit 1 | X roja + BUILD_ID en el log |
| sondeo `builds describe` | timeout del sondeo | PollTimeout | S | exit 1 con el BUILD_ID | X roja, build sigue vivo |
| `migrate-cloud.sh` (en Cloud Build) | migración rota | MigrationError | S | build para, **no despliega** | dev sigue en la versión anterior |
| proxy Cloud SQL | nunca levanta | ProxyTimeout | S | ya hay mensaje explícito | `::error::` con frase |
| restore de caché | entrada desalojada | CacheMiss | S | compila en frío | run más lento, nunca rojo |
| guardado de caché | pasa de 10 GB | CacheEvict | **N ← GAP** | — | degradación silenciosa |
| script de shards (Etapa 2) | módulo nuevo sin shard | (test) | S | test de tabla falla | rojo en CI |

#### Registro de modos de falla
| Codepath | Modo de falla | ¿Rescatado? | ¿Test? | ¿Se ve? | ¿Log? |
| --- | --- | --- | --- | --- | --- |
| deploy async sin sondeo | build viejo aterriza al final | S (por el sondeo) | verificación 6 | X roja | S |
| alerta por correo de Cloud Build | no llega (spam/SMTP) | S (se descarta el diseño) | — | — | — |
| caché de Go | desalojo por LRU | **N ← GAP** | medición T1 | no | no |
| T8 (rechazado) | despliegue sin verificar | S (se borró T8) | — | — | — |

**GAP crítico restante: 1** — el desalojo de caché degrada en silencio. Mitigación
aceptada: el paso de medición de T1 y la tasa de `ci-minutes.sh` lo delatan como
una regresión de minutos, no como un fallo. No bloquea.

#### Delta contra el estado ideal
La Etapa 1 deja el pipeline en ~890 min/ciclo, ~8 min por run de CI y ~2 por
deploy. Lo que falta para el ideal a 12 meses: visibilidad continua. T0 es un
script que alguien tiene que acordarse de correr; el ideal tiene una alerta.

<!-- autoplan-accepted:ceo -->
- Confirmar el límite de gasto de Actions ($0 o no) ANTES de empezar; el plan entero depende de esa premisa. Verificación: captura de Settings → Billing → Spending limits.
- Nombrar el costo en dólares del excedente (~$20 USD/mes al doble de consumo) en `## Problema`.
- T8 se elimina del plan. Verificación: `git rev-list --parents -n1` da un padre en los merges a dev (squash) y `check-runs` sobre 8534a22b muestra que su CI verde viene del run de push.
- T1 se rediseña: llave por shard sin `github.run_id`, `GOMODCACHE` en una entrada compartida, guardado solo en push. Precedido por un run instrumentado que mida el tamaño real. Verificación: la medición se publica antes de fijar el número de shards.
- T2 conserva `govulncheck` e integración en la ruta del PR. Verificación: son los únicos pasos de Go rojos en 58 runs (35394352750, 35395204982).
- T7 sondea el estado terminal del build en vez de enviar y salir. Verificación: romper un build a propósito y confirmar que el run de GitHub queda en rojo.
- T7 documenta que no requiere Terraform (main.tf:183) y que un workflow_run corre la copia de master.
- Nuevo T10: quitar de cloudbuild.yaml las compuertas que CI ya corrió, para que T7 quepa en los 120 min/día gratuitos.
- T5 absorbe `actionlint` (no estaba instalado) y pinea `@redocly/cli`.
- T4 lee los archivos cambiados por `env:`, nunca por `${{ }}` dentro de un `run:`.
- T3/T4/T6 pasan a Etapa 2 (después del 30 nov). T3 lleva un test que falle si un módulo de go.work no está en ningún shard.
- El criterio de aceptación pasa a ser una tasa (≤8 min/run de CI, ≤2 min/deploy) con meta escalonada 900/700.
- Los motivos de rechazo de runners auto-hospedados, comprar minutos y repo público quedan escritos explícitamente.
<!-- /autoplan-accepted:ceo -->

<!-- autoplan-baseline-edits:dx {"sourceSha256":"196b4554947ef1caadd53195c591e4a79249d88327708ee97c1127798f5558ae","replacements":[{"oldText":"# PLAN — Bajar el consumo de minutos de GitHub Actions\n\nEstado: REVISADO (CEO) — entra a DX y Eng\nRama: `feature/optimizacion-minutos-ci`\nFecha: 2026-09-18\n\n## Problema\n\nGitHub reporta **2,700 min usados de 3,000 incluidos (90%)** en el ciclo actual.\n\n**Cuánto dinero es, dicho explícitamente:** el excedente en repo privado cuesta\n**$0.008 USD/min**. Hoy el excedente es cero. Aun duplicando el consumo a 5,500\nmin/ciclo, la factura sería **~$20 USD/mes (~370 MXN)**. Así que este plan NO se\njustifica por dinero.\n\nSe justifica por una sola premisa, que hay que confirmar antes de empezar:\n**el límite de gasto de Actions está en $0**, igual que el corte duro de\nfacturación que el dueño eligió para GCP. Con el límite en $0, agotar la cuota\napaga CI y apaga el deploy — con demos **diarias** para el colegio y entrega el\n30 nov 2026, eso es quedarse sin poder desplegar a media semana. Con un límite\ndistinto de $0, el problema real no es la cuota sino el tiempo de pared, y\nsiete de las diez tareas de aquí sobran.\n\n> **Bloqueante:** confirmar el límite en\n> Settings → Billing → Spending limits → Actions. No pude verificarlo: el\n> endpoint de facturación pide el scope `user`, que esta sesión no tiene.\n\n## Medición (no estimación)\n\nMedido con la API de Actions, 94 runs entre 2026-08-19 y 2026-09-18, sumando\n`ceil(duración_del_job / 60s)` por job — que es exactamente como factura GitHub.\n(El endpoint `/timing` devuelve `total_ms: 0` en este repo y no sirve.)\n\n**Total medido: 2,739 min** — coincide con los 2,700 de la factura (1.4% de error).\n\n| Bucket | Minutos | % | Runs |\n| --- | ---: | ---: | ---: |\n| `Deploy dev` → job `build images and roll out` | 789 | 29% | 35 |\n| `CI` → matriz `go` (14 módulos de `go.work`) | 1,619 | 59% | 50 |\n| `CI` → job `web` | 150 | 5.5% | 58 |\n| `CI` → `proto` + `openapi` + `ci` (3 jobs triviales) | 173 | 6.3% | 58 c/u |\n\n| Evento | Minutos | Runs | Promedio |\n| --- | ---: | ---: | ---: |\n| `CI` / pull_request | 1,015 | 21 | **48.3 min/run** |\n| `CI` / push a `dev` | 935 | 37 | **25.3 min/run** |\n| `Deploy dev` / workflow_run | 708 | 25 | 28.3 (58 min los exitosos) |\n| `Deploy dev` / dispatch + push | 81 | 11 | — |\n\n**Advertencia sobre la línea base:** estos 2,739 min se midieron en un mes en el\nque hubo commits retenidos sin empujar justamente para ahorrar minutos. Comparar\nun total contra un mes sin esa restricción mide dos cargas de trabajo distintas.\nPor eso el criterio de aceptación es una **tasa**, no un total (ver §Verificación).\n\n## Causas raíz (verificadas en logs, no inferidas)\n\n### C1 — El caché de Go es una lápida de 7.6 KB\n`actions/setup-go@v6` cachea `GOCACHE` + `GOMODCACHE` con una llave derivada de\n`hashFiles('**/go.sum')`. Las 14 patas de la matriz calculan **la misma llave**.\nLas entradas de caché de Actions son inmutables: la primera pata que ganó la\ncarrera guardó un caché casi vacío y quedó congelado ahí para siempre.\n\nLog del run 35396038508, pata `go (./services/iam)`:\n\n```\nCache hit for: setup-go-Linux-x64-ubuntu24-go-1.25.14-f4c882bf...\nCache Size: ~0 MB (7599 B)\nCache restored successfully\ngo: downloading cloud.google.com/go/firestore v1.22.0\n...\nCache hit occurred on the primary key ..., not saving cache.\n```\n\nReporta \"cache hit\", restaura 7.6 KB, y acto seguido **descarga todos los\nmódulos y compila el grafo completo desde cero** — en cada pata, en cada run,\ndesde que existe el workflow. Como la llave ya existe, nunca vuelve a guardar:\nel caché no se puede auto-reparar.\n\nCosto por pata en ese run: `go vet` 109 s + `go test -race` 112 s +\n`integration` 43 s + `govulncheck` 27 s = **307 s**, casi todo compilación fría.\n\n### C2 — El mismo código se compila cuatro veces por pata\n`go vet ./...` compila el grafo entero (109 s). `go test -race` lo recompila con\ninstrumentación (112 s). Integración lo compila otra vez con `-tags`.\n`govulncheck` carga el grafo una cuarta vez. `go test` **ya corre el\nsubconjunto de vet de alta confianza**.\n\n**Evidencia de que `go vet` no compra nada:** en 58 runs de CI, `go vet` y\n`go test -race` fallaron **cero** veces. Los únicos pasos de Go que se pusieron\nen rojo fueron `govulncheck` (9 patas, run 35394352750) y\n`go test -tags=integration` (students, runs 35394352750 y 35395204982). Esos dos\nse quedan en la ruta del PR: son todo el valor demostrado de la matriz.\n\n### C3 — Se paga un runner de GitHub por mirar un build de Google\nEl job `build images and roll out` dura **~3,500 s (58 min) en los deploys\nexitosos**. Hace `gcloud builds submit`, que **transmite logs de forma síncrona**\nmientras Cloud Build construye las diez imágenes en infraestructura de Google.\nEl runner no compila nada: espera, y factura mientras espera. ~10 deploys\nexitosos × 58 min = **~580 min** de cuota gastados en un `tail -f`.\n\n### C4 — Impuesto de redondeo por job\nGitHub factura cada job redondeado al minuto completo:\n\n- `proto`: 17 s de trabajo → 1 min × 58 runs = **58 min**\n- `openapi (lint)`: 9 s (3 de ellos bajando `@redocly/cli@latest`) → **57 min**\n- `ci` (un `if` de cinco líneas) → **58 min**\n- matriz `go`: 153 s reales → 3 min facturados en PR; 81 s → 2 min en push.\n  Solo el redondeo, sobre 700 jobs de matriz, son **~400 min**.\n\n### C5 — La matriz no mira el diff\nCambiar un archivo en `apps/web` corre las 14 patas de Go. `paths-ignore` solo\nexcluye markdown, `docs/` y `.claude/`.\n\n### C6 — Los dos pipelines corren la MISMA suite de compuertas\n`cloudbuild.yaml` vuelve a correr, sobre el mismo commit que CI acaba de\naprobar: `buf lint`, `go vet` (todos los módulos), `go test -race` (todos los\nmódulos), `govulncheck`, `web build` y la compuerta de source maps. Eso es la\nmayor parte de los 58 min de build. CI es la compuerta; Cloud Build debería\nconstruir imágenes y desplegar, nada más.\n\n## Objetivo\n\nQue un run de CI cueste **≤ 8 min facturados** y un deploy **≤ 2 min\nfacturados**, sin quitar ninguna compuerta que haya atrapado algo real.\n\n## Plan de implementación\n\nDos etapas. La Etapa 1 lleva ~94% del ahorro con el riesgo bajo; la Etapa 2\nlleva ~6% y concentra todo el riesgo, así que va después del 30 nov.\n\n### ETAPA 1 — antes del 30 nov\n\n#### T0 — Línea base medible antes de tocar nada\n`scripts/ci-minutes.sh`, en el estilo de `scripts/deploy-preflight.sh`\n(`/bin/sh`, solo lectura, helpers `ok/bad/warn`, exit 1): consulta la API de\nActions, suma `ceil(job_duration/60)` y reporta **min por run de CI** y **min\npor deploy**, no solo el total. Es la prueba que falla primero: hoy imprime\n~48 min/run de PR y ~58 min/deploy.\n\n#### T1 — Reparar el caché de Go (la palanca más grande de CI)\nEl diseño con `github.run_id` en la llave **no sirve**: medido en esta máquina,\n`GOMODCACHE` son **6.0 GB** y `GOCACHE` **1.0 GB**, contra un presupuesto LRU de\n**10 GB por repo**. Rotar la llave cada run llenaría y desalojaría el caché en\nmenos de un día, y los PR restaurarían basura o nada. La lápida se arregla\nhaciendo la llave **por shard**, no rotándola.\n\n- Paso 1: **medir**. Un run instrumentado que imprima el tamaño real de\n  `GOCACHE` y `GOMODCACHE` por shard en Linux. El número decide el resto.\n- `cache: false` en `actions/setup-go` (deja de emitir la llave compartida).\n- **`GOMODCACHE`**: UNA entrada compartida, llave\n  `gomod-${{ hashFiles('**/go.sum') }}`, sin rotación (cambia cuando cambian las\n  dependencias, que es lo correcto). La guarda un job dedicado.\n- **`GOCACHE`**: una entrada **por shard**, llave\n  `gocache-<shard>-${{ hashFiles('**/go.sum') }}-${{ github.sha }}` con\n  `restore-keys` que caen al prefijo del shard.\n- **Guardar solo en push a `dev`** (`if: github.event_name == 'push'`). Los PR\n  restauran y no escriben: así no se desaloja nada y se cierra de paso el vector\n  de envenenamiento de caché desde una rama.\n- Si la medición dice que no cabe, bajar a 2 shards antes que sacrificar el caché.\n\n#### T2 — Dejar de compilar lo mismo cuatro veces\n- Borrar el paso `go vet` por módulo (cero hallazgos en 58 runs, 109 s × 14 × 50).\n- `go vet ./...` **una vez** sobre todo el workspace, en un job con caché caliente.\n- `govulncheck`: instalar el binario **una vez** y compartirlo por caché, en vez\n  de `go install` en cada pata. El paso se queda en la ruta del PR: es uno de los\n  dos únicos que han atrapado algo.\n\n#### T5 — Fundir `openapi (lint)` y `actionlint` dentro del job `proto`\nMismo checkout, mismo runner, cero minutos extra. Pinear `@redocly/cli` a una\nversión en vez de `@latest` — mismo error que tumbó el deploy el 2026-09-10 con\n`govulncheck@latest`. `actionlint` entra aquí (el job ya tiene Go): la\nverificación del plan lo daba por instalado y no lo está.\nNota: `openapi (lint)` es el job que **más veces se ha puesto en rojo** (9 de 58\nruns), así que fundirlo no lo debilita — lo abarata.\n\n#### T7 — Enviar el build y esperar el veredicto, no los logs\n- `gcloud builds submit --async`, capturar el BUILD_ID, y **sondear el estado\n  terminal** con `gcloud builds describe` cada ~30 s. Costo: **~2-3 min\n  facturados** en vez de 58.\n- **No es fire-and-forget, y eso es deliberado.** Un envío sin espera rompe en\n  silencio `concurrency: deploy-dev, cancel-in-progress`: hoy ese candado\n  funciona solo porque el runner sostiene el build entero. Sin la espera, dos\n  pushes seguidos lanzan dos builds que se carreran a\n  `gcloud run services update` y **el más viejo puede aterrizar al final**. Esa\n  es exactamente la falla de \"la demo del 12 de noviembre corrió el commit\n  equivocado dos días\". El sondeo conserva el candado y conserva la X roja.\n- La X roja importa el doble aquí: la mitigación que este plan proponía en su\n  primera versión (alerta por correo) **está rota y documentado que lo está** —\n  el correo saliente de este proyecto cae en spam y quedó bloqueado en una\n  credencial SMTP.\n- Migraciones, rollout y smoke test se mueven a pasos de `cloudbuild.yaml` con\n  `_DEPLOY=true`, conservando el orden ya probado: **esquema antes que código**;\n  si la migración falla no se despliega nada.\n- **No requiere cambios de Terraform.** `deploy/terraform/main.tf:183` ya le da\n  al SA de build `roles/run.admin`, `roles/cloudsql.client`,\n  `roles/iam.serviceAccountUser` y acceso al secreto\n  `garcia-lorca-db-migrator-password-dev`, con un comentario que dice\n  literalmente que es para que el deploy corra `scripts/migrate-cloud.sh` antes\n  del rollout. La infraestructura lleva ahí desde el incidente del 2026-09-07.\n- **Restricción de secuencia (bloqueante para probarlo):** un workflow\n  `workflow_run` siempre ejecuta la copia de la **rama por defecto**, que aquí\n  es `master`. Editar `deploy-dev.yml` en una rama **no tiene efecto** hasta que\n  llega a `master`. T7 va al final de la etapa y no se puede validar en rama.\n\n#### T9 — Higiene de disparadores\n- `paths-ignore` gana `apps/mobile/**` para los jobs de Go (el móvil no toca Go).\n- `concurrency` de CI ya cancela runs superados; la llave `github.ref` es correcta.\n\n#### T10 — Quitar de Cloud Build las compuertas que CI ya corrió (C6)\nBorrar `go-vet`, `go-test`, `govulncheck`, `web-build` y `sourcemap-gate` de\n`cloudbuild.yaml`; dejar `buf generate`, las diez imágenes, migraciones, rollout\ny smoke. Esto es lo que hace que T7 **quepa** en los 120 min/día gratuitos de\ne2-medium en vez de pasarse: sin esto, añadir migraciones y rollout a un build\nque ya dura 58 min lo empuja arriba de 60, y dos deploys al día rozan el tope.\nEl tope de GCP es un corte duro que tira el ambiente de la demo; el de GitHub\nsolo frena CI.\n\n### ETAPA 2 — después del 30 nov (no antes)\n\n#### T3 — Colapsar la matriz de 14 patas a 4 shards\nAgrupar los módulos por tiempo medido. Mata diez arranques y diez redondeos por\nrun. **Deuda que introduce:** la agrupación es una lista a mano que se desfasa\ncuando se agrega un servicio; hoy `go.work` es la única fuente de verdad y esto\nla rompe. Mitigación: un test que falle si un módulo de `go.work` no está en\nningún shard.\n\n#### T4 — Correr solo los módulos que el diff puede afectar\nRegla explícita, en un script con tests de tabla, no en YAML:\n- Cambia `pkg/`, `gen/`, `proto/`, `go.work`, `go.work.sum` o el workflow →\n  **todos** los shards.\n- Si no, solo los shards con un `services/<x>` tocado.\n- **La lista de archivos cambiados se lee por `env:`, nunca interpolando\n  `${{ }}` dentro de un `run:`** — títulos de PR y nombres de rama son texto que\n  un atacante controla, y `${{ }}` los mete como código shell.\n\n#### T6 — Eliminar el job agregador `ci`\nCon T3 los nombres son fijos y se registran como required checks directamente.\n\n### RECHAZADO — T8 (no re-probar el push a `dev`)\n\n**La premisa era falsa y la verifiqué.** El plan decía que el push a `dev`\nre-verifica lo mismo que el PR. No:\n\n1. `ci.yml:176-205` deja `-race`, integración y `govulncheck` en\n   `if: github.event_name == 'pull_request'`. El push corre una compuerta\n   **distinta y más débil**, no la misma.\n2. Los merges a `dev` son **squash**: `git rev-list --parents -n1` devuelve un\n   solo padre en `8534a22b`, `d2b40091`, `875978c5` y `bf86d107`. El commit que\n   se despliega es un SHA que **ningún run de PR vio nunca**.\n3. `check-runs` sobre `8534a22b` confirma que su CI verde viene del run de\n   **push** (35397296141) — justamente el que T8 quería borrar.\n\nT8 no eliminaba trabajo redundante: eliminaba **la única verificación que\nexiste sobre el árbol exacto que llega al colegio**. Los 935 min del push se\nabaratan con T1 y T3; no se quitan.\n\n## Ahorro proyectado\n\n| Cambio | De | A | Ahorro |\n| --- | ---: | ---: | ---: |\n| T1+T2 — caché + compilaciones duplicadas | 1,619 | ~500 | ~1,120 |\n| T5 — fundir openapi+actionlint en proto | 57 | 0 | ~57 |\n| T7 — enviar y sondear en vez de mirar | 789 | ~35 | ~754 |\n| **Etapa 1** | **2,739** | **~890** | **~1,930 (70%)** |\n| T3+T4+T6 (Etapa 2) | ~890 | ~700 | ~190 (7%) |\n\n`web` (150 min) y `proto` (58 min) se quedan como están: tras la Etapa 1 pasan a\nser ~23% del presupuesto, y son baratos y útiles. Aceptado explícitamente.\n\n## Verificación\n\nCriterio de aceptación **por tasa**, no por total — el total depende del volumen\nde trabajo del mes, que va a subir en cuanto se pueda empujar sin miedo:\n\n1. **≤ 8 min facturados por run de CI** (hoy: 48 en PR, 25 en push).\n2. **≤ 2 min facturados por deploy** (hoy: 58).\n3. Meta escalonada del total: **≤ 900 min/ciclo éxito, ≤ 700 objetivo**.\n4. `actionlint` verde sobre los dos workflows (dentro del job `proto`).\n5. Tests de tabla para la regla de selección de shards (Etapa 2).\n6. Un deploy real a dev: el commit desplegado es el que se envió, y un build\n   fallado **pone en rojo** el run de GitHub. Esto se prueba a propósito\n   rompiendo un build.\n7. `scripts/ci-minutes.sh` a los 30 días.\n\n## Lo que NO entra en este plan\n\n- **Comprar minutos** (el excedente cuesta ~$20 USD/mes aun al doble del\n  consumo). Rechazado solo si el límite de gasto está deliberadamente en $0; si\n  no lo está, ésta es la opción correcta y este plan se reduce a T1+T2+T7.\n- **Hacer el repo público** (Actions es gratis e ilimitado en repos públicos).\n  Rechazado: es el sistema de gestión de un colegio que se entrega en noviembre;\n  publicarlo publica el modelo de auth, las políticas RLS y el hueco conocido\n  del limitador global de login, además de regalar el entregable del dueño.\n- **Runners auto-hospedados.** El motivo real es que no hay apetito de agregar\n  superficie operativa nueva a 73 días de la entrega — no que \"el stack local se\n  muere\", que es una memoria sobre un binario bajo `air` en esta laptop, no sobre\n  un runner administrado.\n- Cambiar de proveedor de CI.\n- Optimizar los Dockerfile (el build vive en el tramo gratuito de Cloud Build).\n\n## Notas de contexto\n\n- La memoria del proyecto dice que `Deploy dev` está bloqueado desde el 10-sep\n  por `govulncheck@latest`. **Está desactualizada**: `cloudbuild.yaml` ya pinea\n  `_GOVULNCHECK_VERSION: v1.7.0` y hubo un deploy exitoso el 2026-09-14.\n- La memoria dice que hay 47 commits sin empujar. **También desactualizada**:\n  `origin/dev == dev == 8534a22b`. Todo está empujado, y eso explica el 90%.\n\n\n","newText":"# PLAN — Bajar el consumo de minutos de GitHub Actions\n\nEstado: REVISADO (CEO) — entra a DX y Eng\nRama: `feature/optimizacion-minutos-ci`\nFecha: 2026-09-18\n\n## Problema\n\nGitHub reporta **2,700 min usados de 3,000 incluidos (90%)** en el ciclo actual.\n\n**Cuánto dinero es, dicho explícitamente:** el excedente en repo privado cuesta\n**$0.008 USD/min**. Hoy el excedente es cero. Aun duplicando el consumo a 5,500\nmin/ciclo, la factura sería **~$20 USD/mes (~370 MXN)**. Así que este plan NO se\njustifica por dinero.\n\nSe justifica por una sola premisa, que hay que confirmar antes de empezar:\n**el límite de gasto de Actions está en $0**, igual que el corte duro de\nfacturación que el dueño eligió para GCP. Con el límite en $0, agotar la cuota\napaga CI y apaga el deploy — con demos **diarias** para el colegio y entrega el\n30 nov 2026, eso es quedarse sin poder desplegar a media semana. Con un límite\ndistinto de $0, el problema real no es la cuota sino el tiempo de pared, y\nsiete de las diez tareas de aquí sobran.\n\n> **Bloqueante — cómo se resuelve, no solo dónde mirar:**\n> ```sh\n> gh auth refresh -h github.com -s user          # el endpoint pide este scope\n> gh api /users/romodevx/settings/billing/actions\n> ```\n> O en la UI: Settings → Billing → Spending limits → Actions.\n> **Regla de bifurcación:** si el límite es $0 → Etapa 1 completa. Si NO es $0 →\n> solo T1, T2 y T7 (reparación de defectos, no cuestan cobertura); el resto no\n> se justifica a ~$20 USD/mes.\n\n## Medición (no estimación)\n\nMedido con la API de Actions, 94 runs entre 2026-08-19 y 2026-09-18, sumando\n`ceil(duración_del_job / 60s)` por job — que es exactamente como factura GitHub.\n(El endpoint `/timing` devuelve `total_ms: 0` en este repo y no sirve.)\n\n**Total medido: 2,739 min** — coincide con los 2,700 de la factura (1.4% de error).\n\n| Bucket | Minutos | % | Runs |\n| --- | ---: | ---: | ---: |\n| `Deploy dev` → job `build images and roll out` | 789 | 29% | 35 |\n| `CI` → matriz `go` (14 módulos de `go.work`) | 1,619 | 59% | 50 |\n| `CI` → job `web` | 150 | 5.5% | 58 |\n| `CI` → `proto` + `openapi` + `ci` (3 jobs triviales) | 173 | 6.3% | 58 c/u |\n\n| Evento | Minutos | Runs | Promedio |\n| --- | ---: | ---: | ---: |\n| `CI` / pull_request | 1,015 | 21 | **48.3 min/run** |\n| `CI` / push a `dev` | 935 | 37 | **25.3 min/run** |\n| `Deploy dev` / workflow_run | 708 | 25 | 28.3 (58 min los exitosos) |\n| `Deploy dev` / dispatch + push | 81 | 11 | — |\n\n**Advertencia sobre la línea base:** estos 2,739 min se midieron en un mes en el\nque hubo commits retenidos sin empujar justamente para ahorrar minutos. Comparar\nun total contra un mes sin esa restricción mide dos cargas de trabajo distintas.\nPor eso el criterio de aceptación es una **tasa**, no un total (ver §Verificación).\n\n## Causas raíz (verificadas en logs, no inferidas)\n\n### C1 — El caché de Go es una lápida de 7.6 KB\n`actions/setup-go@v6` cachea `GOCACHE` + `GOMODCACHE` con una llave derivada de\n`hashFiles('**/go.sum')`. Las 14 patas de la matriz calculan **la misma llave**.\nLas entradas de caché de Actions son inmutables: la primera pata que ganó la\ncarrera guardó un caché casi vacío y quedó congelado ahí para siempre.\n\nLog del run 35396038508, pata `go (./services/iam)`:\n\n```\nCache hit for: setup-go-Linux-x64-ubuntu24-go-1.25.14-f4c882bf...\nCache Size: ~0 MB (7599 B)\nCache restored successfully\ngo: downloading cloud.google.com/go/firestore v1.22.0\n...\nCache hit occurred on the primary key ..., not saving cache.\n```\n\nReporta \"cache hit\", restaura 7.6 KB, y acto seguido **descarga todos los\nmódulos y compila el grafo completo desde cero** — en cada pata, en cada run,\ndesde que existe el workflow. Como la llave ya existe, nunca vuelve a guardar:\nel caché no se puede auto-reparar.\n\nCosto por pata en ese run: `go vet` 109 s + `go test -race` 112 s +\n`integration` 43 s + `govulncheck` 27 s = **307 s**, casi todo compilación fría.\n\n### C2 — El mismo código se compila cuatro veces por pata\n`go vet ./...` compila el grafo entero (109 s). `go test -race` lo recompila con\ninstrumentación (112 s). Integración lo compila otra vez con `-tags`.\n`govulncheck` carga el grafo una cuarta vez. `go test` **ya corre el\nsubconjunto de vet de alta confianza**.\n\n**Evidencia de que `go vet` no compra nada:** en 58 runs de CI, `go vet` y\n`go test -race` fallaron **cero** veces. Los únicos pasos de Go que se pusieron\nen rojo fueron `govulncheck` (9 patas, run 35394352750) y\n`go test -tags=integration` (students, runs 35394352750 y 35395204982). Esos dos\nse quedan en la ruta del PR: son todo el valor demostrado de la matriz.\n\n### C3 — Se paga un runner de GitHub por mirar un build de Google\nEl job `build images and roll out` dura **~3,500 s (58 min) en los deploys\nexitosos**. Hace `gcloud builds submit`, que **transmite logs de forma síncrona**\nmientras Cloud Build construye las diez imágenes en infraestructura de Google.\nEl runner no compila nada: espera, y factura mientras espera. ~10 deploys\nexitosos × 58 min = **~580 min** de cuota gastados en un `tail -f`.\n\n### C4 — Impuesto de redondeo por job\nGitHub factura cada job redondeado al minuto completo:\n\n- `proto`: 17 s de trabajo → 1 min × 58 runs = **58 min**\n- `openapi (lint)`: 9 s (3 de ellos bajando `@redocly/cli@latest`) → **57 min**\n- `ci` (un `if` de cinco líneas) → **58 min**\n- matriz `go`: 153 s reales → 3 min facturados en PR; 81 s → 2 min en push.\n  Solo el redondeo, sobre 700 jobs de matriz, son **~400 min**.\n\n### C5 — La matriz no mira el diff\nCambiar un archivo en `apps/web` corre las 14 patas de Go. `paths-ignore` solo\nexcluye markdown, `docs/` y `.claude/`.\n\n### C6 — Los dos pipelines corren la MISMA suite de compuertas\n`cloudbuild.yaml` vuelve a correr, sobre el mismo commit que CI acaba de\naprobar: `buf lint`, `go vet` (todos los módulos), `go test -race` (todos los\nmódulos), `govulncheck`, `web build` y la compuerta de source maps. Eso es la\nmayor parte de los 58 min de build. CI es la compuerta; Cloud Build debería\nconstruir imágenes y desplegar, nada más.\n\n## Objetivo\n\nQue un run de CI cueste **≤ 8 min facturados** y un deploy **≤ 2 min\nfacturados**, sin quitar ninguna compuerta que haya atrapado algo real.\n\n## Plan de implementación\n\nDos etapas. La Etapa 1 lleva ~94% del ahorro con el riesgo bajo; la Etapa 2\nlleva ~6% y concentra todo el riesgo, así que va después del 30 nov.\n\n### ETAPA 1 — antes del 30 nov\n\n#### T0 — Línea base medible antes de tocar nada\n`scripts/ci-minutes.sh`, en el estilo de `scripts/deploy-preflight.sh`\n(`/bin/sh`, solo lectura, helpers `ok/bad/warn`, exit 1): consulta la API de\nActions, suma `ceil(job_duration/60)` y reporta **min por run de CI** y **min\npor deploy**, no solo el total. Es la prueba que falla primero: hoy imprime\n~48 min/run de PR y ~58 min/deploy.\n\n#### T1 — Reparar el caché de Go (la palanca más grande de CI)\nEl diseño con `github.run_id` en la llave **no sirve**: medido en esta máquina,\n`GOMODCACHE` son **6.0 GB** y `GOCACHE` **1.0 GB**, contra un presupuesto LRU de\n**10 GB por repo**. Rotar la llave cada run llenaría y desalojaría el caché en\nmenos de un día, y los PR restaurarían basura o nada. La lápida se arregla\nhaciendo la llave **por shard**, no rotándola.\n\n- Paso 1: **medir**. Un run instrumentado que imprima el tamaño real de\n  `GOCACHE` y `GOMODCACHE` **por módulo** en Linux. El número decide el resto.\n- `cache: false` en `actions/setup-go` (deja de emitir la llave compartida).\n- **`GOMODCACHE`**: UNA entrada compartida, llave\n  `gomod-${{ hashFiles('**/go.sum') }}`, sin rotación (cambia cuando cambian las\n  dependencias, que es lo correcto). La guarda un job dedicado.\n- **`GOCACHE`**: una entrada **por módulo de la matriz**, llave\n  `gocache-${{ matrix.module }}-${{ hashFiles('**/go.sum') }}-${{ github.sha }}`\n  con `restore-keys` que caen al prefijo del módulo.\n  **En la Etapa 1 la unidad es `matrix.module` (14 patas), no \"shard\":** los\n  shards no existen hasta T3, que es Etapa 2. Escribir la llave en términos de\n  shards deja la tarea sin poder implementarse.\n- **Guardar solo en push a `dev`** (`if: github.event_name == 'push'`). Los PR\n  restauran y no escriben: así no se desaloja nada y se cierra de paso el vector\n  de envenenamiento de caché desde una rama.\n  **Consecuencia que va escrita en el comentario del workflow:** una rama que\n  toca `go.sum` compila en frío en cada run de PR hasta que se mergea. Sin ese\n  comentario, el siguiente que lo vea concluye \"el caché volvió a romperse\".\n- **Si no cabe en 10 GB, el plan B es de Etapa 1, no T3:** cachear `GOCACHE`\n  solo en los módulos caros (iam, students, finance, comms, uniforms, grades,\n  attendance) y dejar que los baratos compilen en frío. Bajar el número de\n  shards NO es una salida disponible antes del 30 nov.\n- **Observabilidad del caché (si no, esto se repite).** La lápida sobrevivió\n  meses porque un caché vacío y uno lleno se ven igual en el log. Paso posterior\n  al restore que imprima el tamaño restaurado y avise — y falle en push a\n  `dev` — si está por debajo de un umbral.\n\n#### T2 — Dejar de compilar lo mismo cuatro veces\n- Borrar el paso `go vet` por módulo (cero hallazgos en 58 runs, 109 s × 14 × 50).\n- `go vet ./...` **una vez** sobre todo el workspace, en un job con caché caliente.\n- `govulncheck`: instalar el binario **una vez** y compartirlo por caché, en vez\n  de `go install` en cada pata. El paso se queda en la ruta del PR: es uno de los\n  dos únicos que han atrapado algo.\n\n#### T5 — Fundir `openapi (lint)` y `actionlint` dentro del job `proto`\nMismo checkout, mismo runner, cero minutos extra. Pinear `@redocly/cli` a una\nversión en vez de `@latest` — mismo error que tumbó el deploy el 2026-09-10 con\n`govulncheck@latest`. `actionlint` entra aquí (el job ya tiene Go): la\nverificación del plan lo daba por instalado y no lo está.\n**`actionlint` se pinea igual**, con su variable `ACTIONLINT_VERSION`. Meterlo\ncon `@latest` en el mismo párrafo que pinea redocly sería repetir el error que\nse está corrigiendo.\nNota: `openapi (lint)` es el job que **más veces se ha puesto en rojo** (9 de 58\nruns), así que fundirlo no lo debilita — lo abarata.\n\n#### T7 — Enviar el build y esperar el veredicto, no los logs\n- `gcloud builds submit --async`, capturar el BUILD_ID, y **sondear el estado\n  terminal** con `gcloud builds describe` cada ~30 s. Costo: **~2-3 min\n  facturados** en vez de 58.\n- **El sondeo lleva fecha límite.** Sin tope, un build colgado factura el job de\n  GitHub hasta el límite de 6 h — justo la falla que este plan existe para\n  evitar. Deadline explícito (p. ej. 75 min) que falla el job con el BUILD_ID.\n- **Y lleva trampa de cancelación.** `cancel-in-progress` cancela el *runner*,\n  no el build ya enviado: el build viejo sigue vivo y llega igual a\n  `gcloud run services update`. Un `trap` que corra\n  `gcloud builds cancel $BUILD_ID` antes de salir es lo que de verdad cierra la\n  carrera; el sondeo solo no basta.\n- **Qué imprime cuando falla.** Hoy el runner transmite los logs, así que un\n  build roto se diagnostica en la misma pantalla. Cambiarlo por un estado y un\n  BUILD_ID es una **regresión** de diagnóstico. En cualquier estado terminal\n  distinto de SUCCESS, el paso escribe al resumen del job: el paso que falló,\n  las últimas ~100 líneas (`gcloud builds log $BUILD_ID`) y la URL de consola.\n- **No es fire-and-forget, y eso es deliberado.** Un envío sin espera rompe en\n  silencio `concurrency: deploy-dev, cancel-in-progress`: hoy ese candado\n  funciona solo porque el runner sostiene el build entero. Sin la espera, dos\n  pushes seguidos lanzan dos builds que se carreran a\n  `gcloud run services update` y **el más viejo puede aterrizar al final**. Esa\n  es exactamente la falla de \"la demo del 12 de noviembre corrió el commit\n  equivocado dos días\". El sondeo conserva el candado y conserva la X roja.\n- La X roja importa el doble aquí: la mitigación que este plan proponía en su\n  primera versión (alerta por correo) **está rota y documentado que lo está** —\n  el correo saliente de este proyecto cae en spam y quedó bloqueado en una\n  credencial SMTP.\n- Migraciones, rollout y smoke test se mueven a pasos de `cloudbuild.yaml` con\n  `_DEPLOY=true`, conservando el orden ya probado: **esquema antes que código**;\n  si la migración falla no se despliega nada.\n- **No requiere cambios de Terraform.** `deploy/terraform/main.tf:183` ya le da\n  al SA de build `roles/run.admin`, `roles/cloudsql.client`,\n  `roles/iam.serviceAccountUser` y acceso al secreto\n  `garcia-lorca-db-migrator-password-dev`, con un comentario que dice\n  literalmente que es para que el deploy corra `scripts/migrate-cloud.sh` antes\n  del rollout. La infraestructura lleva ahí desde el incidente del 2026-09-07.\n- **Restricción de secuencia (bloqueante para probarlo):** un workflow\n  `workflow_run` siempre ejecuta la copia de la **rama por defecto**, que aquí\n  es `master`. Editar `deploy-dev.yml` en una rama **no tiene efecto** hasta que\n  llega a `master`. T7 va al final de la etapa y no se puede validar en rama.\n\n#### T9 — Higiene de disparadores\n- `paths-ignore` gana `apps/mobile/**` para los jobs de Go (el móvil no toca Go).\n- `concurrency` de CI ya cancela runs superados; la llave `github.ref` es correcta.\n\n#### T10 — Quitar de Cloud Build las compuertas que CI ya corrió (C6)\nBorrar `go-vet`, `go-test`, `govulncheck`, `web-build` y `sourcemap-gate` de\n`cloudbuild.yaml`; dejar `buf generate`, las diez imágenes, migraciones, rollout\ny smoke. Esto es lo que hace que T7 **quepa** en los 120 min/día gratuitos de\ne2-medium en vez de pasarse: sin esto, añadir migraciones y rollout a un build\nque ya dura 58 min lo empuja arriba de 60, y dos deploys al día rozan el tope.\nEl tope de GCP es un corte duro que tira el ambiente de la demo; el de GitHub\nsolo frena CI.\n\n**Guardia obligatoria que viene con T10.** Quitar las compuertas de Cloud Build\nsolo es correcto mientras TODO deploy venga después de un CI verde. Hoy no lo\nes: `deploy-dev.yml:60` deja pasar `workflow_dispatch` a propósito, y un\n`gcloud builds submit` a mano también. Sin guardia, un deploy manual publicaría\ncódigo sin `go test`, sin `govulncheck` y **sin la compuerta de source maps**,\nque es regla del proyecto. Antes de enviar el build, el job verifica que el SHA\nexacto tenga un check-run de CI en verde y falla nombrando el que falta;\nsaltarla exige una bandera explícita, no es el camino por defecto.\n\n#### T11 — `make ci`: la misma compuerta, en la laptop, antes de empujar\nLa causa de raíz de 21 runs de PR a 48 min es que los fallos se descubren\n**después** de empujar. El `Makefile` ya tiene `generate`, `lint`, `test` y\n`build` con sus cadenas de ayuda `##`; falta el objetivo que los componga en\n\"lo que corre CI\", más `actionlint`. No es un script nuevo: es un target que\nreusa los que ya existen. `make ci-minutes` entra al lado de `deploy-preflight`,\ncon el mismo patrón. Objetivo: verde en local en menos de 5 min.\n\n#### T12 — Actualizar la documentación del pipeline\nCuatro cambios a \"qué hace CI\" (desaparece `go vet`, el deploy deja de\ntransmitir logs, Cloud Build deja de ser compuerta, aparece un script nuevo) y\ncero tareas de documentación. `deploy/README.md` describe el pipeline actual y\nquedaría mintiendo. Se actualiza: qué corre dónde, por qué `go vet` ya no está,\ncómo se lee un deploy fallido, y cómo se corre `make ci` / `ci-minutes.sh`.\nDocumentación vieja aquí cuesta runs desperdiciados y hace que alguien vuelva a\nproponer T8 dentro de tres meses.\n\n### ETAPA 2 — después del 30 nov (no antes)\n\n#### T3 — Colapsar la matriz de 14 patas a 4 shards\nAgrupar los módulos por tiempo medido. Mata diez arranques y diez redondeos por\nrun. **Deuda que introduce:** la agrupación es una lista a mano que se desfasa\ncuando se agrega un servicio; hoy `go.work` es la única fuente de verdad y esto\nla rompe. Mitigación: un test que falle si un módulo de `go.work` no está en\nningún shard.\n\n#### T4 — Correr solo los módulos que el diff puede afectar\nRegla explícita, en un script con tests de tabla, no en YAML:\n- Cambia `pkg/`, `gen/`, `proto/`, `go.work`, `go.work.sum` o el workflow →\n  **todos** los shards.\n- Si no, solo los shards con un `services/<x>` tocado.\n- **La lista de archivos cambiados se lee por `env:`, nunca interpolando\n  `${{ }}` dentro de un `run:`** — títulos de PR y nombres de rama son texto que\n  un atacante controla, y `${{ }}` los mete como código shell.\n- **Escotilla de escape, documentada junto a la regla:** un token `[ci full]` en\n  el mensaje del commit (o una entrada de `workflow_dispatch`) fuerza las 14\n  patas. Sin ella, cuando la heurística se equivoca la gente empuja commits\n  vacíos para vencer el filtro, que es peor que no tener filtro.\n\n#### T6 — Eliminar el job agregador `ci`\nCon T3 los nombres son fijos y se registran como required checks directamente.\n\n### RECHAZADO — T8 (no re-probar el push a `dev`)\n\n**La premisa era falsa y la verifiqué.** El plan decía que el push a `dev`\nre-verifica lo mismo que el PR. No:\n\n1. `ci.yml:176-205` deja `-race`, integración y `govulncheck` en\n   `if: github.event_name == 'pull_request'`. El push corre una compuerta\n   **distinta y más débil**, no la misma.\n2. Los merges a `dev` son **squash**: `git rev-list --parents -n1` devuelve un\n   solo padre en `8534a22b`, `d2b40091`, `875978c5` y `bf86d107`. El commit que\n   se despliega es un SHA que **ningún run de PR vio nunca**.\n3. `check-runs` sobre `8534a22b` confirma que su CI verde viene del run de\n   **push** (35397296141) — justamente el que T8 quería borrar.\n\nT8 no eliminaba trabajo redundante: eliminaba **la única verificación que\nexiste sobre el árbol exacto que llega al colegio**. Los 935 min del push se\nabaratan con T1 y T3; no se quitan.\n\n## Ahorro proyectado\n\n| Cambio | De | A | Ahorro |\n| --- | ---: | ---: | ---: |\n| T1+T2 — caché + compilaciones duplicadas | 1,619 | ~500 | ~1,120 |\n| T5 — fundir openapi+actionlint en proto | 57 | 0 | ~57 |\n| T7 — enviar y sondear en vez de mirar | 789 | ~35 | ~754 |\n| **Etapa 1** | **2,739** | **~890** | **~1,930 (70%)** |\n| T3+T4+T6 (Etapa 2) | ~890 | ~700 | ~190 (7%) |\n\n`web` (150 min) y `proto` (58 min) se quedan como están: tras la Etapa 1 pasan a\nser ~23% del presupuesto, y son baratos y útiles. Aceptado explícitamente.\n\n## Verificación\n\nCriterio de aceptación **por tasa**, no por total — el total depende del volumen\nde trabajo del mes, que va a subir en cuanto se pueda empujar sin miedo:\n\n1. **≤ 8 min facturados por run de CI** (hoy: 48 en PR, 25 en push).\n2. **≤ 2 min facturados por deploy** (hoy: 58).\n3. Meta escalonada del total: **≤ 900 min/ciclo éxito, ≤ 700 objetivo**.\n4. `actionlint` verde sobre los dos workflows (job `proto`), versión pineada.\n4b. `make ci` verde en local en menos de 5 min, en una máquina limpia.\n4c. `scripts/ci-minutes.sh` corre a la primera con UN comando documentado, y\n   ante un 403 imprime el `gh auth refresh -s user` que lo arregla.\n4d. Un deploy manual (`workflow_dispatch`) sobre un SHA sin CI verde **falla**\n   nombrando el check que falta.\n4e. Un run cancelado a media carrera deja el build de Cloud Build cancelado\n   también, no huérfano.\n5. Tests de tabla para la regla de selección de shards (Etapa 2).\n6. Un deploy real a dev: el commit desplegado es el que se envió, y un build\n   fallado **pone en rojo** el run de GitHub. Esto se prueba a propósito\n   rompiendo un build.\n7. `scripts/ci-minutes.sh` a los 30 días.\n8. `deploy/README.md` describe el pipeline que existe después del cambio.\n\n## Lo que NO entra en este plan\n\n- **Comprar minutos** (el excedente cuesta ~$20 USD/mes aun al doble del\n  consumo). Rechazado solo si el límite de gasto está deliberadamente en $0; si\n  no lo está, ésta es la opción correcta y este plan se reduce a T1+T2+T7.\n- **Hacer el repo público** (Actions es gratis e ilimitado en repos públicos).\n  Rechazado: es el sistema de gestión de un colegio que se entrega en noviembre;\n  publicarlo publica el modelo de auth, las políticas RLS y el hueco conocido\n  del limitador global de login, además de regalar el entregable del dueño.\n- **Runners auto-hospedados.** El motivo real es que no hay apetito de agregar\n  superficie operativa nueva a 73 días de la entrega — no que \"el stack local se\n  muere\", que es una memoria sobre un binario bajo `air` en esta laptop, no sobre\n  un runner administrado.\n- Cambiar de proveedor de CI.\n- Optimizar los Dockerfile (el build vive en el tramo gratuito de Cloud Build).\n\n## Notas de contexto\n\n- La memoria del proyecto dice que `Deploy dev` está bloqueado desde el 10-sep\n  por `govulncheck@latest`. **Está desactualizada**: `cloudbuild.yaml` ya pinea\n  `_GOVULNCHECK_VERSION: v1.7.0` y hubo un deploy exitoso el 2026-09-14.\n- La memoria dice que hay 47 commits sin empujar. **También desactualizada**:\n  `origin/dev == dev == 8534a22b`. Todo está empujado, y eso explica el 90%.\n\n\n"}]} -->

### Fase 2.5 — DX (DX POLISH)

Voces: subagente completado (12 hallazgos); Codex **no instalado** → sin
cobertura externa. Consenso N/A en las seis dimensiones.
Puntajes: inicial 4.6/10 → 7.9/10. La voz externa puntuó 4.5/10 por separado.
Persona: dueño solo + **el mantenedor que hereda el repo el 30 nov 2026**.
Tiempo-a-veredicto-confiable: 48 min hoy → objetivo <5 min en local.

<!-- autoplan-accepted:dx -->
- T1 usa `matrix.module` como unidad en la Etapa 1, no "shard": T3 es Etapa 2 y la tarea no se podia implementar como estaba escrita. Verificacion: la llave del cache no menciona shards antes del 30 nov.
- El plan B de T1 si no cabe en 10 GB es cachear solo los modulos caros, no reducir shards. Verificacion: el run de medicion decide y su salida queda en el PR.
- T1 anade un paso que imprime el tamano restaurado del cache y falla en push a `dev` bajo un umbral. Verificacion: un cache vaciado a proposito pone el run en rojo.
- T1 documenta en el comentario del workflow que una rama que toca `go.sum` compila en frio hasta el merge.
- T5 pinea `actionlint` con `ACTIONLINT_VERSION`, igual que `@redocly/cli`.
- T7 lleva deadline de sondeo explicito que falla el job con el BUILD_ID.
- T7 lleva `trap` con `gcloud builds cancel $BUILD_ID`: `cancel-in-progress` cancela el runner, no el build. Verificacion: cancelar un run a media carrera deja el build cancelado, no huerfano.
- T7 imprime, en cualquier estado terminal distinto de SUCCESS, el paso que fallo, las ultimas ~100 lineas del log y la URL de consola. Sin esto el cambio es una regresion de diagnostico.
- T10 viene con guardia: el job verifica un check-run de CI verde sobre el SHA exacto antes de enviar el build, y falla nombrando el que falta. Verificacion: un `workflow_dispatch` sobre un SHA sin CI verde falla.
- T4 (Etapa 2) lleva escotilla `[ci full]` documentada junto a la regla.
- Nuevo T11: `make ci` que compone los targets `generate`/`lint`/`test` que YA existen en el Makefile, mas actionlint; y `make ci-minutes` al lado de `deploy-preflight`. Verificacion: verde en local en menos de 5 min.
- Nuevo T12: actualizar `deploy/README.md` con el pipeline posterior al cambio.
- T0 documenta los scopes de `gh` e imprime el `gh auth refresh -s user` ante un 403.
- El bloqueante del limite de gasto trae comando exacto y regla de bifurcacion: si no es $0, solo T1+T2+T7.
<!-- /autoplan-accepted:dx -->

<!-- autoplan-baseline-edits:eng {"sourceSha256":"f89dfb44252fe53c1ff59254927156343058469c8272f5304d6bb48edfda2c81","replacements":[{"oldText":"# PLAN — Bajar el consumo de minutos de GitHub Actions\n\nEstado: REVISADO (CEO) — entra a DX y Eng\nRama: `feature/optimizacion-minutos-ci`\nFecha: 2026-09-18\n\n## Problema\n\nGitHub reporta **2,700 min usados de 3,000 incluidos (90%)** en el ciclo actual.\n\n**Cuánto dinero es, dicho explícitamente:** el excedente en repo privado cuesta\n**$0.008 USD/min**. Hoy el excedente es cero. Aun duplicando el consumo a 5,500\nmin/ciclo, la factura sería **~$20 USD/mes (~370 MXN)**. Así que este plan NO se\njustifica por dinero.\n\nSe justifica por una sola premisa, que hay que confirmar antes de empezar:\n**el límite de gasto de Actions está en $0**, igual que el corte duro de\nfacturación que el dueño eligió para GCP. Con el límite en $0, agotar la cuota\napaga CI y apaga el deploy — con demos **diarias** para el colegio y entrega el\n30 nov 2026, eso es quedarse sin poder desplegar a media semana. Con un límite\ndistinto de $0, el problema real no es la cuota sino el tiempo de pared, y\nsiete de las diez tareas de aquí sobran.\n\n> **Bloqueante — cómo se resuelve, no solo dónde mirar:**\n> ```sh\n> gh auth refresh -h github.com -s user          # el endpoint pide este scope\n> gh api /users/romodevx/settings/billing/actions\n> ```\n> O en la UI: Settings → Billing → Spending limits → Actions.\n> **Regla de bifurcación:** si el límite es $0 → Etapa 1 completa. Si NO es $0 →\n> solo T1, T2 y T7 (reparación de defectos, no cuestan cobertura); el resto no\n> se justifica a ~$20 USD/mes.\n\n## Medición (no estimación)\n\nMedido con la API de Actions, 94 runs entre 2026-08-19 y 2026-09-18, sumando\n`ceil(duración_del_job / 60s)` por job — que es exactamente como factura GitHub.\n(El endpoint `/timing` devuelve `total_ms: 0` en este repo y no sirve.)\n\n**Total medido: 2,739 min** — coincide con los 2,700 de la factura (1.4% de error).\n\n| Bucket | Minutos | % | Runs |\n| --- | ---: | ---: | ---: |\n| `Deploy dev` → job `build images and roll out` | 789 | 29% | 35 |\n| `CI` → matriz `go` (14 módulos de `go.work`) | 1,619 | 59% | 50 |\n| `CI` → job `web` | 150 | 5.5% | 58 |\n| `CI` → `proto` + `openapi` + `ci` (3 jobs triviales) | 173 | 6.3% | 58 c/u |\n\n| Evento | Minutos | Runs | Promedio |\n| --- | ---: | ---: | ---: |\n| `CI` / pull_request | 1,015 | 21 | **48.3 min/run** |\n| `CI` / push a `dev` | 935 | 37 | **25.3 min/run** |\n| `Deploy dev` / workflow_run | 708 | 25 | 28.3 (58 min los exitosos) |\n| `Deploy dev` / dispatch + push | 81 | 11 | — |\n\n**Advertencia sobre la línea base:** estos 2,739 min se midieron en un mes en el\nque hubo commits retenidos sin empujar justamente para ahorrar minutos. Comparar\nun total contra un mes sin esa restricción mide dos cargas de trabajo distintas.\nPor eso el criterio de aceptación es una **tasa**, no un total (ver §Verificación).\n\n## Causas raíz (verificadas en logs, no inferidas)\n\n### C1 — El caché de Go es una lápida de 7.6 KB\n`actions/setup-go@v6` cachea `GOCACHE` + `GOMODCACHE` con una llave derivada de\n`hashFiles('**/go.sum')`. Las 14 patas de la matriz calculan **la misma llave**.\nLas entradas de caché de Actions son inmutables: la primera pata que ganó la\ncarrera guardó un caché casi vacío y quedó congelado ahí para siempre.\n\nLog del run 35396038508, pata `go (./services/iam)`:\n\n```\nCache hit for: setup-go-Linux-x64-ubuntu24-go-1.25.14-f4c882bf...\nCache Size: ~0 MB (7599 B)\nCache restored successfully\ngo: downloading cloud.google.com/go/firestore v1.22.0\n...\nCache hit occurred on the primary key ..., not saving cache.\n```\n\nReporta \"cache hit\", restaura 7.6 KB, y acto seguido **descarga todos los\nmódulos y compila el grafo completo desde cero** — en cada pata, en cada run,\ndesde que existe el workflow. Como la llave ya existe, nunca vuelve a guardar:\nel caché no se puede auto-reparar.\n\nCosto por pata en ese run: `go vet` 109 s + `go test -race` 112 s +\n`integration` 43 s + `govulncheck` 27 s = **307 s**, casi todo compilación fría.\n\n### C2 — El mismo código se compila cuatro veces por pata\n`go vet ./...` compila el grafo entero (109 s). `go test -race` lo recompila con\ninstrumentación (112 s). Integración lo compila otra vez con `-tags`.\n`govulncheck` carga el grafo una cuarta vez. `go test` **ya corre el\nsubconjunto de vet de alta confianza**.\n\n**Evidencia de que `go vet` no compra nada:** en 58 runs de CI, `go vet` y\n`go test -race` fallaron **cero** veces. Los únicos pasos de Go que se pusieron\nen rojo fueron `govulncheck` (9 patas, run 35394352750) y\n`go test -tags=integration` (students, runs 35394352750 y 35395204982). Esos dos\nse quedan en la ruta del PR: son todo el valor demostrado de la matriz.\n\n### C3 — Se paga un runner de GitHub por mirar un build de Google\nEl job `build images and roll out` dura **~3,500 s (58 min) en los deploys\nexitosos**. Hace `gcloud builds submit`, que **transmite logs de forma síncrona**\nmientras Cloud Build construye las diez imágenes en infraestructura de Google.\nEl runner no compila nada: espera, y factura mientras espera. ~10 deploys\nexitosos × 58 min = **~580 min** de cuota gastados en un `tail -f`.\n\n### C4 — Impuesto de redondeo por job\nGitHub factura cada job redondeado al minuto completo:\n\n- `proto`: 17 s de trabajo → 1 min × 58 runs = **58 min**\n- `openapi (lint)`: 9 s (3 de ellos bajando `@redocly/cli@latest`) → **57 min**\n- `ci` (un `if` de cinco líneas) → **58 min**\n- matriz `go`: 153 s reales → 3 min facturados en PR; 81 s → 2 min en push.\n  Solo el redondeo, sobre 700 jobs de matriz, son **~400 min**.\n\n### C5 — La matriz no mira el diff\nCambiar un archivo en `apps/web` corre las 14 patas de Go. `paths-ignore` solo\nexcluye markdown, `docs/` y `.claude/`.\n\n### C6 — Los dos pipelines corren la MISMA suite de compuertas\n`cloudbuild.yaml` vuelve a correr, sobre el mismo commit que CI acaba de\naprobar: `buf lint`, `go vet` (todos los módulos), `go test -race` (todos los\nmódulos), `govulncheck`, `web build` y la compuerta de source maps. Eso es la\nmayor parte de los 58 min de build. CI es la compuerta; Cloud Build debería\nconstruir imágenes y desplegar, nada más.\n\n## Objetivo\n\nQue un run de CI cueste **≤ 8 min facturados** y un deploy **≤ 2 min\nfacturados**, sin quitar ninguna compuerta que haya atrapado algo real.\n\n## Plan de implementación\n\nDos etapas. La Etapa 1 lleva ~94% del ahorro con el riesgo bajo; la Etapa 2\nlleva ~6% y concentra todo el riesgo, así que va después del 30 nov.\n\n### ETAPA 1 — antes del 30 nov\n\n#### T0 — Línea base medible antes de tocar nada\n`scripts/ci-minutes.sh`, en el estilo de `scripts/deploy-preflight.sh`\n(`/bin/sh`, solo lectura, helpers `ok/bad/warn`, exit 1): consulta la API de\nActions, suma `ceil(job_duration/60)` y reporta **min por run de CI** y **min\npor deploy**, no solo el total. Es la prueba que falla primero: hoy imprime\n~48 min/run de PR y ~58 min/deploy.\n\n#### T1 — Reparar el caché de Go (la palanca más grande de CI)\nEl diseño con `github.run_id` en la llave **no sirve**: medido en esta máquina,\n`GOMODCACHE` son **6.0 GB** y `GOCACHE` **1.0 GB**, contra un presupuesto LRU de\n**10 GB por repo**. Rotar la llave cada run llenaría y desalojaría el caché en\nmenos de un día, y los PR restaurarían basura o nada. La lápida se arregla\nhaciendo la llave **por shard**, no rotándola.\n\n- Paso 1: **medir**. Un run instrumentado que imprima el tamaño real de\n  `GOCACHE` y `GOMODCACHE` **por módulo** en Linux. El número decide el resto.\n- `cache: false` en `actions/setup-go` (deja de emitir la llave compartida).\n- **`GOMODCACHE`**: UNA entrada compartida, llave\n  `gomod-${{ hashFiles('**/go.sum') }}`, sin rotación (cambia cuando cambian las\n  dependencias, que es lo correcto). La guarda un job dedicado.\n- **`GOCACHE`**: una entrada **por módulo de la matriz**, llave\n  `gocache-${{ matrix.module }}-${{ hashFiles('**/go.sum') }}-${{ github.sha }}`\n  con `restore-keys` que caen al prefijo del módulo.\n  **En la Etapa 1 la unidad es `matrix.module` (14 patas), no \"shard\":** los\n  shards no existen hasta T3, que es Etapa 2. Escribir la llave en términos de\n  shards deja la tarea sin poder implementarse.\n- **Guardar solo en push a `dev`** (`if: github.event_name == 'push'`). Los PR\n  restauran y no escriben: así no se desaloja nada y se cierra de paso el vector\n  de envenenamiento de caché desde una rama.\n  **Consecuencia que va escrita en el comentario del workflow:** una rama que\n  toca `go.sum` compila en frío en cada run de PR hasta que se mergea. Sin ese\n  comentario, el siguiente que lo vea concluye \"el caché volvió a romperse\".\n- **Si no cabe en 10 GB, el plan B es de Etapa 1, no T3:** cachear `GOCACHE`\n  solo en los módulos caros (iam, students, finance, comms, uniforms, grades,\n  attendance) y dejar que los baratos compilen en frío. Bajar el número de\n  shards NO es una salida disponible antes del 30 nov.\n- **Observabilidad del caché (si no, esto se repite).** La lápida sobrevivió\n  meses porque un caché vacío y uno lleno se ven igual en el log. Paso posterior\n  al restore que imprima el tamaño restaurado y avise — y falle en push a\n  `dev` — si está por debajo de un umbral.\n\n#### T2 — Dejar de compilar lo mismo cuatro veces\n- Borrar el paso `go vet` por módulo (cero hallazgos en 58 runs, 109 s × 14 × 50).\n- `go vet ./...` **una vez** sobre todo el workspace, en un job con caché caliente.\n- `govulncheck`: instalar el binario **una vez** y compartirlo por caché, en vez\n  de `go install` en cada pata. El paso se queda en la ruta del PR: es uno de los\n  dos únicos que han atrapado algo.\n\n#### T5 — Fundir `openapi (lint)` y `actionlint` dentro del job `proto`\nMismo checkout, mismo runner, cero minutos extra. Pinear `@redocly/cli` a una\nversión en vez de `@latest` — mismo error que tumbó el deploy el 2026-09-10 con\n`govulncheck@latest`. `actionlint` entra aquí (el job ya tiene Go): la\nverificación del plan lo daba por instalado y no lo está.\n**`actionlint` se pinea igual**, con su variable `ACTIONLINT_VERSION`. Meterlo\ncon `@latest` en el mismo párrafo que pinea redocly sería repetir el error que\nse está corrigiendo.\nNota: `openapi (lint)` es el job que **más veces se ha puesto en rojo** (9 de 58\nruns), así que fundirlo no lo debilita — lo abarata.\n\n#### T7 — Enviar el build y esperar el veredicto, no los logs\n- `gcloud builds submit --async`, capturar el BUILD_ID, y **sondear el estado\n  terminal** con `gcloud builds describe` cada ~30 s. Costo: **~2-3 min\n  facturados** en vez de 58.\n- **El sondeo lleva fecha límite.** Sin tope, un build colgado factura el job de\n  GitHub hasta el límite de 6 h — justo la falla que este plan existe para\n  evitar. Deadline explícito (p. ej. 75 min) que falla el job con el BUILD_ID.\n- **Y lleva trampa de cancelación.** `cancel-in-progress` cancela el *runner*,\n  no el build ya enviado: el build viejo sigue vivo y llega igual a\n  `gcloud run services update`. Un `trap` que corra\n  `gcloud builds cancel $BUILD_ID` antes de salir es lo que de verdad cierra la\n  carrera; el sondeo solo no basta.\n- **Qué imprime cuando falla.** Hoy el runner transmite los logs, así que un\n  build roto se diagnostica en la misma pantalla. Cambiarlo por un estado y un\n  BUILD_ID es una **regresión** de diagnóstico. En cualquier estado terminal\n  distinto de SUCCESS, el paso escribe al resumen del job: el paso que falló,\n  las últimas ~100 líneas (`gcloud builds log $BUILD_ID`) y la URL de consola.\n- **No es fire-and-forget, y eso es deliberado.** Un envío sin espera rompe en\n  silencio `concurrency: deploy-dev, cancel-in-progress`: hoy ese candado\n  funciona solo porque el runner sostiene el build entero. Sin la espera, dos\n  pushes seguidos lanzan dos builds que se carreran a\n  `gcloud run services update` y **el más viejo puede aterrizar al final**. Esa\n  es exactamente la falla de \"la demo del 12 de noviembre corrió el commit\n  equivocado dos días\". El sondeo conserva el candado y conserva la X roja.\n- La X roja importa el doble aquí: la mitigación que este plan proponía en su\n  primera versión (alerta por correo) **está rota y documentado que lo está** —\n  el correo saliente de este proyecto cae en spam y quedó bloqueado en una\n  credencial SMTP.\n- Migraciones, rollout y smoke test se mueven a pasos de `cloudbuild.yaml` con\n  `_DEPLOY=true`, conservando el orden ya probado: **esquema antes que código**;\n  si la migración falla no se despliega nada.\n- **No requiere cambios de Terraform.** `deploy/terraform/main.tf:183` ya le da\n  al SA de build `roles/run.admin`, `roles/cloudsql.client`,\n  `roles/iam.serviceAccountUser` y acceso al secreto\n  `garcia-lorca-db-migrator-password-dev`, con un comentario que dice\n  literalmente que es para que el deploy corra `scripts/migrate-cloud.sh` antes\n  del rollout. La infraestructura lleva ahí desde el incidente del 2026-09-07.\n- **Restricción de secuencia (bloqueante para probarlo):** un workflow\n  `workflow_run` siempre ejecuta la copia de la **rama por defecto**, que aquí\n  es `master`. Editar `deploy-dev.yml` en una rama **no tiene efecto** hasta que\n  llega a `master`. T7 va al final de la etapa y no se puede validar en rama.\n\n#### T9 — Higiene de disparadores\n- `paths-ignore` gana `apps/mobile/**` para los jobs de Go (el móvil no toca Go).\n- `concurrency` de CI ya cancela runs superados; la llave `github.ref` es correcta.\n\n#### T10 — Quitar de Cloud Build las compuertas que CI ya corrió (C6)\nBorrar `go-vet`, `go-test`, `govulncheck`, `web-build` y `sourcemap-gate` de\n`cloudbuild.yaml`; dejar `buf generate`, las diez imágenes, migraciones, rollout\ny smoke. Esto es lo que hace que T7 **quepa** en los 120 min/día gratuitos de\ne2-medium en vez de pasarse: sin esto, añadir migraciones y rollout a un build\nque ya dura 58 min lo empuja arriba de 60, y dos deploys al día rozan el tope.\nEl tope de GCP es un corte duro que tira el ambiente de la demo; el de GitHub\nsolo frena CI.\n\n**Guardia obligatoria que viene con T10.** Quitar las compuertas de Cloud Build\nsolo es correcto mientras TODO deploy venga después de un CI verde. Hoy no lo\nes: `deploy-dev.yml:60` deja pasar `workflow_dispatch` a propósito, y un\n`gcloud builds submit` a mano también. Sin guardia, un deploy manual publicaría\ncódigo sin `go test`, sin `govulncheck` y **sin la compuerta de source maps**,\nque es regla del proyecto. Antes de enviar el build, el job verifica que el SHA\nexacto tenga un check-run de CI en verde y falla nombrando el que falta;\nsaltarla exige una bandera explícita, no es el camino por defecto.\n\n#### T11 — `make ci`: la misma compuerta, en la laptop, antes de empujar\nLa causa de raíz de 21 runs de PR a 48 min es que los fallos se descubren\n**después** de empujar. El `Makefile` ya tiene `generate`, `lint`, `test` y\n`build` con sus cadenas de ayuda `##`; falta el objetivo que los componga en\n\"lo que corre CI\", más `actionlint`. No es un script nuevo: es un target que\nreusa los que ya existen. `make ci-minutes` entra al lado de `deploy-preflight`,\ncon el mismo patrón. Objetivo: verde en local en menos de 5 min.\n\n#### T12 — Actualizar la documentación del pipeline\nCuatro cambios a \"qué hace CI\" (desaparece `go vet`, el deploy deja de\ntransmitir logs, Cloud Build deja de ser compuerta, aparece un script nuevo) y\ncero tareas de documentación. `deploy/README.md` describe el pipeline actual y\nquedaría mintiendo. Se actualiza: qué corre dónde, por qué `go vet` ya no está,\ncómo se lee un deploy fallido, y cómo se corre `make ci` / `ci-minutes.sh`.\nDocumentación vieja aquí cuesta runs desperdiciados y hace que alguien vuelva a\nproponer T8 dentro de tres meses.\n\n### ETAPA 2 — después del 30 nov (no antes)\n\n#### T3 — Colapsar la matriz de 14 patas a 4 shards\nAgrupar los módulos por tiempo medido. Mata diez arranques y diez redondeos por\nrun. **Deuda que introduce:** la agrupación es una lista a mano que se desfasa\ncuando se agrega un servicio; hoy `go.work` es la única fuente de verdad y esto\nla rompe. Mitigación: un test que falle si un módulo de `go.work` no está en\nningún shard.\n\n#### T4 — Correr solo los módulos que el diff puede afectar\nRegla explícita, en un script con tests de tabla, no en YAML:\n- Cambia `pkg/`, `gen/`, `proto/`, `go.work`, `go.work.sum` o el workflow →\n  **todos** los shards.\n- Si no, solo los shards con un `services/<x>` tocado.\n- **La lista de archivos cambiados se lee por `env:`, nunca interpolando\n  `${{ }}` dentro de un `run:`** — títulos de PR y nombres de rama son texto que\n  un atacante controla, y `${{ }}` los mete como código shell.\n- **Escotilla de escape, documentada junto a la regla:** un token `[ci full]` en\n  el mensaje del commit (o una entrada de `workflow_dispatch`) fuerza las 14\n  patas. Sin ella, cuando la heurística se equivoca la gente empuja commits\n  vacíos para vencer el filtro, que es peor que no tener filtro.\n\n#### T6 — Eliminar el job agregador `ci`\nCon T3 los nombres son fijos y se registran como required checks directamente.\n\n### RECHAZADO — T8 (no re-probar el push a `dev`)\n\n**La premisa era falsa y la verifiqué.** El plan decía que el push a `dev`\nre-verifica lo mismo que el PR. No:\n\n1. `ci.yml:176-205` deja `-race`, integración y `govulncheck` en\n   `if: github.event_name == 'pull_request'`. El push corre una compuerta\n   **distinta y más débil**, no la misma.\n2. Los merges a `dev` son **squash**: `git rev-list --parents -n1` devuelve un\n   solo padre en `8534a22b`, `d2b40091`, `875978c5` y `bf86d107`. El commit que\n   se despliega es un SHA que **ningún run de PR vio nunca**.\n3. `check-runs` sobre `8534a22b` confirma que su CI verde viene del run de\n   **push** (35397296141) — justamente el que T8 quería borrar.\n\nT8 no eliminaba trabajo redundante: eliminaba **la única verificación que\nexiste sobre el árbol exacto que llega al colegio**. Los 935 min del push se\nabaratan con T1 y T3; no se quitan.\n\n## Ahorro proyectado\n\n| Cambio | De | A | Ahorro |\n| --- | ---: | ---: | ---: |\n| T1+T2 — caché + compilaciones duplicadas | 1,619 | ~500 | ~1,120 |\n| T5 — fundir openapi+actionlint en proto | 57 | 0 | ~57 |\n| T7 — enviar y sondear en vez de mirar | 789 | ~35 | ~754 |\n| **Etapa 1** | **2,739** | **~890** | **~1,930 (70%)** |\n| T3+T4+T6 (Etapa 2) | ~890 | ~700 | ~190 (7%) |\n\n`web` (150 min) y `proto` (58 min) se quedan como están: tras la Etapa 1 pasan a\nser ~23% del presupuesto, y son baratos y útiles. Aceptado explícitamente.\n\n## Verificación\n\nCriterio de aceptación **por tasa**, no por total — el total depende del volumen\nde trabajo del mes, que va a subir en cuanto se pueda empujar sin miedo:\n\n1. **≤ 8 min facturados por run de CI** (hoy: 48 en PR, 25 en push).\n2. **≤ 2 min facturados por deploy** (hoy: 58).\n3. Meta escalonada del total: **≤ 900 min/ciclo éxito, ≤ 700 objetivo**.\n4. `actionlint` verde sobre los dos workflows (job `proto`), versión pineada.\n4b. `make ci` verde en local en menos de 5 min, en una máquina limpia.\n4c. `scripts/ci-minutes.sh` corre a la primera con UN comando documentado, y\n   ante un 403 imprime el `gh auth refresh -s user` que lo arregla.\n4d. Un deploy manual (`workflow_dispatch`) sobre un SHA sin CI verde **falla**\n   nombrando el check que falta.\n4e. Un run cancelado a media carrera deja el build de Cloud Build cancelado\n   también, no huérfano.\n5. Tests de tabla para la regla de selección de shards (Etapa 2).\n6. Un deploy real a dev: el commit desplegado es el que se envió, y un build\n   fallado **pone en rojo** el run de GitHub. Esto se prueba a propósito\n   rompiendo un build.\n7. `scripts/ci-minutes.sh` a los 30 días.\n8. `deploy/README.md` describe el pipeline que existe después del cambio.\n\n## Lo que NO entra en este plan\n\n- **Comprar minutos** (el excedente cuesta ~$20 USD/mes aun al doble del\n  consumo). Rechazado solo si el límite de gasto está deliberadamente en $0; si\n  no lo está, ésta es la opción correcta y este plan se reduce a T1+T2+T7.\n- **Hacer el repo público** (Actions es gratis e ilimitado en repos públicos).\n  Rechazado: es el sistema de gestión de un colegio que se entrega en noviembre;\n  publicarlo publica el modelo de auth, las políticas RLS y el hueco conocido\n  del limitador global de login, además de regalar el entregable del dueño.\n- **Runners auto-hospedados.** El motivo real es que no hay apetito de agregar\n  superficie operativa nueva a 73 días de la entrega — no que \"el stack local se\n  muere\", que es una memoria sobre un binario bajo `air` en esta laptop, no sobre\n  un runner administrado.\n- Cambiar de proveedor de CI.\n- Optimizar los Dockerfile (el build vive en el tramo gratuito de Cloud Build).\n\n## Notas de contexto\n\n- La memoria del proyecto dice que `Deploy dev` está bloqueado desde el 10-sep\n  por `govulncheck@latest`. **Está desactualizada**: `cloudbuild.yaml` ya pinea\n  `_GOVULNCHECK_VERSION: v1.7.0` y hubo un deploy exitoso el 2026-09-14.\n- La memoria dice que hay 47 commits sin empujar. **También desactualizada**:\n  `origin/dev == dev == 8534a22b`. Todo está empujado, y eso explica el 90%.\n\n\n","newText":"# PLAN — Bajar el consumo de minutos de GitHub Actions\n\nEstado: REVISADO (CEO) — entra a DX y Eng\nRama: `feature/optimizacion-minutos-ci`\nFecha: 2026-09-18\n\n## Problema\n\nGitHub reporta **2,700 min usados de 3,000 incluidos (90%)** en el ciclo actual.\n\n**Cuánto dinero es, dicho explícitamente:** el excedente en repo privado cuesta\n**$0.008 USD/min**. Hoy el excedente es cero. Aun duplicando el consumo a 5,500\nmin/ciclo, la factura sería **~$20 USD/mes (~370 MXN)**. Así que este plan NO se\njustifica por dinero.\n\nSe justifica por una sola premisa, que hay que confirmar antes de empezar:\n**el límite de gasto de Actions está en $0**, igual que el corte duro de\nfacturación que el dueño eligió para GCP. Con el límite en $0, agotar la cuota\napaga CI y apaga el deploy — con demos **diarias** para el colegio y entrega el\n30 nov 2026, eso es quedarse sin poder desplegar a media semana. Con un límite\ndistinto de $0, el problema real no es la cuota sino el tiempo de pared, y\nsiete de las diez tareas de aquí sobran.\n\n> **Bloqueante — cómo se resuelve, no solo dónde mirar:**\n> ```sh\n> gh auth refresh -h github.com -s user          # el endpoint pide este scope\n> gh api /users/romodevx/settings/billing/actions\n> ```\n> O en la UI: Settings → Billing → Spending limits → Actions.\n> **Regla de bifurcación:** si el límite es $0 → Etapa 1 completa. Si NO es $0 →\n> solo T1, T2 y T7 (reparación de defectos, no cuestan cobertura); el resto no\n> se justifica a ~$20 USD/mes.\n\n## Medición (no estimación)\n\nMedido con la API de Actions, 94 runs entre 2026-08-19 y 2026-09-18, sumando\n`ceil(duración_del_job / 60s)` por job — que es exactamente como factura GitHub.\n(El endpoint `/timing` devuelve `total_ms: 0` en este repo y no sirve.)\n\n**Total medido: 2,739 min** — coincide con los 2,700 de la factura (1.4% de error).\n\n| Bucket | Minutos | % | Runs |\n| --- | ---: | ---: | ---: |\n| `Deploy dev` → job `build images and roll out` | 789 | 29% | 35 |\n| `CI` → matriz `go` (14 módulos de `go.work`) | 1,619 | 59% | 50 |\n| `CI` → job `web` | 150 | 5.5% | 58 |\n| `CI` → `proto` + `openapi` + `ci` (3 jobs triviales) | 173 | 6.3% | 58 c/u |\n\n| Evento | Minutos | Runs | Promedio |\n| --- | ---: | ---: | ---: |\n| `CI` / pull_request | 1,015 | 21 | **48.3 min/run** |\n| `CI` / push a `dev` | 935 | 37 | **25.3 min/run** |\n| `Deploy dev` / workflow_run | 708 | 25 | 28.3 (58 min los exitosos) |\n| `Deploy dev` / dispatch + push | 81 | 11 | — |\n\n**Advertencia sobre la línea base:** estos 2,739 min se midieron en un mes en el\nque hubo commits retenidos sin empujar justamente para ahorrar minutos. Comparar\nun total contra un mes sin esa restricción mide dos cargas de trabajo distintas.\nPor eso el criterio de aceptación es una **tasa**, no un total (ver §Verificación).\n\n## Causas raíz (verificadas en logs, no inferidas)\n\n### C1 — El caché de Go es una lápida de 7.6 KB\n`actions/setup-go@v6` cachea `GOCACHE` + `GOMODCACHE` con una llave derivada de\n`hashFiles('**/go.sum')`. Las 14 patas de la matriz calculan **la misma llave**.\nLas entradas de caché de Actions son inmutables: la primera pata que ganó la\ncarrera guardó un caché casi vacío y quedó congelado ahí para siempre.\n\nLog del run 35396038508, pata `go (./services/iam)`:\n\n```\nCache hit for: setup-go-Linux-x64-ubuntu24-go-1.25.14-f4c882bf...\nCache Size: ~0 MB (7599 B)\nCache restored successfully\ngo: downloading cloud.google.com/go/firestore v1.22.0\n...\nCache hit occurred on the primary key ..., not saving cache.\n```\n\nReporta \"cache hit\", restaura 7.6 KB, y acto seguido **descarga todos los\nmódulos y compila el grafo completo desde cero** — en cada pata, en cada run,\ndesde que existe el workflow. Como la llave ya existe, nunca vuelve a guardar:\nel caché no se puede auto-reparar.\n\nCosto por pata en ese run: `go vet` 109 s + `go test -race` 112 s +\n`integration` 43 s + `govulncheck` 27 s = **307 s**, casi todo compilación fría.\n\n### C2 — El mismo código se compila cuatro veces por pata\n`go vet ./...` compila el grafo entero (109 s). `go test -race` lo recompila con\ninstrumentación (112 s). Integración lo compila otra vez con `-tags`.\n`govulncheck` carga el grafo una cuarta vez. `go test` **ya corre el\nsubconjunto de vet de alta confianza**.\n\n**Evidencia de que `go vet` no compra nada:** en 58 runs de CI, `go vet` y\n`go test -race` fallaron **cero** veces. Los únicos pasos de Go que se pusieron\nen rojo fueron `govulncheck` (9 patas, run 35394352750) y\n`go test -tags=integration` (students, runs 35394352750 y 35395204982). Esos dos\nse quedan en la ruta del PR: son todo el valor demostrado de la matriz.\n\n### C3 — Se paga un runner de GitHub por mirar un build de Google\nEl job `build images and roll out` dura **~3,500 s (58 min) en los deploys\nexitosos**. Hace `gcloud builds submit`, que **transmite logs de forma síncrona**\nmientras Cloud Build construye las diez imágenes en infraestructura de Google.\nEl runner no compila nada: espera, y factura mientras espera. ~10 deploys\nexitosos × 58 min = **~580 min** de cuota gastados en un `tail -f`.\n\n### C4 — Impuesto de redondeo por job\nGitHub factura cada job redondeado al minuto completo:\n\n- `proto`: 17 s de trabajo → 1 min × 58 runs = **58 min**\n- `openapi (lint)`: 9 s (3 de ellos bajando `@redocly/cli@latest`) → **57 min**\n- `ci` (un `if` de cinco líneas) → **58 min**\n- matriz `go`: 153 s reales → 3 min facturados en PR; 81 s → 2 min en push.\n  Solo el redondeo, sobre 700 jobs de matriz, son **~400 min**.\n\n### C5 — La matriz no mira el diff\nCambiar un archivo en `apps/web` corre las 14 patas de Go. `paths-ignore` solo\nexcluye markdown, `docs/` y `.claude/`.\n\n### C6 — Los dos pipelines corren la MISMA suite de compuertas\n`cloudbuild.yaml` vuelve a correr, sobre el mismo commit que CI acaba de\naprobar: `buf lint`, `go vet` (todos los módulos), `go test -race` (todos los\nmódulos), `govulncheck`, `web build` y la compuerta de source maps. Eso es la\nmayor parte de los 58 min de build. CI es la compuerta; Cloud Build debería\nconstruir imágenes y desplegar, nada más.\n\n## Objetivo\n\nQue un deploy cueste **≤ 2 min facturados** y un run de CI baje por etapas, sin\nquitar ninguna compuerta que haya atrapado algo real.\n\n**El piso aritmético manda sobre el deseo.** GitHub cobra cada job redondeado al\nminuto, y un run de PR hoy tiene **18 jobs** (14 de matriz + `proto` + `web` +\n`contract` + `ci`). Aunque cada uno terminara en 3 segundos, el run cuesta\n**18 min**. Por eso:\n\n- **Etapa 1: ≤ 18 min por run de CI** (hoy: 48 en PR, 25 en push).\n- **Etapa 2 (con T3, que baja la matriz a 4 shards → 8 jobs): ≤ 8 min por run.**\n\nPedir ≤8 en la Etapa 1 es pedir algo imposible con 18 jobs; era un criterio que\nel plan no podía cumplir ni haciendo todo bien.\n\n## Plan de implementación\n\nDos etapas. La Etapa 1 lleva ~94% del ahorro con el riesgo bajo; la Etapa 2\nlleva ~6% y concentra todo el riesgo, así que va después del 30 nov.\n\n### ETAPA 1 — antes del 30 nov\n\n#### T0 — Línea base medible antes de tocar nada\n`scripts/ci-minutes.sh`, en el estilo de `scripts/deploy-preflight.sh`\n(`/bin/sh`, solo lectura, helpers `ok/bad/warn`, exit 1): consulta la API de\nActions, suma `ceil(job_duration/60)` y reporta **min por run de CI** y **min\npor deploy**, no solo el total. Es la prueba que falla primero: hoy imprime\n~48 min/run de PR y ~58 min/deploy.\n\n#### T1 — Reparar el caché de Go (la palanca más grande de CI)\nEl diseño con `github.run_id` en la llave **no sirve**: medido en esta máquina,\n`GOMODCACHE` son **6.0 GB** y `GOCACHE` **1.0 GB**, contra un presupuesto LRU de\n**10 GB por repo**. Rotar la llave cada run llenaría y desalojaría el caché en\nmenos de un día, y los PR restaurarían basura o nada. La lápida se arregla\nhaciendo la llave **por shard**, no rotándola.\n\n- Paso 1: **medir**. Un run instrumentado que imprima el tamaño real de\n  `GOCACHE` y `GOMODCACHE` **por módulo** en Linux. El número decide el resto.\n- `cache: false` en `actions/setup-go` (deja de emitir la llave compartida).\n- **`GOMODCACHE`**: UNA entrada compartida, llave\n  `gomod-${{ hashFiles('**/go.sum') }}`, sin rotación (cambia cuando cambian las\n  dependencias, que es lo correcto). La guarda un job dedicado.\n- **`GOCACHE`**: una entrada **por módulo de la matriz**, llave\n  `gocache-${{ matrix.module }}-${{ hashFiles('**/go.sum') }}` — **sin\n  `github.sha`**. Poner el SHA rota la llave en cada push igual que `run_id`:\n  la llave primaria nunca acierta, cada push sube 14 entradas nuevas que no\n  reemplazan a las viejas, y el LRU desaloja justo la generación recién subida.\n  Para invalidar a mano existe una variable `CACHE_EPOCH` que se sube a mano.\n  **En la Etapa 1 la unidad es `matrix.module` (14 patas), no \"shard\":** los\n  shards no existen hasta T3, que es Etapa 2. Escribir la llave en términos de\n  shards deja la tarea sin poder implementarse.\n- **Guardar solo en push a `dev`** (`if: github.event_name == 'push'`). Los PR\n  restauran y no escriben: así no se desaloja nada y se cierra de paso el vector\n  de envenenamiento de caché desde una rama.\n  **Consecuencia que va escrita en el comentario del workflow:** una rama que\n  toca `go.sum` compila en frío en cada run de PR hasta que se mergea. Sin ese\n  comentario, el siguiente que lo vea concluye \"el caché volvió a romperse\".\n- **Si no cabe en 10 GB, el plan B es de Etapa 1, no T3:** cachear `GOCACHE`\n  solo en los módulos caros (iam, students, finance, comms, uniforms, grades,\n  attendance) y dejar que los baratos compilen en frío. Bajar el número de\n  shards NO es una salida disponible antes del 30 nov.\n- **Observabilidad del caché (si no, esto se repite).** La lápida sobrevivió\n  meses porque un caché vacío y uno lleno se ven igual en el log. Paso posterior\n  al restore que imprima el tamaño restaurado y avise — y falle en push a\n  `dev` — si está por debajo de un umbral.\n  **La aserción se salta cuando `cache-hit != 'true'`.** En el primer push\n  después del cambio todavía no hay caché: sin esa condición el guardia se pone\n  rojo por construcción y lo primero que hace cualquiera es borrarlo. Lo que se\n  afirma es \"un acierto no es una lápida\", no \"siempre hay caché\".\n- **El presupuesto de 10 GB no está vacío.** `ci.yml` ya guarda el store de pnpm\n  y un caché de turbo con llave `turbo-${{ runner.os }}-${{ github.sha }}` —\n  otra entrada nueva por run, compitiendo por la misma cuota y desalojando\n  entradas de Go. La llave de turbo se arregla en la misma tarea.\n- **Restaurar 6 GB también cuesta minutos**, catorce veces. El run instrumentado\n  mide el tiempo de restore, no solo el tamaño: si restaurar cuesta más de lo\n  que ahorra en un módulo barato, ese módulo no se cachea.\n\n#### T2 — Dejar de compilar lo mismo cuatro veces\n- Borrar el paso `go vet` por módulo (cero hallazgos en 58 runs, 109 s × 14 × 50).\n- `go vet` **una vez** sobre todo el workspace, en un job con caché caliente.\n  **El comando NO es `go vet ./...`.** No hay `go.mod` en la raíz, solo\n  `go.work`, así que desde la raíz falla con\n  `pattern ./...: directory prefix . does not contain modules listed in go.work`\n  (verificado ejecutándolo). `cloudbuild.yaml` ya documenta exactamente esto en\n  el comentario del paso `go-vet`. La forma que sí corre:\n  `go vet $(go work edit -json | jq -r '.Use[].DiskPath+\"/...\"')`.\n- `govulncheck`: instalar el binario **una vez** y compartirlo por caché, en vez\n  de `go install` en cada pata. El paso se queda en la ruta del PR: es uno de los\n  dos únicos que han atrapado algo.\n\n#### T5 — Fundir `openapi (lint)` y `actionlint` dentro del job `proto`\nMismo checkout, mismo runner, cero minutos extra. Pinear `@redocly/cli` a una\nversión en vez de `@latest` — mismo error que tumbó el deploy el 2026-09-10 con\n`govulncheck@latest`. `actionlint` entra aquí (el job ya tiene Go): la\nverificación del plan lo daba por instalado y no lo está.\n**`actionlint` se pinea igual**, con su variable `ACTIONLINT_VERSION`. Meterlo\ncon `@latest` en el mismo párrafo que pinea redocly sería repetir el error que\nse está corrigiendo.\n**Fundirlo obliga a editar el job `ci`.** `ci.yml:319` declara\n`needs: [proto, go, web, contract]` y `ci.yml:325` lee\n`${{ needs.contract.result }}`. Si `contract` desaparece sin tocar eso, el\nworkflow ya no parsea y **todos** los runs fallan, incluido el disparador del\ndeploy. T5 edita `needs`, la cadena `results`, y revisa el nombre del required\ncheck en la protección de rama.\nNota sobre los 9 rojos de `openapi (lint)`: no son contratos rotos que la\ncompuerta atrapó, son deriva de `@redocly/cli@latest`. El pineo los elimina; la\ncompuerta se queda por el canon spec-first, no por ese número.\n\n#### T7 — Enviar el build y esperar el veredicto, no los logs\n- `gcloud builds submit --async`, capturar el BUILD_ID, y **sondear el estado\n  terminal** con `gcloud builds describe` cada ~30 s. Costo: **~2-3 min\n  facturados** en vez de 58.\n- **El sondeo lleva fecha límite.** Sin tope, un build colgado factura el job de\n  GitHub hasta el límite de 6 h — justo la falla que este plan existe para\n  evitar. Deadline explícito (p. ej. 75 min) que falla el job con el BUILD_ID.\n- **Y lleva trampa de cancelación.** `cancel-in-progress` cancela el *runner*,\n  no el build ya enviado: el build viejo sigue vivo y llega igual a\n  `gcloud run services update`. Un `trap` que corra\n  `gcloud builds cancel $BUILD_ID` antes de salir es lo que de verdad cierra la\n  carrera; el sondeo solo no basta.\n- **Qué imprime cuando falla.** Hoy el runner transmite los logs, así que un\n  build roto se diagnostica en la misma pantalla. Cambiarlo por un estado y un\n  BUILD_ID es una **regresión** de diagnóstico. En cualquier estado terminal\n  distinto de SUCCESS, el paso escribe al resumen del job: el paso que falló,\n  las últimas ~100 líneas (`gcloud builds log $BUILD_ID`) y la URL de consola.\n- **No es fire-and-forget, y eso es deliberado.** Un envío sin espera rompe en\n  silencio `concurrency: deploy-dev, cancel-in-progress`: hoy ese candado\n  funciona solo porque el runner sostiene el build entero. Sin la espera, dos\n  pushes seguidos lanzan dos builds que se carreran a\n  `gcloud run services update` y **el más viejo puede aterrizar al final**. Esa\n  es exactamente la falla de \"la demo del 12 de noviembre corrió el commit\n  equivocado dos días\". El sondeo conserva el candado y conserva la X roja.\n- La X roja importa el doble aquí: la mitigación que este plan proponía en su\n  primera versión (alerta por correo) **está rota y documentado que lo está** —\n  el correo saliente de este proyecto cae en spam y quedó bloqueado en una\n  credencial SMTP.\n- Migraciones, rollout y smoke test se mueven a pasos de `cloudbuild.yaml` con\n  `_DEPLOY=true`, conservando el orden: **esquema antes que código**; si la\n  migración falla no se despliega nada.\n- **Ojo con \"el orden ya probado\": ese camino nunca se ha ejecutado.** El paso\n  `deploy` de `cloudbuild.yaml` está detrás de `_DEPLOY=true` y `deploy-dev.yml`\n  manda `_DEPLOY=false` en cada run, así que lo probado son los pasos de GitHub,\n  no los de Cloud Build. Además el paso de cloudbuild usa `gcloud run deploy`\n  mientras dev siempre se ha desplegado con `gcloud run services update`. Se\n  portan los pasos de GitHub **literalmente**, no se confía en los que ya están.\n- **Dos pasos se quedan en el runner, después del sondeo, y hay que decirlo:**\n  `Confirm nothing was left warm` (el guardia de costo de min-instances) y el\n  resumen `if: always()` de qué commit corre dev. El plan no les asignaba dueño\n  y son justo lo que evita una factura sorpresa y un \"¿la demo está caída?\".\n- **El rollout va al final del build y es idempotente.** El `trap` de\n  cancelación puede caer entre el servicio 5 y el 6 y dejar dev partido en dos\n  SHAs; un runner cancelado tampoco corre el resumen `if: always()`, así que\n  nadie sabría qué mitad aterrizó. El trap imprime el estado de imagen por\n  servicio antes de salir.\n- **No requiere cambios de Terraform.** `deploy/terraform/main.tf:183` ya le da\n  al SA de build `roles/run.admin`, `roles/cloudsql.client`,\n  `roles/iam.serviceAccountUser` y acceso al secreto\n  `garcia-lorca-db-migrator-password-dev`, con un comentario que dice\n  literalmente que es para que el deploy corra `scripts/migrate-cloud.sh` antes\n  del rollout. La infraestructura lleva ahí desde el incidente del 2026-09-07.\n- **Restricción de secuencia (bloqueante para probarlo):** un workflow\n  `workflow_run` siempre ejecuta la copia de la **rama por defecto**, que aquí\n  es `master`. Editar `deploy-dev.yml` en una rama **no tiene efecto** hasta que\n  llega a `master`. T7 va al final de la etapa y no se puede validar en rama.\n\n#### T8b — El contrato nunca se valida cuando cambia el contrato (defecto vivo)\nNo lo introduce este plan: ya pasa. `paths-ignore` excluye `docs/**` y el\ncontrato vive en `docs/api/openapi.yaml`, así que **un PR que solo toca el\ncontrato se salta CI entero** y `openapi (lint)` no corre justo cuando importa.\nContradice el canon spec-first (`api-standards.md`: el contrato se lintea ANTES\nde implementar). `paths-ignore` no admite negaciones, así que la salida es\nmover el contrato fuera de `docs/` o darle un disparador `paths:` propio.\n\n#### T9 — Higiene de disparadores\n- **NO por `paths-ignore`.** `paths-ignore` es una llave de `on:`, no de job:\n  no existe forma de limitarla \"a los jobs de Go\". Agregar `apps/mobile/**`\n  saltaría el workflow **completo** en un cambio solo-móvil, que es el único\n  lugar donde el móvil se prueba (`pnpm turbo run test lint` corre todo el\n  workspace), y dejaría el required check sin aparecer nunca: el PR se traba.\n- La forma correcta es un filtro por archivos (el mismo script de T4, o\n  `dorny/paths-filter`) que deje las patas de Go en no-op **mientras el workflow\n  sigue reportando**. Hasta que exista ese filtro, T9 se limita a lo de abajo.\n- `concurrency` de CI ya cancela runs superados; la llave `github.ref` es correcta.\n\n#### T10 — Quitar de Cloud Build las compuertas que CI ya corrió (C6)\nBorrar `go-vet`, `go-test`, `govulncheck`, `web-build` y `sourcemap-gate` de\n`cloudbuild.yaml`; dejar `buf generate`, las diez imágenes, migraciones, rollout\ny smoke. Esto es lo que hace que T7 **quepa** en los 120 min/día gratuitos de\ne2-medium en vez de pasarse: sin esto, añadir migraciones y rollout a un build\nque ya dura 58 min lo empuja arriba de 60, y dos deploys al día rozan el tope.\nEl tope de GCP es un corte duro que tira el ambiente de la demo; el de GitHub\nsolo frena CI.\n\n**Guardia obligatoria que viene con T10.** Quitar las compuertas de Cloud Build\nsolo es correcto mientras TODO deploy venga después de un CI verde. Hoy no lo\nes: `deploy-dev.yml:60` deja pasar `workflow_dispatch` a propósito, y un\n`gcloud builds submit` a mano también. Sin guardia, un deploy manual publicaría\ncódigo sin `go test`, sin `govulncheck` y **sin la compuerta de source maps**,\nque es regla del proyecto. Antes de enviar el build, el job verifica que el SHA\nexacto tenga un check-run de CI en verde y falla nombrando el que falta;\nsaltarla exige una bandera explícita, no es el camino por defecto.\n**Y necesita permiso.** `deploy-dev.yml` declara solo `contents: read` e\n`id-token: write`, y su propio comentario dice que `checks: read` se quitó al\ncambiar a `workflow_run`. Sin volver a agregarlo la guardia da 403 en **todos**\nlos deploys — y como es compuerta dura, los bloquea todos. T10 agrega\n`checks: read` (y `actions: read` si consulta runs).\n\n#### T11 — `make ci`: la misma compuerta, en la laptop, antes de empujar\nLa causa de raíz de 21 runs de PR a 48 min es que los fallos se descubren\n**después** de empujar. El `Makefile` ya tiene `generate`, `lint`, `test` y\n`build` con sus cadenas de ayuda `##`; falta el objetivo que los componga en\n\"lo que corre CI\", más `actionlint`. No es un script nuevo: es un target que\nreusa los que ya existen. `make ci-minutes` entra al lado de `deploy-preflight`,\ncon el mismo patrón.\n**Objetivo honesto: verde en local en menos de 5 min *en caliente*, y el target\ndice qué compuertas NO corre.** En una máquina fría son un `pnpm install`, la\ncompilación de 14 módulos de Go y testcontainers bajando Postgres — muy por\nencima de 5 min según la propia cifra de 300 s por módulo en frío. Si el\ncriterio se escribe como \"5 min en limpio\", se va a cumplir quitándole\ncompuertas al target, que es justo lo contrario de lo que sirve.\n\n#### T12 — Actualizar la documentación del pipeline\nCuatro cambios a \"qué hace CI\" (desaparece `go vet`, el deploy deja de\ntransmitir logs, Cloud Build deja de ser compuerta, aparece un script nuevo) y\ncero tareas de documentación. `deploy/README.md` describe el pipeline actual y\nquedaría mintiendo. Se actualiza: qué corre dónde, por qué `go vet` ya no está,\ncómo se lee un deploy fallido, y cómo se corre `make ci` / `ci-minutes.sh`.\nDocumentación vieja aquí cuesta runs desperdiciados y hace que alguien vuelva a\nproponer T8 dentro de tres meses.\n\n### ETAPA 2 — después del 30 nov (no antes)\n\n#### T3 — Colapsar la matriz de 14 patas a 4 shards\nAgrupar los módulos por tiempo medido. Mata diez arranques y diez redondeos por\nrun. **Deuda que introduce:** la agrupación es una lista a mano que se desfasa\ncuando se agrega un servicio; hoy `go.work` es la única fuente de verdad y esto\nla rompe. Mitigación: un test que falle si un módulo de `go.work` no está en\nningún shard.\n\n#### T4 — Correr solo los módulos que el diff puede afectar\nRegla explícita, en un script con tests de tabla, no en YAML:\n- Cambia `pkg/`, `gen/`, `proto/`, `go.work`, `go.work.sum` o el workflow →\n  **todos** los shards.\n- Si no, solo los shards con un `services/<x>` tocado.\n- **La lista de archivos cambiados se lee por `env:`, nunca interpolando\n  `${{ }}` dentro de un `run:`** — títulos de PR y nombres de rama son texto que\n  un atacante controla, y `${{ }}` los mete como código shell.\n- **Escotilla de escape, documentada junto a la regla:** un token `[ci full]` en\n  el mensaje del commit (o una entrada de `workflow_dispatch`) fuerza las 14\n  patas. Sin ella, cuando la heurística se equivoca la gente empuja commits\n  vacíos para vencer el filtro, que es peor que no tener filtro.\n\n#### T6 — Eliminar el job agregador `ci`\nCon T3 los nombres son fijos y se registran como required checks directamente.\n\n### RECHAZADO — T8 (no re-probar el push a `dev`)\n\n**La premisa era falsa y la verifiqué.** El plan decía que el push a `dev`\nre-verifica lo mismo que el PR. No:\n\n1. `ci.yml:176-205` deja `-race`, integración y `govulncheck` en\n   `if: github.event_name == 'pull_request'`. El push corre una compuerta\n   **distinta y más débil**, no la misma.\n2. Los merges a `dev` son **squash**: `git rev-list --parents -n1` devuelve un\n   solo padre en `8534a22b`, `d2b40091`, `875978c5` y `bf86d107`. El commit que\n   se despliega es un SHA que **ningún run de PR vio nunca**.\n3. `check-runs` sobre `8534a22b` confirma que su CI verde viene del run de\n   **push** (35397296141) — justamente el que T8 quería borrar.\n\nT8 no eliminaba trabajo redundante: eliminaba **la única verificación que\nexiste sobre el árbol exacto que llega al colegio**. Los 935 min del push se\nabaratan con T1 y T3; no se quitan.\n\n## Ahorro proyectado\n\n| Cambio | De | A | Ahorro |\n| --- | ---: | ---: | ---: |\n| T1+T2 — caché + compilaciones duplicadas | 1,619 | ~500 | ~1,120 |\n| T5 — fundir openapi+actionlint en proto | 57 | 0 | ~57 |\n| T7 — enviar y sondear en vez de mirar | 789 | ~35 | ~754 |\n| **Etapa 1** | **2,739** | **~890** | **~1,930 (70%)** |\n| T3+T4+T6 (Etapa 2) | ~890 | ~700 | ~190 (7%) |\n\n`web` (150 min) y `proto` (58 min) se quedan como están: tras la Etapa 1 pasan a\nser ~23% del presupuesto, y son baratos y útiles. Aceptado explícitamente.\n\n## Verificación\n\nCriterio de aceptación **por tasa**, no por total — el total depende del volumen\nde trabajo del mes, que va a subir en cuanto se pueda empujar sin miedo:\n\n1. **≤ 18 min facturados por run de CI** en la Etapa 1; **≤ 8 min** solo\n   después de T3 (ver §Objetivo: 18 jobs × 1 min de piso).\n2. **≤ 2 min facturados por deploy** (hoy: 58).\n3. Meta escalonada del total: **≤ 900 min/ciclo éxito, ≤ 700 objetivo**.\n3b. Un ensayo de **rollback**: después de un deploy por el camino nuevo, volver\n   dev al SHA anterior y confirmar que responde. Nada en el plan lo probaba.\n4. `actionlint` verde sobre los dos workflows (job `proto`), versión pineada.\n4b. `make ci` verde en local en menos de 5 min **en caliente**, con la lista de\n   compuertas que omite escrita en su cadena de ayuda `##`.\n4c. `scripts/ci-minutes.sh` corre a la primera con UN comando documentado, y\n   ante un 403 imprime el `gh auth refresh -s user` que lo arregla.\n4d. Un deploy manual (`workflow_dispatch`) sobre un SHA sin CI verde **falla**\n   nombrando el check que falta.\n4e. Un run cancelado a media carrera deja el build de Cloud Build cancelado\n   también, no huérfano.\n5. Tests de tabla para la regla de selección de shards (Etapa 2).\n6. Un deploy real a dev: el commit desplegado es el que se envió, y un build\n   fallado **pone en rojo** el run de GitHub. Esto se prueba a propósito\n   rompiendo un build.\n7. `scripts/ci-minutes.sh` a los 30 días.\n8. `deploy/README.md` describe el pipeline que existe después del cambio.\n\n## Lo que NO entra en este plan\n\n- **Comprar minutos** (el excedente cuesta ~$20 USD/mes aun al doble del\n  consumo). Rechazado solo si el límite de gasto está deliberadamente en $0; si\n  no lo está, ésta es la opción correcta y este plan se reduce a T1+T2+T7.\n- **Hacer el repo público** (Actions es gratis e ilimitado en repos públicos).\n  Rechazado: es el sistema de gestión de un colegio que se entrega en noviembre;\n  publicarlo publica el modelo de auth, las políticas RLS y el hueco conocido\n  del limitador global de login, además de regalar el entregable del dueño.\n- **Runners auto-hospedados.** El motivo real es que no hay apetito de agregar\n  superficie operativa nueva a 73 días de la entrega — no que \"el stack local se\n  muere\", que es una memoria sobre un binario bajo `air` en esta laptop, no sobre\n  un runner administrado.\n- Cambiar de proveedor de CI.\n- Optimizar los Dockerfile (el build vive en el tramo gratuito de Cloud Build).\n\n## Notas de contexto\n\n- La memoria del proyecto dice que `Deploy dev` está bloqueado desde el 10-sep\n  por `govulncheck@latest`. **Está desactualizada**: `cloudbuild.yaml` ya pinea\n  `_GOVULNCHECK_VERSION: v1.7.0` y hubo un deploy exitoso el 2026-09-14.\n- La memoria dice que hay 47 commits sin empujar. **También desactualizada**:\n  `origin/dev == dev == 8534a22b`. Todo está empujado, y eso explica el 90%.\n\n\n"}]} -->

### Fase 3 — Eng (FULL REVIEW)

Voces: subagente completado (12 hallazgos, 6 bloqueantes/altos verificados
contra el repo); Codex **no instalado** → sin cobertura externa. Consenso N/A.
Veredicto de la voz externa: REVISE. Sin tension cross-model: cada hallazgo se
verifico ejecutandolo o leyendo el archivo, asi que no habia nada que arbitrar.
Complejidad: 6 archivos en Etapa 1, 0 servicios nuevos — no dispara reduccion.
Artefacto de pruebas: ~/.gstack/projects/romodevx-garcia-lorca/andresromo-feature-optimizacion-minutos-ci-eng-review-test-plan-20260918-153701.md
Paralelizacion: Lane A (scripts/Makefile/docs) y Lane B (ci.yml) en paralelo;
Lane C (cloudbuild + deploy-dev) al final, porque T7 no se valida desde rama.

<!-- autoplan-accepted:eng -->
- T2 corrige el comando: `go vet ./...` NO existe aqui (no hay go.mod raiz; verificado: "pattern ./...: directory prefix . does not contain modules listed in go.work"). La forma correcta es `go vet $(go work edit -json | jq -r '.Use[].DiskPath+"/..."')`. REGRESION: el texto anterior borraba 14 vets que funcionan y los cambiaba por un comando que falla en la primera linea.
- T1 quita `github.sha` de la llave del cache: rotaba igual que `run_id` y volvia a producir la misma churn. Se usa `CACHE_EPOCH` manual para invalidar. Verificacion: la llave primaria acierta en el segundo push.
- T1 arregla tambien la llave de turbo (`turbo-...-${{ github.sha }}`), que compite por los mismos 10 GB.
- T1 mide tiempo de restore, no solo tamano: un modulo cuyo restore cuesta mas que su compilacion en frio no se cachea.
- T1 salta la asercion de tamano cuando `cache-hit != 'true'`, para que el primer push no se ponga rojo por construccion.
- T5 edita `needs` y la cadena `results` del job `ci` (ci.yml:319 y :325). REGRESION: borrar el job `contract` sin eso deja el workflow sin parsear y TODOS los runs fallan, incluido el disparador del deploy.
- T10 agrega `checks: read` (y `actions: read` si aplica) a `deploy-dev.yml`. REGRESION: hoy solo tiene `contents: read` e `id-token: write`, asi que la guardia daria 403 y bloquearia todos los deploys.
- T9 NO usa `paths-ignore` para el movil: es una llave de `on:`, no de job, y saltaria el workflow entero dejando el required check sin aparecer. Se usa un filtro por archivos que deja las patas en no-op y el workflow reportando.
- Nuevo T8b: el contrato nunca se valida cuando cambia el contrato — `docs/**` esta en `paths-ignore` y el contrato es `docs/api/openapi.yaml`. Defecto vivo, no introducido por este plan.
- T7 porta los pasos de GitHub literalmente: el camino `_DEPLOY=true` de cloudbuild NUNCA se ha ejecutado y usa `gcloud run deploy` donde dev siempre uso `services update`.
- T7 deja en el runner, despues del sondeo, el guardia de min-instances y el resumen `if: always()` de que commit corre dev.
- T7 pone el rollout al final y lo hace idempotente; el trap imprime el estado de imagen por servicio antes de salir, porque una cancelacion a media carrera puede dejar dev partido en dos SHAs.
- El criterio de CI pasa a ≤18 min/run en Etapa 1 y ≤8 min solo tras T3: con 18 jobs y cobro por minuto, el piso aritmetico es 18 min. Verificacion: el run 35396038508 tiene 18 jobs.
- T11 apunta a <5 min EN CALIENTE y su cadena `##` nombra las compuertas que omite.
- Se agrega un ensayo de rollback: volver dev al SHA anterior y confirmar que responde.
<!-- /autoplan-accepted:eng -->

### Diagrama del pipeline (Eng — antes / después de la Etapa 1)

```
HOY — un commit a dev cuesta ~25 min de CI + ~58 min de deploy
─────────────────────────────────────────────────────────────────────────────
  push/PR
     │
     ├─ CI (workflow)                                       1,619 + 150 + 173
     │    ├─ proto ─────────────┐                                  (58 min)
     │    ├─ openapi ───────────┤  3 jobs triviales, 1 min de piso (57 min)
     │    ├─ contract ──────────┘                                  (58 min)
     │    ├─ web ──────────────────────────────────────────────── (150 min)
     │    └─ go (matriz × 14 módulos de go.work)              ── (1,619 min)
     │         │  caché: llave hashFiles('**/go.sum') → MISMA en las 14 patas
     │         │  restaura 7.6 KB ← lápida; "cache-hit: true" miente
     │         ├─ go vet      (109 s, compila de cero)   ┐
     │         ├─ go build    (compila de cero)          │ el MISMO código
     │         ├─ go test -race (compila de cero)        │ compilado 4 veces
     │         └─ govulncheck (compila de cero)          ┘ por pata
     │              └─ ci (agregador: un if de 5 líneas)           (58 min)
     │
     └─ Deploy dev (workflow_run) ───────────────────────────────── (789 min)
          └─ build images and roll out
               └─ gcloud builds submit  ← el runner de GitHub se queda
                    │                      MIRANDO un build de Google
                    │                      58 min facturados por esperar
                    └─ Cloud Build: vet + test-race + govulncheck + web build
                       + sourcemap gate  ← las MISMAS compuertas que CI
                       ya corrió sobre el mismo commit (C6)

DESPUÉS DE LA ETAPA 1 — ~890 min/ciclo
─────────────────────────────────────────────────────────────────────────────
  push/PR
     │
     ├─ CI                                                    ~500 + 150 + 116
     │    ├─ proto + openapi + actionlint  (T5: fundidos)     ── (58 min)
     │    ├─ web                                              ── (150 min)
     │    ├─ contract (T5 fundido; T8b: ya corre si cambia
     │    │            docs/api/openapi.yaml)
     │    └─ go (matriz × 14)                                  ── (~500 min)
     │         │  caché: llave POR MÓDULO + GOMODCACHE compartido
     │         │        + CACHE_EPOCH manual (E2) — guarda solo en push
     │         │  imprime el tamaño restaurado y se pone ROJO bajo umbral (T1b)
     │         ├─ go build                     ┐ vet una sola vez, a nivel
     │         ├─ go test -race                │ workspace, con la forma
     │         ├─ govulncheck (pineado)        │ correcta (E1):
     │         └─ [vet workspace-wide, 1 job]  ┘ go work edit -json | jq …
     │              └─ ci (agregador: needs[] y results actualizados — E4)
     │
     └─ Deploy dev                                             ── (~35 min)
          └─ guardia: check-run de CI VERDE sobre el SHA exacto (T10b)
             │        requiere permissions: checks: read (E5)
             └─ gcloud builds submit --async → BUILD_ID
                  ├─ sondeo con deadline; estado terminal → verde/rojo (T7)
                  ├─ trap → gcloud builds cancel $BUILD_ID (T7b)
                  ├─ no-SUCCESS → paso fallido + 100 líneas + URL (T7c)
                  └─ Cloud Build (tramo gratuito, 120 min/día):
                     migrate-cloud.sh → rollout idempotente AL FINAL (E11)
                     SIN las compuertas que CI ya corrió (T10)
                  └─ en el runner: warm-check + resumen if: always() (E10)
```

Lectura del diagrama: el ahorro no viene de correr menos pruebas. Viene de
(a) dejar de recompilar lo mismo cuatro veces porque el caché está vacío,
(b) dejar de pagar un runner de GitHub por mirar un build de Google, y
(c) dejar de correr las mismas compuertas dos veces sobre el mismo commit.
Ninguna compuerta de calidad desaparece de la ruta del PR.

### Temas cross-fase

Tres preocupaciones aparecieron de forma independiente en 2+ fases. Señal alta.

**Tema 1 — Este pipeline falla en silencio, y el plan lo empeoraba.**
Detectado en CEO (el GAP crítico: el desalojo de caché degrada sin ponerse
rojo), en DX (DX-5: la lápida de 7.6 KB sobrevivió meses porque un acierto y
un fallo se ven idénticos en el log) y en Eng (E9: una guardia que se pone
roja en el arranque es una guardia que alguien borra). Consecuencia adoptada:
cada cambio de este plan trae su propia forma de verse — T1b imprime y falla
bajo umbral, T7c imprime el paso fallido, E11 imprime el estado por servicio.
El plan pasó de "ahorrar minutos" a "ahorrar minutos sin perder el rojo".

**Tema 2 — Quitar algo de este pipeline sin mirar quién lo lee.**
CEO propuso T10 (quitar compuertas de cloudbuild) y T8 (no re-probar el push).
DX respondió con T10b (hace falta una guardia de CI verde, porque
deploy-dev.yml:60 deja pasar workflow_dispatch a propósito). Eng encontró tres
regresiones del mismo molde: E4 (borrar el job `contract` sin tocar `needs[]`
ni la cadena `results` deja el workflow sin parsear y **todos** los runs
fallan), E5 (la guardia da 403 sin `checks: read`) y E6 (`paths-ignore` es una
llave de `on:`, no de job: saltaría el workflow entero y dejaría el required
check colgado). Consecuencia: ninguna eliminación de este plan se aplica sin
nombrar antes a su lector aguas abajo.

**Tema 3 — "Enviar y salir" no es ahorro, es pérdida de control.**
T7 apareció en las tres fases. CEO exigió sondear el estado terminal en vez de
disparar y salir (si no, el candado de concurrencia deja de funcionar y el
canal de alerta por correo está roto de forma conocida). DX añadió el deadline,
el `trap` de cancelación (DX-2: `cancel-in-progress` cancela el runner, no el
build) y el rescate de diagnóstico (DX-3). Eng añadió que el camino
`_DEPLOY=true` de cloudbuild **nunca se ha ejecutado** y usa `gcloud run
deploy` donde dev siempre usó `services update` (E10), y que el rollout debe
ir al final y ser idempotente porque una cancelación a media carrera puede
dejar dev partido en dos SHAs (E11). El ahorro de 754 min es real; el precio
es que T7 es la tarea más delicada del plan y por eso lleva ensayo de rollback.

<!-- AUTONOMOUS DECISION LOG -->
## Decision Audit Trail

Una fila por bifurcación real. Las aceptaciones mecánicas restantes están
enumeradas íntegras en los bloques de aceptación por fase de arriba.

| # | Fase | Decisión | Clase | Principio | Razón | Rechazado |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | CEO | Mantener `govulncheck` e integración en la ruta del PR | Mecánica | P1 completitud | Son los **únicos** dos pasos de Go que se han puesto rojos en 58 runs (35394352750, 35395204982) | Moverlos a un run nocturno (propuesta F9 de la voz externa) |
| 2 | CEO | Borrar T8 (no re-probar el push a `dev`) | Mecánica | P4 DRY | Los merges a dev son squash (un padre) y el CI verde de 8534a22b viene del run de push: la re-prueba no existía | Conservar T8 como "seguridad" |
| 3 | CEO | Diferir T3/T4/T6 a Etapa 2 (después del 30 nov) | **Gusto** | P3 pragmatismo | 7% del beneficio (~190 min) y todo el riesgo de deuda, a 73 días de la entrega | Hacer los 6 cambios en una sola etapa |
| 4 | CEO | Criterio de aceptación por **tasa**, no por total | Mecánica | P1 completitud | La línea base se midió en un mes con commits retenidos a propósito; un total compara dos cargas distintas | "≤700 min/ciclo" a secas |
| 5 | CEO | Nuevo T10: quitar de cloudbuild las compuertas que CI ya corrió | Mecánica | P2 boil lakes | Está en el radio de impacto y es lo que hace que T7 quepa en los 120 min/día gratuitos | Dejar cloudbuild intacto |
| 6 | CEO | Aceptar sin mitigar el GAP de desalojo de caché | **Gusto** | P3 pragmatismo | Se delata como regresión de minutos en `ci-minutes.sh`, no como fallo; mitigarlo de verdad pide telemetría que no existe | Construir alerta de desalojo ahora |
| 7 | CEO | Dejar `web` (150) y `proto` (58) como están | **Gusto** | P3 pragmatismo | Tras la Etapa 1 son ~23% del presupuesto, y son baratos y útiles | Optimizarlos también (~200 min más) |
| 8 | DX | T1 usa `matrix.module` como unidad, no "shard" | Mecánica | P5 explícito | T3 (los shards) es Etapa 2: la tarea no se podía implementar como estaba escrita | Escribir T1 contra shards inexistentes |
| 9 | DX | Plan B de caché = cachear solo los módulos caros | **Gusto** | P3 pragmatismo | Si no cabe en 10 GB, reducir shards reintroduce T3; el run de medición decide | Reducir el número de shards |
| 10 | DX | Nuevo T11: `make ci` + `make ci-minutes` | Mecánica | P2 boil lakes | 21 runs de PR a 48 min porque los fallos solo se descubren empujando; el Makefile ya tiene las piezas | Dejar el bucle de retroalimentación en 48 min |
| 11 | DX | T10b: guardia de check-run verde sobre el SHA exacto | Mecánica | P1 completitud | `deploy-dev.yml:60` deja pasar `workflow_dispatch` a propósito; sin guardia, T10 despacharía código sin compuerta de source-maps | Confiar en que nadie use dispatch |
| 12 | Eng | E1: `go vet $(go work edit -json \| jq …)` en vez de `go vet ./...` | Mecánica | P5 explícito | **Regresión verificada ejecutándola**: no hay `go.mod` raíz → "directory prefix . does not contain modules listed in go.work" | El texto anterior de T2 |
| 13 | Eng | E2: quitar `github.sha` de la llave; usar `CACHE_EPOCH` manual | Mecánica | P5 explícito | `github.sha` rota en cada push: reproducía exactamente la churn por la que se rechazó `run_id` | La llave con `github.sha` |
| 14 | Eng | E3: criterio ≤18 min/run en Etapa 1, ≤8 solo tras T3 | Mecánica | P5 explícito | 18 jobs × 1 min de piso de cobro: ≤8 es aritméticamente imposible antes de fragmentar (run 35396038508) | El ≤8 min de la Etapa 1 |
| 15 | Eng | Nuevo T8b: el contrato nunca se valida cuando cambia el contrato | **Gusto** | P2 boil lakes | Defecto **vivo**, no introducido aquí: `docs/**` está en `paths-ignore` y el contrato es `docs/api/openapi.yaml`. En radio, barato, contradice el canon spec-first | Registrarlo como deuda aparte |
| 16 | Eng | Ensayo de rollback como criterio de aceptación | Mecánica | P1 completitud | Nada en el plan verificaba que el camino nuevo de deploy se pueda deshacer | Confiar en el rollout idempotente |
| 17 | Eng | No reducir alcance pese al veredicto REVISE de la voz externa | Mecánica | P6 sesgo a la acción | 6 archivos en Etapa 1, 0 servicios nuevos; los 12 hallazgos se arreglan dentro del plan, no recortándolo | Partir el plan en dos PRs |

## Implementation Tasks

Agregadas de las tres fases (CEO, DX, Eng). 32 tareas: 19 P1, 8 P2, 5 P3.
P1 bloquea; P2 va en la misma rama; P3 es seguimiento.

- [ ] **T2 (P1, human: ~1h / CC: ~10min) — ci** — Delete the per-module go vet step; run vet once workspace-wide; install govulncheck once
  - Surfaced by: ceo-review — C2 — go vet and go test -race failed 0 times in 58 runs; vet costs 109s x 14 legs x 50 runs
  - Files: .github/workflows/ci.yml
- [ ] **T1 (P1, human: ~4h / CC: ~30min) — ci** — Repair the Go cache: per-shard key, shared GOMODCACHE entry, save only on push; measure sizes first
  - Surfaced by: ceo-review — C1 + F7 — cache reports hit and restores 7599 B; measured 6.0GB GOMODCACHE + 1.0GB GOCACHE vs a 10GB LRU budget
  - Files: .github/workflows/ci.yml
- [ ] **T7 (P1, human: ~1d / CC: ~1h) — deploy** — Submit Cloud Build async and poll terminal status; move migrate/rollout/smoke into cloudbuild.yaml
  - Surfaced by: ceo-review — C3 + A3 + F4 — 58 billed min watching a Google build; pure fire-and-forget silently defeats the concurrency lock and the alert channel is known broken
  - Files: .github/workflows/deploy-dev.yml, deploy/cloudbuild/cloudbuild.yaml
- [ ] **T10 (P1, human: ~2h / CC: ~15min) — deploy** — Strip from cloudbuild.yaml the quality gates CI already ran
  - Surfaced by: ceo-review — C6 + F3 — Cloud Build re-runs vet, test-race, govulncheck, web build and the sourcemap gate on the same commit; this is what keeps T7 inside the 120 free build-min/day
  - Files: deploy/cloudbuild/cloudbuild.yaml
- [ ] **T0 (P1, human: ~3h / CC: ~20min) — scripts** — Add scripts/ci-minutes.sh reporting billed min PER CI RUN and PER DEPLOY
  - Surfaced by: ceo-review — F5 — baseline was measured during a month with commits deliberately withheld; a total compares two workloads, so the criterion must be a rate
  - Files: scripts/ci-minutes.sh
- [ ] **E2 (P1, human: ~20min / CC: ~5min) — ci** — T1: drop github.sha from the GOCACHE key; add a manual CACHE_EPOCH instead
  - Surfaced by: eng-review — E2, 9/10 — github.sha rotates every push, reproducing the exact churn the run_id design was rejected for
  - Files: .github/workflows/ci.yml
- [ ] **E1 (P1, human: ~30min / CC: ~5min) — ci** — T2: use go vet $(go work edit -json | jq -r '.Use[].DiskPath+\"/...\"') — go vet ./... does not work here
  - Surfaced by: eng-review — E1 REGRESSION, confidence 9/10, verified by running it: no root go.mod, cloudbuild.yaml already documents this
  - Files: .github/workflows/ci.yml
- [ ] **E4 (P1, human: ~30min / CC: ~5min) — ci** — T5: edit the ci aggregator's needs[] and results string when folding the contract job in
  - Surfaced by: eng-review — E4 REGRESSION, 9/10 — ci.yml:319 needs contract, :325 reads needs.contract.result; removing the job is a parse error that fails every run
  - Files: .github/workflows/ci.yml
- [ ] **E6 (P1, human: ~1h / CC: ~15min) — ci** — T9: filter Go legs by changed files instead of paths-ignore, so the workflow still reports
  - Surfaced by: eng-review — E6, 9/10 — paths-ignore is an on: key not a job key; apps/mobile there skips the whole workflow and deadlocks the required check
  - Files: .github/workflows/ci.yml
- [ ] **RB (P1, human: ~1h / CC: ~15min) — deploy** — Rehearse a rollback: return dev to the previous SHA and confirm it answers
  - Surfaced by: eng-review — Test review — nothing in the plan verified that the new deploy path can be undone
  - Files: .github/workflows/deploy-dev.yml
- [ ] **E5 (P1, human: ~10min / CC: ~5min) — deploy** — T10: add checks: read (and actions: read) to deploy-dev.yml permissions
  - Surfaced by: eng-review — E5 REGRESSION, 8/10 — only contents:read and id-token:write are granted, so the CI guard 403s and blocks every deploy
  - Files: .github/workflows/deploy-dev.yml
- [ ] **E10 (P1, human: ~3h / CC: ~30min) — deploy** — T7: port the GitHub deploy steps verbatim; keep the warm-check and state summary on the runner
  - Surfaced by: eng-review — E10, 7/10 — the _DEPLOY=true cloudbuild path has never executed and uses run deploy where dev uses services update
  - Files: deploy/cloudbuild/cloudbuild.yaml, .github/workflows/deploy-dev.yml
- [ ] **E11 (P1, human: ~1h / CC: ~15min) — deploy** — T7: make the rollout the last build step and idempotent; trap prints per-service image state
  - Surfaced by: eng-review — E11, 7/10 — a cancel landing mid-rollout leaves dev split across two SHAs with no summary
  - Files: deploy/cloudbuild/cloudbuild.yaml
- [ ] **E3 (P1, human: ~15min / CC: ~5min) — docs** — Restate the CI criterion: <=18 min/run in Etapa 1, <=8 min only after T3
  - Surfaced by: eng-review — E3, 9/10 — 18 jobs x 1 min billing floor makes <=8 arithmetically impossible before sharding
  - Files: .claude/docs/PLAN-minutos-ci.md
- [ ] **T1b (P1, human: ~1h / CC: ~10min) — ci** — T1: print restored cache size after restore and fail on push to dev below a threshold
  - Surfaced by: devex-review — DX-5 — the 7.6 KB tombstone survived for months because a miss and a hit look identical in the log
  - Files: .github/workflows/ci.yml
- [ ] **T10b (P1, human: ~2h / CC: ~20min) — deploy** — T10: assert a green CI check-run on the exact SHA before submitting; workflow_dispatch and manual submits must not bypass it
  - Surfaced by: devex-review — DX-4 — deploy-dev.yml:60 deliberately lets workflow_dispatch through, so stripping cloudbuild gates would ship unverified code with no source-map gate
  - Files: .github/workflows/deploy-dev.yml
- [ ] **T7b (P1, human: ~2h / CC: ~20min) — deploy** — T7: add a polling deadline and a trap running gcloud builds cancel on runner cancellation
  - Surfaced by: devex-review — DX-2 — cancel-in-progress cancels the runner, not the submitted build, so the older build still reaches gcloud run services update
  - Files: .github/workflows/deploy-dev.yml
- [ ] **T7c (P1, human: ~1h / CC: ~10min) — deploy** — T7: on any non-SUCCESS terminal state print the failing step, last 100 log lines and the console URL to the job summary
  - Surfaced by: devex-review — DX-3 — replacing streamed logs with a status string is a diagnostic regression, and the email fallback is known broken
  - Files: .github/workflows/deploy-dev.yml
- [ ] **T11 (P1, human: ~2h / CC: ~15min) — tooling** — Add a make ci target composing the existing generate/lint/test targets, plus pinned actionlint; add make ci-minutes beside deploy-preflight
  - Surfaced by: devex-review — DX-8 + Pass 6 — 21 PR runs at 48 min because failures are only discoverable after pushing; the Makefile already has the pieces
  - Files: Makefile
- [ ] **T5 (P2, human: ~1h / CC: ~10min) — ci** — Fold openapi lint and actionlint into the proto job; pin @redocly/cli
  - Surfaced by: ceo-review — C4 + S6 — openapi job is 57 billed min for 9s of work and actionlint is not installed although the plan assumes it
  - Files: .github/workflows/ci.yml
- [ ] **E8 (P2, human: ~30min / CC: ~10min) — ci** — T1: fix the turbo cache key and budget the 6 GB restore cost against the same 10 GB
  - Surfaced by: eng-review — E8, 8/10 — turbo-<os>-<sha> writes a fresh entry per run and evicts Go entries
  - Files: .github/workflows/ci.yml
- [ ] **E9 (P2, human: ~15min / CC: ~5min) — ci** — T1: skip the cache-size assertion when cache-hit != true so the first push is not red by construction
  - Surfaced by: eng-review — E9, 8/10 — a guard that fails on bootstrap is a guard someone deletes
  - Files: .github/workflows/ci.yml
- [ ] **T8b (P2, human: ~1h / CC: ~15min) — ci** — Make the contract lint run on contract changes: docs/** in paths-ignore hides docs/api/openapi.yaml
  - Surfaced by: eng-review — E7, 9/10 — LIVE DEFECT, not introduced here: a contract-only PR skips CI entirely, contradicting the spec-first canon
  - Files: .github/workflows/ci.yml, docs/api/openapi.yaml
- [ ] **E12 (P2, human: ~30min / CC: ~10min) — tooling** — T11: target <5 min WARM and document which gates make ci omits
  - Surfaced by: eng-review — E12, 7/10 — a cold-machine 5 min target gets met by deleting gates from the target
  - Files: Makefile
- [ ] **T5b (P2, human: ~15min / CC: ~5min) — ci** — T5: pin actionlint via ACTIONLINT_VERSION
  - Surfaced by: devex-review — DX-6 — the plan pins @redocly/cli citing the govulncheck@latest breakage, then adds actionlint unpinned in the same task
  - Files: .github/workflows/ci.yml
- [ ] **T12 (P2, human: ~2h / CC: ~15min) — docs** — Update deploy/README.md for the post-change pipeline: what runs where, why go vet is gone, how to read a failed deploy
  - Surfaced by: devex-review — DX-11 + Pass 4 — four changes to what CI does and zero documentation tasks
  - Files: deploy/README.md
- [ ] **T0b (P2, human: ~1h / CC: ~10min) — scripts** — T0: document required gh scopes, print the gh auth refresh -s user remedy on 403, and specify flags (window, repo, output)
  - Surfaced by: devex-review — DX-7 — this review hit that exact 403 on first use
  - Files: scripts/ci-minutes.sh
- [ ] **T9 (P3, human: ~15min / CC: ~5min) — ci** — Add apps/mobile to paths-ignore for the Go jobs
  - Surfaced by: ceo-review — C5 — a mobile-only change runs all 14 Go legs
  - Files: .github/workflows/ci.yml
- [ ] **T3 (P3, human: ~4h / CC: ~30min) — ci** — ETAPA 2: collapse the 14-leg matrix into 4 shards, with a test that fails if a go.work module has no shard
  - Surfaced by: ceo-review — C4 — per-job rounding costs ~400 min; deferred past 30-nov because it carries debt and little saving
  - Files: .github/workflows/ci.yml
- [ ] **T6 (P3, human: ~30min / CC: ~10min) — ci** — ETAPA 2: delete the ci aggregator job once shard names are fixed
  - Surfaced by: ceo-review — C4 — 58 billed min for a five-line if
  - Files: .github/workflows/ci.yml
- [ ] **T4 (P3, human: ~1d / CC: ~45min) — ci** — ETAPA 2: run only the shards the diff can affect; changed-file list read via env, never interpolated into run
  - Surfaced by: ceo-review — C5 + S3 — script-injection surface if PR-controlled text reaches a run block
  - Files: .github/workflows/ci.yml, scripts/affected-shards.sh
- [ ] **T4b (P3, human: ~30min / CC: ~10min) — ci** — ETAPA 2: add a [ci full] commit token / dispatch input that forces every leg
  - Surfaced by: devex-review — DX-9 — without an override, people push empty commits to defeat a wrong heuristic
  - Files: .github/workflows/ci.yml

