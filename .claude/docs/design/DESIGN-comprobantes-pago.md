# DESIGN — Comprobantes de pago desde la app (`module_payment_vouchers`)

- Estado: propuesta (2026-10-21) · Autor: architect · Decisión del dueño: final (ver "Alcance")
- Requiere ADR nuevo: **0040 — Comprobantes de pago: segunda escritura de un tutor** (extiende
  ADR 0010) y referencia ADR 0002 (nudo de pagos), 0015 (Cobranza), 0026 (correcciones),
  0027 (comprobantes de egreso), 0035 (día de Morelia).

## Alcance (decisión del dueño)

1. El tutor sube desde la app un comprobante (foto o PDF de transferencia) para un hijo, con
   importe y fecha de pago **opcionales**.
2. En web, la encargada ve los comprobantes **pendientes primero** dentro de Registro de pagos,
   abre la vista previa, confirma/captura el importe y registra el pago con el `RegisterDeposit`
   **existente** (mes completo → saldo 0; menos → parcial; recargos siguen corriendo).
3. Si registra manualmente buscando al alumno, el comprobante pendiente de ese alumno se
   **vincula solo** y queda aplicado (misma transacción).
4. Rechazar con motivo. El tutor ve `pendiente / aplicado / rechazado`.
5. **Invariante duro: nunca doble pago.** Un comprobante ↔ a lo más un depósito; aplicar uno ya
   vinculado se rechaza o es idempotente; aplicar por comprobante usa una llave de idempotencia
   derivada del comprobante.

## 1. Qué se reutiliza (comprobantes de egreso, T10 / ADR 0027)

| Pieza | Dónde | Se reutiliza así |
|---|---|---|
| Validación del archivo por sus bytes (JPEG/PNG/PDF), ≤ 5 MB | `services/finance/internal/domain/expense.go` (`MaxReceiptBytes`, `NewReceiptFile`) | Se **extrae** a `domain/receipt_file.go` (refactor sin cambio de conducta, tests existentes en verde) y lo usan egreso y comprobante. |
| Almacén privado | puerto `ports.ReceiptStore` (`internal/ports/expenses.go:91`), adaptador `internal/adapters/gcsreceipts/store.go`, cableado en `cmd/main.go:329` (`EXPENSE_RECEIPTS_BUCKET`) | **Mismo bucket**, prefijo `vouchers/<voucherId>/<random>`. Sin bucket ⇒ el upload responde 404 "no habilitado" como los egresos. (Renombrar la variable no vale la pena; se documenta.) |
| Subir el archivo como cuerpo crudo | `services/gateway/internal/handler/expenses.go:350` (`http.MaxBytesReader` 5 MB → 413) | Mismo patrón en el handler de familia. |
| Servir el archivo | `expenses.go:376` `GetReceipt`: `Content-Type` probado, `nosniff`, `Cache-Control: private, no-store`, CSP `receiptSandboxCSP`, `inline` | Idéntico para el archivo del comprobante (staff). |
| Web: abrir el archivo | `apps/web/app/expenses/api.ts:295` `getReceiptBlob` + `URL.createObjectURL` (`ExpenseDrawer.tsx:196`) | Vista previa: `<img>` para imagen, `<iframe sandbox>`/nueva pestaña para PDF, siempre desde un Blob con sesión (no hay URL pública). |
| Transporte bytes por Connect | `AttachExpenseReceiptRequest.data bytes` (finance.proto:1586); Connect sin el tope de 4 MB de grpc-go | Igual. |

## 2. Cómo cobra hoy (y dónde se engancha)

- `usecase.FinanceService.RegisterDeposit` (`internal/usecase/deposit.go:104`): `paymentDate` (ADR
  0035: `paid_on` = día de Morelia, futuro rechazado) → `prepareDeposit` → `repo.RegisterDeposit`.
- `postgres.Repository.RegisterDeposit` (`internal/adapters/postgres/repository.go:1192`): dentro de
  `withTenant` (SET LOCAL tenant): `replayOf(key)` → con llave, `lockedChargesOf` + `replayOf` de
  nuevo (doble clic) → `storeDeposit` (cargos `FOR UPDATE` por fecha de vencimiento, plan, folio,
  asignaciones) → outbox. Índice `uq_deposits_idempotency (tenant_id, idempotency_key) WHERE <> ''`
  (`migrations/finance/0007_deposits.sql:59`).
- `PreviewDeposit` (`deposit.go:142`) proyecta sin escribir.
- `VoidDeposit` (`repository.go:1981`) bloquea depósito → cargos → libera. `CorrectDeposit`
  (`repository.go:2123`, ADR 0026) reemplaza un depósito por otro (`superseded_by`).
- Web: `apps/web/app/_screens/payments/PaymentsScreen.tsx` ("Registro de pagos": buscador →
  `CollectDrawer`), `apps/web/app/_screens/finance/CollectDrawer.tsx` (`registerDeposit` en :357,
  `newAttemptKey()` de `_design/payments/depositRequest.ts:8`, maneja 409 CC-202).
- Gateway: `POST /api/v1/finance/deposits` = `paymentsWrite` (finance:write | payments:write),
  `preview` y lecturas = `collectionsRead` (`services/gateway/cmd/main.go:715-746`).

**Enganche:** el comprobante entra al MISMO `RegisterDeposit` (no hay RPC paralelo de cobro). Se
añaden dos campos al request y el bloqueo/vínculo del comprobante ocurre dentro de la misma
transacción de `repo.RegisterDeposit`, después de `lockedChargesOf` (orden de locks: cargos →
comprobantes por id; igual en Void/Correct para no hacer deadlock).

## 3. Modelo de datos — `migrations/finance/0032_payment_vouchers.sql`

```sql
CREATE TABLE payment_vouchers (
  id uuid PRIMARY KEY, tenant_id uuid NOT NULL,
  student_id uuid NOT NULL,
  uploaded_by uuid NOT NULL, uploaded_by_name text NOT NULL,       -- el tutor (principal)
  claimed_amount_cents bigint NULL CHECK (claimed_amount_cents IS NULL OR claimed_amount_cents > 0),
  claimed_paid_on date NULL,                                        -- día civil (ADR 0035)
  file_key text NOT NULL CHECK (file_key <> ''),
  file_content_type text NOT NULL CHECK (file_content_type IN ('image/jpeg','image/png','application/pdf')),
  file_size_bytes integer NOT NULL CHECK (file_size_bytes > 0 AND file_size_bytes <= 5242880),
  file_sha256 bytea NOT NULL CHECK (octet_length(file_sha256) = 32),
  status text NOT NULL CHECK (status IN ('pending','applied','rejected')),
  deposit_id uuid NULL REFERENCES deposits(id),
  decided_by uuid NULL, decided_by_name text NULL, decided_at timestamptz NULL,
  rejection_reason text NULL CHECK (rejection_reason IS NULL OR char_length(btrim(rejection_reason)) BETWEEN 3 AND 300),
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
  CONSTRAINT payment_vouchers_shape_ck CHECK (
    (status = 'pending'  AND deposit_id IS NULL AND rejection_reason IS NULL AND decided_at IS NULL) OR
    (status = 'applied'  AND deposit_id IS NOT NULL AND rejection_reason IS NULL AND decided_at IS NOT NULL) OR
    (status = 'rejected' AND deposit_id IS NULL AND rejection_reason IS NOT NULL AND decided_at IS NOT NULL))
);
-- EL invariante: un depósito tiene a lo más un comprobante (y por la columna, uno a lo más un depósito).
CREATE UNIQUE INDEX uq_payment_vouchers_deposit ON payment_vouchers (deposit_id) WHERE deposit_id IS NOT NULL;
-- Reintento/doble envío del mismo archivo para el mismo alumno = el mismo comprobante.
CREATE UNIQUE INDEX uq_payment_vouchers_file ON payment_vouchers (tenant_id, student_id, file_sha256) WHERE status <> 'rejected';
CREATE INDEX ix_payment_vouchers_queue   ON payment_vouchers (tenant_id, created_at) WHERE status = 'pending';
CREATE INDEX ix_payment_vouchers_student ON payment_vouchers (tenant_id, student_id, status);

CREATE TABLE payment_voucher_events (           -- historial append-only (auditoría de dinero)
  id uuid PRIMARY KEY, tenant_id uuid NOT NULL, voucher_id uuid NOT NULL REFERENCES payment_vouchers(id),
  kind text NOT NULL CHECK (kind IN ('uploaded','applied','auto_linked','rejected','released_by_void','moved_by_correction')),
  deposit_id uuid NULL, actor_id uuid NULL, actor_name text NULL, reason text NULL, at timestamptz NOT NULL);
-- ENABLE + FORCE RLS y policy tenant_isolation en AMBAS, idéntico a 0024:142-145
-- (USING/WITH CHECK tenant_id = current_setting('app.current_tenant', true)::uuid).
```

Dominio (`internal/domain/payment_voucher.go`, sin GORM, inmutable como `Expense`):
`NewPaymentVoucher(...)`, `Apply(depositID, by, at)`, `Reject(reason, by, at)`,
`Release(at)` (vuelve a `pending`, por anulación), `MoveTo(depositID)` (por corrección).
Errores tipados: `ErrVoucherAlreadyApplied` (Conflict), `ErrVoucherRejected` (Conflict),
`ErrVoucherChoiceRequired` (Conflict), `ErrVoucherLimit` (Conflict), `ErrVoucherNotOwned` (NotFound).
Constante `MaxPendingVouchersPerStudent = 3`. Llave derivada:
`domain.VoucherIdempotencyKey(id) = "voucher:" + id`.

## 4. Reglas de vínculo (todas dentro de la transacción de `repo.RegisterDeposit`)

`RegisterDepositRequest` (finance.proto:1019) gana:
`string voucher_id = 13;` y `repeated string declined_voucher_ids = 14;`
`RegisterDepositResponse` gana `PaymentVoucher linked_voucher = 6;`.

**A. Cobro POR comprobante (`voucher_id` presente).**
1. El usecase **fuerza** `idempotency_key = "voucher:<id>"`; si el cliente manda otra llave
   distinta ⇒ `KindInvalid`. El método queda `transfer` salvo que la encargada lo cambie.
2. En tx: `replayOf("voucher:<id>")` ⇒ si existe, respuesta de replay (idempotente, no cobra).
3. `lockedChargesOf(student)` → `SELECT … FROM payment_vouchers WHERE id=? FOR UPDATE`.
   - no existe o `student_id` ≠ del depósito ⇒ NotFound (nunca se cruza de alumno).
   - `applied` ⇒ Conflict `VOUCHER_ALREADY_APPLIED` (lleva folio del depósito).
   - `rejected` ⇒ Conflict `VOUCHER_REJECTED`.
   - `pending` ⇒ `storeDeposit` → `UPDATE … SET status='applied', deposit_id=…` + evento `applied`.
4. El `uq_payment_vouchers_deposit` y el `uq_deposits_idempotency` son la red de la base: si
   todo lo de arriba fallara, la segunda escritura revienta y la tx entera se revierte.

**B. Cobro manual (sin `voucher_id`).** En tx, tras bloquear cargos:
candidatos = pendientes del alumno `FOR UPDATE`, excluyendo `declined_voucher_ids`.
- 0 ⇒ nada. **1 ⇒ se vincula** (evento `auto_linked`) y vuelve en `linked_voucher`.
- ≥ 2 ⇒ Conflict `VOUCHER_CHOICE_REQUIRED` sin cobrar nada; la web pide escoger.
- Nunca se mira otro alumno; nunca se empareja por importe ni por nombre.
- Un `declined` sigue `pending` (la encargada dijo "este pago es otro"); puede rechazarlo aparte.
- Finanzas no lee flags (FLAGS.md `module_scholarships`): si el flag se apaga con
  comprobantes pendientes, el vínculo automático **sigue** ocurriendo. Es deliberado: es lo que
  impide el doble pago cuando el flag vuelva a encenderse.

**C. Anular / corregir.** `VoidDeposit`: tras bloquear depósito y cargos, el comprobante
vinculado vuelve a `pending` (evento `released_by_void`) y reaparece en la cola. `CorrectDeposit`:
el comprobante se mueve al depósito nuevo (`moved_by_correction`) en la misma tx; así
`deposit_id` siempre apunta a un depósito vivo.

**D. Guardia humana adicional.** Al abrir un comprobante, la web muestra los depósitos del alumno
con `paid_on ≥ claimed_paid_on` (o desde la subida): "ya se registró $X el 21-oct (P-2026-…)".
Cubre el caso en que la encargada declinó el vínculo y luego aplica el comprobante.

## 5. Contrato

**Proto** (`proto/garcialorca/finance/v1/finance.proto`), mensaje `PaymentVoucher` (sin bytes) y:
- `CreatePaymentVoucher(student_id, data, claimed_amount_cents?, claimed_paid_on?, uploader_id, uploader_name)`
  — idempotente por sha256 (devuelve el existente, `replayed=true`); límite 3 pendientes.
- `ListPaymentVouchers(status?, student_ids[] , limit≤500)` — `student_ids` **obligatorio** en la
  ruta de familia (condición 2 de ADR 0010: el alcance se impone en el servicio); vacío = cola.
- `GetPaymentVoucherFile(id, student_ids[])` — mismo alcance.
- `RejectPaymentVoucher(id, reason, actor)` — sólo `pending`; repetir sobre `rejected` = idempotente.
- `RegisterDeposit` + `voucher_id`/`declined_voucher_ids` (arriba). `PreviewDeposit` acepta
  `voucher_id` sólo para prefijar (no bloquea).

**Gateway** (`services/gateway/cmd/main.go`; todas con `guard.Protect("finance")` +
`guard.Protect("payment_vouchers")` ⇒ OFF = 404 antes de auth):

| Ruta | Permiso | Notas |
|---|---|---|
| `POST /api/v1/family/students/{studentId}/payment-vouchers?amountCents=&paidOn=` (cuerpo crudo) | `vouchers_own:write` + `ownsChild` (404 ajeno, `handler/family.go:197`) | `MaxBytesReader` 5 MB → 413; `paidOn` futuro → 400; limitador por principal. |
| `GET /api/v1/family/students/{studentId}/payment-vouchers` | `family:read` + `ownsChild` | estado, importe, fecha, motivo de rechazo, folio si aplicado. |
| `GET /api/v1/finance/payment-vouchers?status=pending&studentId=` | `collectionsRead` | cola escolar (como `/finance/overdue`, ADR 0015: a propósito). |
| `GET /api/v1/finance/payment-vouchers/{id}/file` | `collectionsRead` | cabeceras de `GetReceipt`. |
| `POST /api/v1/finance/payment-vouchers/{id}/reject` | `paymentsWrite` | motivo obligatorio. |
| `POST /api/v1/finance/deposits` (existente) | `paymentsWrite` | con `voucherId`/`declinedVoucherIds`; con flag OFF el handler (`handler/finance.go:617`) responde 404 si vienen esos campos. |

`docs/api/openapi.yaml`: las rutas nuevas + `voucherId`, `declinedVoucherIds`, `linkedVoucher` y
los códigos 409 (`VOUCHER_ALREADY_APPLIED`, `VOUCHER_REJECTED`, `VOUCHER_CHOICE_REQUIRED`,
`VOUCHER_LIMIT`) en el sobre de error existente. Campos nuevos de respuesta **opcionales** hasta
que el productor esté en dev. Ejemplos con placeholders (GitGuardian).

**Permisos** — `migrations/iam/0033_payment_vouchers_permissions.sql`: crea `vouchers_own:write`
y lo otorga al rol `guardian` (segunda escritura de tutor ⇒ ADR 0040 con las 4 condiciones de
ADR 0010: alcance por vínculo, alcance en servicio, el importe del tutor nunca es dinero — sólo
prefija y la encargada confirma —, techos configurables). El personal reutiliza
`collections:read` / `payments:write` (Cobranza ya los tiene, iam 0021) y `finance:*` (dueño).

**Flag** `module_payment_vouchers`, default OFF: `featureflag.Key("payment_vouchers")` en
`services/gateway/internal/config/flags.go` (`AllModuleFlags`) + `flags_registry_test.go`,
`packages/ui/src/flags/types.ts`, fila en `.claude/docs/FLAGS.md`, `ENABLED_MODULES` (env, no
base). Móvil: `apps/mobile/src/flags/bootstrap.ts` es semilla de build ⇒ encenderlo para
familias exige un build nuevo (mismo caso que `module_student_card`).

## 6. Superficies

**Web (Registro de pagos)** — `PaymentsScreen.tsx`: con el flag, **arriba del buscador** una
sección "Comprobantes por revisar (N)", lista más antiguo primero —la API responde del más nuevo al más viejo (b8ce6b79, pensado para la familia) y la cola del personal la invierte en el cliente— (alumno, grupo, importe
declarado, fecha, hace cuánto). Clic ⇒ abre `CollectDrawer` del alumno con prop
`voucher` : vista previa (Blob), importe prellenado con `claimedAmountCents` (o "Debe hoy" si no
vino), `paidOn` = `claimedPaidOn`, método transferencia; botón primario "Registrar pago" (envía
`voucherId`) y secundario "Rechazar" (motivo, 1 campo ⇒ inline en el cajón). En la lista de
búsqueda, cada alumno con pendiente lleva chip "Comprobante" (dato de la cola ya cargada, sin
N+1). En `CollectDrawer` sin `voucher` pero con pendientes del alumno: aviso "Este alumno subió
un comprobante — ¿este pago es ése?" (1: casilla marcada por omisión ⇒ sin `declined`; ≥ 2:
selector obligatorio). Recibo muestra "Comprobante vinculado". Formularios según
`~/.claude/rules/ui-forms-and-modals.md` (botón primario nunca deshabilitado).

**Móvil** — host: `apps/mobile/src/screens/AccountScreen.tsx`, debajo de `HowToPayCard`
(`screens/account/HowToPayCard.tsx`, donde está la CLABE): tarjeta "Ya pagué — subir
comprobante" + lista de comprobantes con `Chip` de estado. Cliente en `src/api/family.ts`
(`apiFetch` con Bearer y refresh) + hook en `state/familyData`. Con 404 del gateway la tarjeta no
aparece (el flag del front es UX).

**Dependencias nativas:** hoy hay `expo-file-system`, `expo-sharing`, `expo-clipboard`; **no hay
picker**. Hace falta **`expo-image-picker`** (Fotos: ahí viven las capturas de la app del banco)
y **`expo-document-picker`** (PDF). Ambos son módulos nativos ⇒ **un APK/EAS build nuevo**
(también necesario para el flag). Pedir permiso de fotos con texto en `app.config.ts`. Comprimir
la imagen (`quality` < 1 ⇒ JPEG, evita HEIC y > 5 MB). Consultar Context7 (SDK 56) antes de
fijar la API de ambos y de la subida binaria.

## 7. Rebanadas (cada una un PR; test que falla primero)

1. **finance dominio + refactor** — extraer `ReceiptFile`; `PaymentVoucher` y transiciones.
   Tests primero: tabla de transiciones (pending→applied/rejected; applied→applied = error;
   rejected→applied = error; Release/MoveTo), techos, `VoucherIdempotencyKey`.
2. **finance datos** — migración 0032 + repo. Tests primero (testcontainers): RLS tenant A no
   ve/escribe B (ambas tablas); `uq_payment_vouchers_deposit` rechaza el 2º vínculo; dedupe sha256;
   CHECK de forma.
3. **finance cobro con comprobante** — proto + usecase + repo dentro de `RegisterDeposit`, Void y
   Correct. Tests primero (integración): mismo comprobante aplicado dos veces **en paralelo** ⇒ un
   depósito; replay con la llave derivada; aplicado manual→luego por comprobante ⇒ 409; auto-link
   con 1, 409 con 2, `declined` respetado; nunca otro alumno; void ⇒ `pending`; correct ⇒ se
   mueve; parcial deja saldo y recargo como hoy (`deposit_rules_integration_test.go`).
4. **finance upload/cola/rechazo + iam 0033** — RPCs restantes, alcance `student_ids`
   obligatorio. Tests: servicio niega comprobante fuera del alcance; límite 3; rechazo idempotente.
5. **gateway + OpenAPI + flag** — rutas, `ownsChild`, 413/400/404, flag OFF ⇒ 404 (suite ON/OFF),
   contract tests contra `openapi.yaml`, cabeceras del archivo, `voucherId` rechazado con flag OFF.
6. **web** — `finance/api.ts` + cola en `PaymentsScreen` + `CollectDrawer` (prop `voucher`, aviso,
   selector, manejo de 409 nuevos). Vitest/RTL primero: cola antes del buscador; prellenado;
   envía `voucherId`; con 2 pendientes no deja enviar sin escoger; flag OFF invisible.
7. **móvil** — deps + tarjeta + lista. Jest primero (picker mockeado, 413/409 con mensaje humano)
   **y** verificación del bundle real (`bundling.test.ts` + Metro), luego EAS build.
8. (opcional) **aviso al tutor** al aplicar/rechazar vía outbox → comms push.

Compuertas tech-lead + seguridad antes de encender el flag en dev.

## 8. Riesgos

- **Doble pago (dinero):** cubierto por llave derivada + `FOR UPDATE` + dos índices únicos +
  409 en ambigüedad + guardia humana D. Residual: la encargada declina un vínculo correcto y
  luego aplica el comprobante ⇒ D lo muestra; no se puede cerrar sin juicio humano.
- **Vínculo automático equivocado** (efectivo de otra cosa con comprobante de transferencia
  pendiente): el comprobante queda oculto como aplicado. Mitigación: casilla visible en el cajón y
  `linked_voucher` en el recibo; arreglo = corrección/anulación (C).
- **Authz:** segunda escritura de tutor ⇒ ADR 0040; 404 nunca 403 para hijo ajeno; alcance
  también en finanzas, no sólo en el borde. La cola escolar es lectura amplia de datos
  financieros: queda en `collections:read` por el mismo argumento que `/finance/overdue`.
- **PII / datos financieros:** el comprobante trae nombre, banco, cuenta parcial y referencia del
  tutor. Bucket privado, sin URL pública, `no-store`, CSP sandbox, nunca en logs (`pkg/gormlog`
  para GORM; no loguear `claimed_*` ni el nombre del archivo). Definir retención: rechazados se
  borran del bucket a los 90 días (lifecycle por prefijo `vouchers/`), aplicados se conservan con
  el depósito. Aviso de privacidad LFPDPPP pendiente (memoria prep-produccion) — bloqueante para
  producción, no para dev.
- **Abuso de subida:** 5 MB, tipo por bytes, 3 pendientes por alumno, dedupe sha256, limitador por
  principal (el limitador del gateway hoy es global, no por IP: no reutilizarlo tal cual).
- **Archivo malicioso:** PDF/imagen servidos con `nosniff` + CSP sandbox desde el gateway; el
  navegador de la encargada no ejecuta nada del archivo.
- **Móvil:** build nativo nuevo (picker); sin él la función no llega a familias aunque el flag del
  gateway esté ON.
- **Huérfanos en el bucket:** subir el archivo antes de la fila (como `AttachReceipt`,
  `usecase/expense.go:270-284`) y borrar el objeto si la fila falla.
