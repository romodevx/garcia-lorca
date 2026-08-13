# PLAN — Auditoría y reemplazo de datos mock/estáticos en el frontend

> **Objetivo (petición original):** identificar TODOS los datos mock/estáticos en el
> front (web + mobile), reemplazar los que ya tienen backend real, y para los que no
> tienen backend crear un plan de acción (marcar como *pendiente por implementar* sin
> borrar la etiqueta/dato — para no perder qué va en ese lugar después).
>
> **Regla de oro de este plan:** ningún dato estático se elimina "a ciegas". O se
> reemplaza por una llamada real, o se conserva marcado explícitamente con
> `// TODO(pendiente-backend): <endpoint que faltaría>` + issue en backlog.

Fecha: 2026-08-12 · Rama: master · Fuente: auditoría de 3 frentes (web, mobile, backend).

---

## 1. Resumen ejecutivo

| App | Estado | Veredicto |
|-----|--------|-----------|
| **Web** (`apps/web`, Next.js) | **Casi limpio.** Todo dataset de dominio (alumnos, calificaciones, cargos, ciclos, roles, asistencia, KPIs del dashboard) ya se hace `fetch` vía `app/<módulo>/api.ts`; ningún `useState` sembrado con datos falsos. | 5 hallazgos, **1 relevante** (identidad del usuario logueado hardcodeada) + 4 menores (catálogos/labels estáticos). |
| **Mobile** (`apps/mobile`, Expo) | **Prototipo 100% estático.** No existe cliente API. Todo sale de `src/data/mock.ts` + bloques inline. Login es un `router.replace` no-op. | ~20 hallazgos, **todos user-visible**. Es el grueso del trabajo. |

**Conclusión:** la web está bien; el problema real vive en el **mobile**, que nunca fue
cableado al gateway. Además hay un **gap de backend**: el mobile muestra datos por-hijo
(calificaciones, asistencia, saldo) pero el gateway solo expone `/api/v1/family/notifications`
— la superficie `family`-scoped para el resto **no existe todavía**.

---

## 2. Mapa backend (qué se puede consumir HOY)

Servicios **substantivos y expuestos en el gateway** (datos reales + Postgres + RLS):
`iam`, `students`, `config`, `grades`, `attendance`, `finance`, `comms`.

**Sin backend (shells health-only, no hay RPCs ni rutas):**
`scholarships`, `docs`, `uniforms`, `billing`. · RPC stub: `finance.ExportContpaqi` (Unimplemented).

**Rutas `family`-scoped (rol `guardian` solo tiene `family:read`):**
- ✅ `GET /api/v1/family/notifications` — existe (agrega hijos + notificaciones).
- ❌ **NO existe** superficie family para grades / attendance / cuenta-saldo / perfil-hijo.
  Los servicios `grades`/`attendance`/`finance` existen pero sus rutas actuales exigen
  `grades:read`/`attendance:read`/`finance:read`, que el rol `guardian` **no** posee.
  → Cablear el mobile a esos datos **requiere backend nuevo** (rutas `/family/*`
  self-scoped), no solo frontend.

---

## 3. Categoría A — REEMPLAZAR YA (el backend existe y está expuesto)

### A-WEB (rápidos, alto valor)

| # | Archivo | Qué es | Reemplazo |
|---|---------|--------|-----------|
| A1 ✅ | `apps/web/app/AppShell.tsx` | **Identidad logueada hardcodeada** (`CHROME_ROLE="admin"` → "Director / Dirección"). `apiMe()` se llamaba pero se descartaba. | **HECHO.** El principal de `me()` ahora se captura (restore/login/post-change-password) y el TopBar muestra **email real** + nombre derivado del local-part. Nombre/rol legibles reales → B1. TDD RED→GREEN; 261/261 web green, typecheck+lint limpios. |
| ~~A2~~ | `apps/web/app/_screens/students/StudentCreatePanel.tsx:12-17` | `LEVEL_OPTIONS`. | **DESCARTADO — falso positivo.** No es catálogo de config: es un **enum de wire fijo** que el gateway mapea a `levelNames` en `services/gateway/internal/client/students.go` (nivel de texto libre = rechazado). Reemplazarlo rompería la creación de alumnos. Se conserva como enum legítimo (como los otros 6 enum-lists verificados). |

### A-MOBILE (requieren primero crear el cliente API — ver fase 0)

> **HECHO** (frontend-sr, TDD). Fase 0 cliente API creado (`apps/mobile/src/api/`: config, tokenStore, auth, client con refresh-on-401, notifications). 23→**50 tests mobile green**, typecheck+lint limpios. Contratos confirmados leyendo el gateway.

| # | Archivo | Qué es | Estado |
|---|---------|--------|--------|
| A3 ✅ | `LoginScreen.tsx` + `app/index.tsx` | Login no-op. | **HECHO.** POST real a `/auth/mobile/login` (`{accessToken,refreshToken,expiresIn,mustChangePassword}`), guarda Bearer (AsyncStorage), error inline, navega solo en éxito. Face ID ya no simula login. `mustChangePassword` se expone (pantalla de cambio → TODO). |
| A4 ✅ | `AnnouncementsScreen`/`AppState` | 5 avisos falsos. | **HECHO.** `GET /family/notifications` (loading/error+retry/empty/list). `announcements` mock eliminado (ya respaldado). |
| A5 ✅ | `HomeScreen` unread counter | Contador "no leídas". | **HECHO.** Derivado de datos reales (`unreadCount`). |

---

## 4. Categoría B — PENDIENTE POR IMPLEMENTAR (falta backend o exposición)

> Estos NO se borran del front. Se conservan con marca `// TODO(pendiente-backend)`
> y entran al backlog con su dependencia real.

### B-BACKEND (nuevo trabajo de servidor)

| # | Gap | Detalle | Consumidores mobile bloqueados |
|---|-----|---------|-------------------------------|
| B1 | **Perfil de usuario / display name** | `me()` retorna `{userId,email,permissions}` — sin nombre ni rol legible. Añadir campo(s) de perfil o RPC. | A1 (web identidad completa), `mock.ts:101` child/parent names. |
| B2 | **Superficie `family` para calificaciones** | Nueva ruta `GET /api/v1/family/students/{id}/grades` self-scoped (`family:read`), reusa `grades` service. | `mock.ts:143-169` (`gradesNum`/`gradesQual`), pantalla Grades. |
| B3 | **Superficie `family` para asistencia** | Nueva ruta `GET /api/v1/family/students/{id}/attendance`. | `mock.ts:116,124-141`, pantalla Attendance + `today`. |
| B4 | **Superficie `family` para estado de cuenta** | Nueva ruta `GET /api/v1/family/students/{id}/account` (reusa `finance.GetAccount`). | `mock.ts:177-189` (`balance`+movimientos), AccountScreen, `today.balance`. |
| B5 | **Perfil del hijo (family)** | Ruta que exponga datos del hijo + grupo/nivel/maestro para el guardián. Servicios `students`+`config` existen; falta ruta family. | `mock.ts:101-114` (`child`,`levels`), Home/Profile. |
| B6 | **Agenda / eventos** | Comms agenda estaba diferida (Phase 9d). No hay endpoint. | `mock.ts:230-234` (`events`), EventsScreen, `today.next`. |
| B6.5 | **Mark-read para guardián** (gap nuevo) | `POST /comms/notifications/{id}/read` exige `comms:write`; el rol `guardian` solo tiene `family:read`. El mobile marca leído solo en sesión (no persiste). Opción: dar al guardián un permiso scoped o ruta `family` de mark-read. | `notificationView.ts` (read state session-local + TODO). |
| B7 | **Catálogo de permisos enumerable** (opcional) | `admin/catalog.ts` duplica el taxonomy en cliente. Añadir endpoint que lo liste. Baja prioridad. | web `PERMISSION_CATALOG`. |

### B-SIN-BACKEND (servicios shell — mantener estático marcado)

| # | Archivo | Qué es | Acción |
|---|---------|--------|--------|
| B8 | `apps/mobile/src/screens/ModuleDetailScreen.tsx:12-16` (`DOCS`) | Lista de documentos. | Servicio `docs` = shell. Mantener mock + `TODO(pendiente-backend: docs)`. |
| B9 | `ModuleDetailScreen.tsx:18-22` (`INVOICES`) | Facturas CFDI. | `billing` fuera de MVP. Mantener mock marcado; idealmente ocultar tras flag OFF. |
| B10 | `ModuleDetailScreen.tsx:51-54` (uniformes tallas) | Tallas de uniforme. | `uniforms` = shell. Mantener marcado. |
| B11 | `ModuleDetailScreen.tsx:83-92` (becas) | "23% beca vigente". | `scholarships` = shell (sin RPCs). Mantener marcado. |
| B12 | `apps/mobile/src/sheets/PaySheet.tsx:23-27` | Método de pago tarjeta "•••• 4242". | Tokenización de tarjeta no construida. Mantener marcado; pago real usa `finance`. |

### B-CHROME (aceptable como estático — decisión de diseño)

| # | Archivo | Qué es | Acción propuesta |
|---|---------|--------|------------------|
| B13 | `apps/web/app/_design/data.ts:33-49` (`MODULES`) | Registro de módulos (nav chrome). Flags vienen de `/features` real. | Dejar estático (es navegación, no dominio). |
| B14 | `apps/web/app/_screens/Login.tsx:101` | "Ciclo escolar 2025–2026" (copy marketing pre-auth). | Muy baja prioridad. Opcional: `listCycles`. |
| B15 | `apps/mobile/.../GradesScreen.tsx:27` `LEVEL_OPTIONS`, `reasons` (mock.ts:244) | Labels/enums UI. | Dejar como enum, o derivar del nivel real del hijo cuando exista B5. |

---

## 5. Plan de acción por fases

**Fase 0 — Infra mobile (habilitador de todo A-MOBILE/B):** crear cliente API en
`apps/mobile` (fetch + Bearer + refresh + config de base URL), replicando el patrón de
`apps/web/app/**/api.ts`. TDD. Sin esto nada del mobile se puede cablear.

**Fase 1 — Reemplazos inmediatos (Categoría A):**
- A1, A2 (web) — pequeños, sin dependencia de backend nuevo (A1 parcial hasta B1).
- A3 (mobile login), A4/A5 (notificaciones) — una vez exista Fase 0.

**Fase 2 — Backend family-scoped (B1–B5):** rutas `/api/v1/family/*` self-scoped con
`family:read`, reusando servicios existentes. Orden sugerido: B1 (perfil) → B4 (saldo,
reusa `GetAccount`) → B3 (asistencia) → B2 (calificaciones) → B5 (perfil hijo). Cada una:
backend TDD → cablear pantalla mobile → quitar el mock correspondiente.

**Fase 3 — Módulos sin backend (B6, B8–B12):** decisión producto: (a) implementar
servicio, o (b) ocultar la superficie tras su flag OFF hasta que exista. Mientras tanto,
mock **marcado**, nunca borrado.

**Fase 4 — Limpieza chrome (B7, B13–B15):** opcional, baja prioridad.

---

## 6. Marcado de datos estáticos que se conservan

Para cada dato estático que NO se reemplaza en esta iteración, se aplica en el sitio:

```ts
// TODO(pendiente-backend: <servicio/ruta que faltaría>) — dato estático provisional.
// No borrar: define la forma/etiqueta esperada del dato real. Ver PLAN-mock-data-audit.md #<id>.
```

Así se cumple la restricción del usuario: *no quitar etiquetas o datos estáticos sin
saber qué va ahí después.*

---

## 8. Cierre de ejecución (branch `feature/wire-frontend-real-data`, 10 commits)

Todo el plan quedó ejecutado en una sola branch, TDD estricto, verificado en runtime.

**Reemplazos hechos + verificados:**
- **A1** identidad web real (`me()`) — commit `ee36b31f`; **drive runtime**: TopBar muestra "Director" + email real, no el estático.
- **B1** `/me` enriquece `displayName`+`role` (iam GetUser+ListRoles, degrada limpio) — `e13175ad`; runtime: `displayName:"Director", role:"owner"`.
- **Fase 0 + login + notificaciones mobile** — `7c45ad6c`; contrato `/auth/mobile/login` verificado.
- **Fixes ultrareview** (ciclo de sesión: hidratación de token en boot, fetch gated por auth, logout real, detalle de aviso, DRY, comentarios en inglés) — `5048f23f`.
- **bug_007** redirect a login en sesión muerta + **ocultar B6/B8–B12 tras flag** — `2787c872`.
- **B2–B5 + B6.5 + `/family/students`** superficie family self-scoped — `41674846`; **verificado runtime con guardián real**: children/profile/account/attendance/grades OK, mark-read persiste (`readAt`), **IDOR→404** en todas. Revisión de seguridad: limpia (sin IDOR/leak).
- **Enriquecer grades** subject/term names — `a79d433c`; runtime: `'Matemáticas'/'Bloque 1'=9.5`.
- **Fix config** `ListSubjects/ListTerms` empty→all (bug 22P02, encontrado en verificación runtime) — `d94fda67`.
- **Cablear mobile a `/family/*`** (account/attendance/grades/home/child + mark-read) — `4051005f`; 104 tests mobile green.

**Decisión de producto aplicada:** módulos sin backend (agenda B6, docs/becas/uniformes/facturas B8–B12) → **ocultos tras flag OFF / estado "Próximamente"**, no fabricados, mock conservado.

**Follow-ups documentados (no bloquean):**
- Grades cualitativas (kínder): backend solo tiene `value` numérico → pantalla muestra "Próximamente". Falta modelar cualitativo (logrado/proceso).
- Perfil: `currentEnrollment` trae ids de grupo/maestro, no nombres → muestra grupo, no maestro. Falta enriquecer nombres vía config.
- `config.ListGradeLevels(levelID)` tiene el mismo bug latente 22P02 con filtro vacío (sin caller vacío hoy; documentado).
- Seguridad INFO (preexistentes): multi-tenant readiness (gateway no propaga tenant por request; hoy tenant fijo) y grant-subset gap ([[vuln_authz_grant_subset]]) — ninguno introducido por este trabajo.
- Verificación: la **GUI del mobile no se ejecutó** (no hay simulador/Expo web en el entorno); se verificaron sus contratos de gateway en vivo + 104 tests unitarios.
- Seed de prueba en la DB de dev: usuario guardián `guardian.test@garcialorca.edu.mx` (rol guardian, hija María Reyes) para verificación runtime.

## 7. Inventario completo de hallazgos (trazabilidad)

- **Web:** 5 reales (A1, A2, B7, B13, B14) + 6 enum-lists verificadas legítimas + 1 dev-only (`DEMO_FLAGS`, correcto).
- **Mobile:** `src/data/mock.ts` (11 exports) + 6 bloques inline + 3 de auth/estado = ~20, todos user-visible.
- **Backend:** 7 dominios reales expuestos; 4 shells (scholarships/docs/uniforms/billing); 1 RPC stub (`ExportContpaqi`); 2 capacidades implementadas-pero-no-expuestas (`ChangeAdmissionStatus`, `Get*` single-item de config).
