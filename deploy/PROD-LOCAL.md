# Levantar el stack en modo producción

Todo el sistema en esta Mac **como corre en Cloud Run**: las mismas imágenes que
construye `deploy/cloudbuild/cloudbuild.yaml` (cada `services/<svc>/Dockerfile`,
distroless, estático, sin símbolos; el web como `next build` standalone, sin
source maps), con variables de entorno de producción, contra la Postgres local y
con las migraciones reales.

Sirve para lo que el stack dev (`make run`, `air` + `next dev`) no puede decir:
si **la imagen** arranca, si el bundle de producción funciona, si una sesión
sobrevive un reinicio, si el web standalone encuentra su CSS.

Archivos: `docker-compose.prod-local.yml` · `scripts/prod-local.sh` ·
`scripts/mobile-release-apk.sh`.

## Comandos

```sh
make prod-build     # construye las 10 imágenes, UNA POR UNA (~10-15 min en frío)
make prod-up        # construye si faltan, detiene la app dev, migra, arranca y sondea
make prod-status    # sondea cada puerto; sale con 1 si alguno no responde
make prod-logs      # logs (scripts/prod-local.sh logs gateway para uno solo)
make prod-down      # baja el modo producción y vuelve a arrancar la app dev
make prod-apk       # APK release de la app de familias (ver abajo)
```

Tras cambiar código: `make prod-build && make prod-up` (`up` sólo construye
cuando falta alguna imagen; no detecta imágenes viejas).

Las imágenes se construyen **en secuencia a propósito**: en paralelo el daemon de
Docker corta con `unexpected EOF` y además es más lento (lo mismo que en Cloud
Build, PR #72). Si `gen/go` está vacío, el script corre `buf generate` con la
imagen fijada `bufbuild/buf:1.50.0` (no hace falta buf en el host).

## Puertos

| Qué | Puerto | Nota |
| --- | --- | --- |
| web (Next standalone) | **13000** | `http://localhost:13000` — el navegador sólo habla con este origen |
| gateway | **18080** | `0.0.0.0`: el emulador Android llega por `http://10.0.2.2:18080` |
| iam · students · finance · grades | 18091 · 18092 · 18093 · 18094 | sólo `127.0.0.1`, para los sondeos |
| attendance · comms · uniforms · config | 18095 · 18096 · 18099 · 18101 | ídem |
| Postgres | la del stack dev (`POSTGRES_HOST_PORT` del `.env`, hoy 5433) | la base es UNA |

`billing` y `docs` no existen en producción (no tienen Dockerfile ni se
despliegan), así que tampoco aquí.

**Salud = puertos, nunca `docker ps`.** `status` y `up` piden `/healthz` a cada
servicio (200), `/readyz` al gateway (que llama a iam de punta a punta) y
`/healthz` al web. `up` espera hasta 150 s y, si algo no responde, imprime el
estado y las últimas líneas del log de cada servicio caído, y sale con error.

## Por qué está armado así (léelo antes de cambiarlo)

- **Proyecto compose aparte** (`garcia-lorca-prod`): sus contenedores, red y
  nombres son suyos. `up`/`down` no recrean nada del proyecto dev (`garcia-lorca`).
- **Una sola base.** Los datos viven en la Postgres del stack dev (volumen
  `garcia-lorca_pgdata`), a la que se llega por su puerto publicado
  (`host.docker.internal`). Este compose **no declara** postgres: dos Postgres
  sobre el mismo volumen lo corrompen. Si la Postgres dev está apagada, `up` hace
  `docker start garcia-lorca-postgres-1`; si no existe, pide crearla una vez
  (`docker compose up -d postgres`).
- **Fuera de la red dev.** Unirse a `garcia-lorca_gl` registraría dos veces los
  nombres `iam`, `students`… y un contenedor dev arrancado después contestaría la
  mitad de las llamadas del gateway sin que nada lo avise.
- **Pub/Sub y fake-gcs propios**, en memoria, como en dev.
- **La app dev se detiene mientras corre** (los 12 contenedores de app, no la
  Postgres). Dos `finance`/`comms` sobre la misma base, cada uno con su
  emulador, se reparten el outbox y los eventos se pierden. `up` anota cuáles
  detuvo en `.prod-local/stopped-dev-containers` y `down` los vuelve a arrancar.
  Para regresar a dev sin `down`: `docker start garcia-lorca-<svc>-1`.

## Variables de entorno (igual que producción, salvo lo marcado)

- `APP_ENV=prod-local`: con cualquier valor distinto de `local`, iam **se niega
  a arrancar** sin `IAM_JWT_SECRET`, igual que en Cloud Run.
- `IAM_JWT_SECRET` y `LEVEL_DELETION_TOKEN_KEY` **estables**, generados una sola
  vez por el script en `.env.prod-local` (gitignored, `chmod 600`, nunca se
  imprimen). Un reinicio no saca a nadie de su sesión. Para rotarlos: borrar el
  archivo (todos vuelven a iniciar sesión).
- `ENABLED_MODULES`: se lee de `deploy/terraform/services.tf` (lo que terraform
  da al gateway de dev) **más** `ui_refresh,announcements_view`, que dev tiene
  encendidos por `gcloud run services update` (12 y 13-oct-2026) y terraform aún
  no lista. `PROD_LOCAL_EXTRA_MODULES=""` para usar sólo terraform;
  `PROD_LOCAL_ENABLED_MODULES=<lista>` para fijarla a mano. El script la imprime
  al arrancar; verificar con `curl localhost:18080/api/v1/features`.
- `SCHOOL_TIMEZONE=America/Mexico_City` explícito; `DB_MAX_OPEN_CONNS=2`,
  `DB_MAX_IDLE_CONNS=1` como en terraform.
- Web: imagen construida con `NEXT_PUBLIC_API_URL=/` (mismo origen, igual que
  Cloud Build) y `GATEWAY_URL=http://gateway:8080` leído por petición.
- Las contraseñas de la base salen del `.env` del stack dev (`APP_DB_PASSWORD`,
  `POSTGRES_PASSWORD`). Nada de esto se commitea.

## Credenciales

- Sólo datos **locales**. El director sembrado localmente es
  `director@garcialorca.edu.mx`; su contraseña local está en la memoria del
  proyecto, no en el repo. Si la base está recién creada, el seed del owner usa
  `OWNER_TEMP_PASSWORD` del `.env` y pide cambiarla al entrar.
- Inicio de sesión **por contraseña** (gateway → iam). Identity Platform queda
  apagado en la imagen web (`NEXT_PUBLIC_AUTH_PROVIDER` vacío) y en iam.
- Nunca se apunta este stack a una base compartida: el `DATABASE_URL` se arma
  siempre con `host.docker.internal` y el puerto local.

Prueba rápida:

```sh
curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:13000/api/v1/auth/login \
  -H 'content-type: application/json' \
  -d '{"email":"director@garcialorca.edu.mx","password":"<contraseña local>"}'   # 200
curl -s -o /dev/null -w '%{http_code}\n' localhost:13000/                        # 200
```

## Source maps: qué se revisa

- Al construir: el bundle de cliente de la imagen (`.next/static`, `public/`) no
  puede traer `*.map` ni `sourceMappingURL` (mismo alcance que el gate de CI,
  `apps/web/scripts/check-no-sourcemaps.mjs`). Next sí deja mapas **del
  servidor** en `.next/server`; nunca se mandan al navegador y el script sólo los
  cuenta. Están igual en la imagen de Cloud Run.
- Al levantar: por HTTP, cada chunk JS que referencia `/login` no lleva
  `sourceMappingURL`, y pedir `<chunk>.js.map` no devuelve un mapa. No devuelve
  404: la ruta comodín `app/[[...slug]]` contesta **cualquier** ruta desconocida,
  un asset inexistente incluido, con el HTML de la app y un 200. Por eso se revisa
  el cuerpo, no el código.

## Vaciar la base LOCAL

```sh
pnpm demo:reset        # = sh scripts/reset-demo-db.sh
```

Vacía todas las tablas operativas y deja `users`, `roles`, `user_roles`,
`user_invitations` y `schema_migrations` (para que el director pueda entrar y no
se re-apliquen migraciones). Por defecto va por `docker exec` al contenedor
`garcia-lorca-postgres-1`, que es justo la base que usa este stack, así que
funciona igual con el modo producción arriba; no hace falta reiniciar nada.

**Guarda:** si `PGHOST` está definido, el script **se niega** salvo que se pase
`--i-know-this-wipes-remote`. Es a propósito tonto: el Cloud SQL Auth Proxy hace
que una base remota parezca `127.0.0.1`, así que la dirección no basta para saber
dónde estás. Con este stack nunca hace falta `PGHOST`.

Ojo: `pnpm demo:verify` (Playwright) también vacía la base (`globalSetup`) salvo
con `E2E_SKIP_RESET=1`.

## App de familias: APK release en el emulador

```sh
make prod-apk                                   # sólo construye
scripts/mobile-release-apk.sh --install         # + arranca el AVD, instala y abre
```

Hace `expo prebuild --platform android --clean` (la carpeta `android/` es
generada y está en `.gitignore`) y `./gradlew assembleRelease` para `arm64-v8a`
(el ABI del AVD). Es un build release real: Hermes, bundle minificado, sin menú
de desarrollo ni Metro, y el script falla si el APK trae un `.map`. Queda en
`.prod-local/garcia-lorca-family-release.apk`.

Desviaciones locales, aplicadas sólo a lo generado:

- `EXPO_PUBLIC_API_URL=http://10.0.2.2:18080` (el emulador ve el loopback de la
  Mac como `10.0.2.2`). Para un teléfono físico: la IP LAN de la Mac, que cambia.
- `android:usesCleartextTraffic="true"`: Android bloquea `http` en release y el
  gateway local no tiene TLS. Producción usa `https`.
- Firmado con la llave **debug** de la plantilla (instalable en el emulador). Un
  build de tienda va por EAS con la llave real.
- Id de aplicación **local** `mx.edu.garcialorca.familias.local`
  (`ANDROID_PACKAGE`); el id definitivo de la tienda lo decide el dueño.

Requisitos: **JDK 17 exacto** (`brew install openjdk@17`, sin sudo; el script lo
toma solo si no hay `JAVA_HOME`). El JBR de Android Studio es 21 y NO sirve: el
plugin de Gradle de React Native pide una toolchain 17, Gradle intenta bajarla con
foojay-resolver 0.5.0 y éste truena en Gradle 9 (`JvmVendorSpec does not have
member field IBM_SEMERU`). Además: Android SDK en `~/Library/Android/sdk` (o
`ANDROID_HOME`) y el AVD `Medium_Phone_API_36.1` (arm64).

Plan B si el build nativo no se puede: Expo Go con el bundle de producción,

```sh
cd apps/mobile && EXPO_PUBLIC_API_URL=http://10.0.2.2:18080 pnpm exec expo start --no-dev --minify
```

y abrirlo en Expo Go del emulador. No es un APK release (corre en el runtime de
Expo Go), pero sí ejecuta el JS minificado sin modo dev.

## Diferencias que quedan contra producción real

| Aspecto | Producción (dev GCP) | Aquí |
| --- | --- | --- |
| Cómputo | Cloud Run: escala a cero, arranques en frío, `max_instances`, CPU sólo por petición | Contenedores siempre arriba; nunca hay arranque en frío |
| Servicio a servicio | `*.run.app` + OIDC con `roles/run.invoker` | HTTP plano en la red de Docker, sin tokens |
| Base | Cloud SQL / Neon con TLS, `setup_app_role.sql`, migraciones por `scripts/migrate-cloud.sh` | Postgres 16 local sin TLS; mismo ledger `schema_migrations` vía `docker/apply-migrations.sh` |
| Identidad | Identity Platform (Firebase) en el web de dev | Contraseña local por gateway → iam |
| Pub/Sub | Real, en modo **push** (`PUBSUB_PUSH_MODE=true`, OIDC del SA de push) | Emulador en modo **pull**; `comms` se suscribe al arrancar |
| Push a teléfonos | Expo Push Service (FCM) si `PUSH_PROVIDER=expo` + `EXPO_ACCESS_TOKEN` | `PUSH_PROVIDER=log`: sólo se registra en el log |
| Correo | Relé SMTP (cuando exista la credencial) | Sin relé: la invitación devuelve el enlace para pasarlo a mano |
| Archivos | Buckets GCS privados | fake-gcs en memoria: fotos y comprobantes se pierden al reiniciar |
| Secretos | Secret Manager | `.env` + `.env.prod-local` (gitignored) |
| Programador | Cloud Scheduler (cumpleaños, etc.) | No hay cron; los jobs se disparan a mano |
| Borde | HTTPS de Google, `TRUST_PROXY` apagado, limitador global | HTTP en localhost; mismo limitador |
| App móvil | Build de EAS firmado, `https` | APK release firmado con llave debug, `http` a 10.0.2.2 |
