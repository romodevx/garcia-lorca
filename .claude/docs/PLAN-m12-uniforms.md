<!-- /autoplan restore point: /Users/andresromo/.gstack/projects/romodevx-garcia-lorca/dev-autoplan-restore-20260824-193454.md -->
# PLAN — M12 Uniforms (tienda de uniformes: inventario, pedidos y despacho)

> Estado: revisado por `/autoplan` · Fase 1 (CEO) cerrada con gate de premisas contestado.
> Rama base: `dev`. Flag: `module_uniforms` (default OFF). Servicio: `services/uniforms`.

## 0. Decisiones del dueño (gate de premisas, 2026-08-24)

| # | Decisión | Efecto |
|---|---|---|
| **D1** | **Sí** hay venta en mostrador | El módulo captura el 100% del movimiento de stock. Sin esto el inventario miente en una semana |
| **D2** | **Sí** se permite entrega parcial | La máquina de estados baja del *pedido* a la **entrega**: un pedido puede partirse en varias entregas, cada una con su folio, su cobro y su plazo |
| **D3** | El calendario de días no hábiles vive en **`config`** | Se vuelve infraestructura compartida; asistencia lo reusará |
| **D4** | **No** se crea matrícula de alumno | El panel busca por **nombre de alumno** y **folio de pedido**. `students` no se toca |
| **D5** | Finanzas gana `external_ref` + **índices únicos parciales** | Desbloquea B1. La colegiatura conserva su idempotencia de "uno por ciclo"; el uniforme se identifica por referencia única. `OnConflict` de `CreateChargesBulk` recibe target explícito |
| **D6** | Entrega **por etapas**: almacén primero, app de padres después | Etapa 1 (~12 d) deja al colegio operando; el cambio riesgoso en Finanzas se valida temprano y con poca superficie encima |
| **D7** | La venta sin alumno genera cargo+pago contra una **cuenta genérica "Mostrador"** | El corte de caja cuadra siempre; deja de ser un agujero de trazabilidad |
| **D8** | **4 pestañas**: Pedidos · Mostrador · Inventario · Catálogo | Catálogo se usa 1 vez al año, Inventario cada semana, Mostrador a diario: ritmos distintos, lugares distintos |

## 1. Problema

El colegio vende uniformes. Hoy no hay sistema: no se sabe qué tallas hay, los padres
preguntan en persona, y los apartados se llevan en papel. Se necesita:

1. **Inventario por prenda × talla**, con la posibilidad (no obligación) de que el uniforme
   sea distinto por nivel (kínder / primaria / secundaria / preparatoria).
2. **Pedido del padre desde la app**: si hay talla se aparta; si no, queda como solicitud de
   surtido para que el colegio reponga.
3. **Estatus visible en todo momento** para el padre.
4. Cuando el colegio marca una entrega **disponible**, el padre tiene **10 días hábiles
   (configurable)** para pasar a **caja**. Mientras tanto la mercancía está **apartada**;
   el descuento real ocurre **al pagar**.
5. Si el padre no pasa, la entrega **se cancela sola** y la mercancía **regresa al inventario**.
6. **Panel web**: listado con filtro por nivel y búsqueda por nombre de alumno o folio,
   ordenamiento por fecha/estatus, y botón **Despachar** para cerrar.
7. **Venta en mostrador** (D1): el padre que llega sin pedido compra en el momento.

## 2. Premisas aceptadas

| # | Premisa |
|---|---------|
| P1 | El pago siempre es en caja del colegio. No hay pago en línea en este alcance |
| P2 | El cobro vive en **Finanzas** (mismo estado de cuenta del alumno), no en una caja paralela |
| P3 | Un pedido puede llevar varias prendas |
| P4 | *(revocada por D2)* Se permite entrega parcial |
| P5 | Días hábiles = lunes a viernes menos los días no hábiles del calendario escolar |
| P6 | El inventario lo administra el colegio; sin integración con proveedor |

## 3. Modelo de dominio

### 3.1 Catálogo e inventario

```
uniform_items                     (SKU de prenda)
  id, tenant_id, code, name, description,
  level          NULL = aplica a todos los niveles
                 ('kinder'|'primaria'|'secundaria'|'preparatoria')
  price_cents    int64 · entero, igual que finance
  active         bool
  created_at, updated_at

uniform_stock                     (una fila por prenda × talla)
  id, tenant_id, item_id, size_label ('4','6','CH','M','G'…), size_order
  on_hand        unidades físicas en almacén
  reserved       unidades comprometidas a líneas vivas
  min_stock      umbral de reorden (alerta en el panel)
  UNIQUE (tenant_id, item_id, size_label)
  CHECK (on_hand >= 0 AND reserved >= 0 AND reserved <= on_hand)

  available = on_hand - reserved          derivado, nunca almacenado

uniform_stock_moves               (bitácora inmutable, solo-inserción)
  id, tenant_id, stock_id, order_line_id NULL, kind, qty (±),
  actor_user_id, note, created_at
  kind ∈ ('receive'|'reserve'|'release'|'sell'|'adjust')
```

`size_label` es texto porque las tallas no son homogéneas (pantalón 4/6/8, suéter CH/M/G);
`size_order` fija el orden de despliegue.

### 3.2 Pedidos y entregas (modelo de entrega parcial · D2)

El **pedido** es el contenedor de lo que pidió la familia. La **entrega** es la unidad que se
surte, se cobra, vence y se despacha. Un pedido puede tener 1..N entregas.

```
uniform_orders                    (contenedor · sin estado propio)
  id, tenant_id, folio  'U-2026-000123'  (secuencia por tenant)
  student_id            UUID cross-service · validado por RPC · SIN FK
  student_name          snapshot al crear  ← el panel filtra/ordena sin salir del servicio
  student_level         snapshot al crear  ← ídem (evita fan-out, ver §7)
  student_grade_group   snapshot al crear  ← R8a: sin esto la columna nivel/grupo no se pinta
  cycle_id              del enrollment activo · requerido por el cargo
  created_by_user_id, source ('family'|'counter'|'staff')
  created_at, updated_at, closed_at NULL

uniform_order_lines
  id, tenant_id, order_id, item_id, item_name (snapshot), size_label,
  qty, unit_price_cents (snapshot · SIEMPRE del catálogo del servidor)
  status ('awaiting_stock'|'reserved'|'sold'|'released')
  fulfillment_id NULL                      ← se asigna al surtir

uniform_fulfillments              (entrega · aquí vive la máquina de estados)
  id, tenant_id, order_id, seq (1,2,3…)    folio derivado 'U-2026-000123-A'
  status, amount_cents (suma de sus líneas)
  finance_charge_id NULL
  ready_at, pickup_deadline, extended_days DEFAULT 0
  paid_at, dispatched_at, cancelled_at, cancel_reason
  created_at, updated_at
```

**Estatus derivado del pedido** (solo para pintar el panel; función pura del dominio):

| Líneas pendientes | Entregas vivas | Etiqueta |
|---|---|---|
| sí | no | `Por surtir` |
| sí | sí | `Parcial` |
| no | sí | `En proceso` |
| no | no | `Cerrado` |

### 3.3 Máquina de estados de la **entrega**

```
   Staff pulsa "Surtir" sobre las líneas que SÍ hay
                    │
                    ▼
             ┌─────────────┐   crea la entrega · la reserva ya está hecha
             │   PICKING   │   (estado transitorio · ver A1)
             └──────┬──────┘
                    │  finance.CreateCharge(idempotency_key = fulfillment_id)
                    ▼
          ┌────────────────────┐   ready_at = ahora
          │  READY_FOR_PICKUP  │   pickup_deadline = +N días hábiles
          └───┬─────────────┬──┘
              │             │
  PaymentRegistered      vence el plazo (barrido o lectura perezosa)
  (remaining == 0)       Y el cargo NO tiene pagos
              ▼             ▼
         ┌────────┐    ┌──────────┐
         │  PAID  │    │ EXPIRED  │  libera reserved · cancela el cargo
         └───┬────┘    └──────────┘  las líneas pasan a 'released'
             │
  "Despachar"│  (la venta en mostrador entra directo aquí)
             ▼
      ┌────────────┐
      │ DISPATCHED │   terminal · on_hand ya se descontó en PAID
      └────────────┘

   CANCELLED: manual desde el panel (staff, con motivo) o por el tutor
              mientras la entrega no exista todavía (líneas awaiting_stock).
   Prórroga:  READY_FOR_PICKUP + extended_days (staff, con motivo, máx 2 veces).
```

**Transiciones inválidas y qué las impide** (tabla de verdad en el dominio, probada):
`PAID → READY`, `DISPATCHED → *`, `EXPIRED → PAID`, `CANCELLED → *`, `PICKING → DISPATCHED`
y cualquier salto que se brinque el cobro. La transición se le pide al agregado; nunca se
escribe `status` desde el repositorio.

**Invariantes del inventario** (unitarias, sin base de datos):
- Solo las líneas `reserved` retienen `reserved`; `sold` y `released` no.
- `on_hand -= qty` ocurre **exactamente una vez**, en la transición a `PAID`.
- `EXPIRED` / `CANCELLED` liberan `reserved` **exactamente una vez** (idempotente).
- `reserved <= on_hand` siempre, garantizado por CHECK en la base **y** por el agregado.

### 3.4 Venta en mostrador (D1)

Una sola acción del panel: una transacción y dos RPC.

```
  POST /api/v1/uniforms/counter-sale
    { studentId?, lines[], method, reference }
        │
        ├─ TX: crea order(source='counter') + líneas + fulfillment(seq=1)
        │      reserva y VENDE en el mismo paso  (on_hand -= qty)
        ├─ finance.CreateCharge(idempotency_key = fulfillment_id)
        ├─ finance.RegisterPayment(chargeId, total, method, reference,
        │                          idempotency_key = fulfillment_id)
        └─ TX: fulfillment ── PAID ──► DISPATCHED
```

`studentId` es opcional: una venta suelta (un tío, un ex-alumno) crea el pedido sin alumno y
**sin cargo en finanzas** — queda como movimiento de inventario con nota. El caso con alumno sí
pasa por finanzas para que aparezca en su estado de cuenta.

Requiere `uniforms:write` **y** `finance:write`. Verificado: `finance:write` **no** es owner-only
(solo `finance:read` lo es — `services/iam/internal/domain/permission.go:41`), así que el colegio
puede crear un rol "Caja/Almacén".

### 3.5 Configuración

```
uniform_settings                  (una fila por tenant)
  tenant_id PK, pickup_window_business_days INT DEFAULT 10,
  auto_cancel_enabled BOOL DEFAULT true,
  max_open_orders_per_student INT DEFAULT 5,
  max_lines_per_order INT DEFAULT 20,
  updated_at
```

El calendario NO vive aquí (D3): vive en `config` como `school_calendar_days`
(`tenant_id, cycle_id, day, label, kind ('holiday'|'no_class')`), capturable desde la pantalla de
Configuración académica que ya existe. `uniforms` lo consulta por RPC y lo cachea en memoria con
TTL corto. Si `config` no responde, se degrada a lunes–viernes sin festivos y se registra un
warning visible en el panel ("plazo calculado sin calendario").

Aritmética de días hábiles: función pura
`businessdays.Add(from time.Time, n int, nonWorking map[string]bool, loc *time.Location) time.Time`,
con `loc = America/Mexico_City` y `now` inyectados. Cero dependencia del reloj real.

## 4. Integración con Finanzas (reusar, no duplicar)

- Al pasar a `READY_FOR_PICKUP`, uniforms llama `finance.CreateCharge` con el concepto
  **"Uniformes"**, `description = "Uniformes · pedido U-2026-000123-A"`,
  `amount_cents` = total de la entrega, `due_date = pickup_deadline`.
- **Cambio aditivo requerido en finanzas:** `idempotency_key` en `CreateChargeRequest`
  (el mismo mecanismo ya existe en `RegisterPaymentRequest`, `finance.proto:258`).
  Sin él, un reintento de "Surtir" cobra dos veces.
- Caja cobra en la pantalla de **Finanzas** que ya existe. No se construye caja nueva.
- Finance publica `PaymentRegistered`. **Cambio aditivo:** agregar `charge_remaining_cents` y
  `charge_status` al evento. uniforms solo transiciona a `PAID` cuando `remaining == 0`, para que
  un **pago parcial no libere mercancía**. Los consumidores viejos (comms) ignoran los campos
  nuevos: es aditivo y `buf breaking` lo permite.
- uniforms consume `payment-events` con el patrón de comms: ledger `processed_events` idempotente,
  en la misma transacción que la transición.
- Al vencer o cancelar: `finance.CancelCharge`, con la salvaguarda A3 (§12).

## 5. Contrato (proto)

`proto/garcialorca/uniforms/v1/uniforms.proto` reemplaza el stub de FASE 0:

```
service UniformsService {
  rpc Health

  // Catálogo e inventario (staff)
  rpc CreateItem / UpdateItem / ListItems
  rpc UpsertSizes           // alta rápida: lista o rango de tallas de una prenda (E5)
  rpc ReceiveStock          // entrada de almacén + asignación FIFO a líneas awaiting_stock
  rpc AdjustStock           // merma/corrección · exige nota · queda en la bitácora
  rpc ListStock

  // Pedidos y entregas
  rpc CreateOrder           // familia (vía gateway /family) y staff
  rpc CounterSale           // venta en mostrador (D1)
  rpc ListOrders            // filtros: level, q, status, from, to, sort, page
  rpc GetOrder              // pedido + líneas + entregas
  rpc PickLines             // líneas reservadas ──► nueva entrega (READY_FOR_PICKUP)
  rpc DispatchFulfillment   // PAID ──► DISPATCHED
  rpc CancelFulfillment     // libera reserved + cancela el cargo
  rpc CancelOrderLines      // cancela líneas aún no surtidas
  rpc ExtendDeadline        // prórroga con motivo (E3)
  rpc ExpireDueFulfillments // barrido idempotente (job) + reconciliación (A2)
  rpc ResumeStuckPicking    // termina entregas atoradas en PICKING (A1)

  // Configuración
  rpc GetSettings / UpdateSettings
}

// Eventos publicados en el topic `uniform-events`
UniformOrderPlaced · UniformBackorderFulfilled · UniformFulfillmentReady ·
UniformFulfillmentPaid · ReservationExpired
```

`ListOrders` no consulta a otro servicio: nombre y nivel viajan en snapshot (§3.2).

## 6. Arquitectura del servicio — desvío del layout "thin" (ADR 0007)

`CLAUDE.md` clasifica `uniforms` como layout **thin** (handler → service → repo). Ya no lo es:
tiene máquina de estados, invariantes de inventario, aritmética de días hábiles, asignación FIFO
y derivación de estatus. Se adopta **Clean Architecture completa**
(`internal/{domain,usecase,ports,adapters/{postgres,grpc,pubsub,financeclient,studentsclient,configclient}}`)
y se documenta en **ADR 0007**. "La arquitectura se gana": aquí se ganó. Sin el ADR esto es una
violación del canon, no una decisión.

## 7. Gateway — rutas, permisos y flag

Todas bajo `guard.Protect("uniforms", …)`: módulo OFF ⇒ **404**.

**Staff** — `uniforms:read` / `uniforms:write`, permisos **otorgables** (para que el colegio pueda
crear un rol "Caja/Almacén"):

| Método | Ruta | Permiso |
|---|---|---|
| GET · POST · PUT | `/api/v1/uniforms/items` | read / write |
| POST | `/api/v1/uniforms/items/{id}/sizes` (alta rápida, E5) | write |
| GET | `/api/v1/uniforms/stock` | read |
| POST | `/api/v1/uniforms/stock/receive` · `/adjust` | write |
| GET | `/api/v1/uniforms/orders` (`level`, `q`, `status`, `from`, `to`, `sort`, `page`) | read |
| GET | `/api/v1/uniforms/orders/{id}` | read |
| POST | `/api/v1/uniforms/orders/{id}/pick` (líneas → entrega; acepta lote, E4) | write |
| POST | `/api/v1/uniforms/fulfillments/{id}/dispatch` · `/cancel` · `/extend` | write |
| POST | `/api/v1/uniforms/counter-sale` | write **+ `finance:write`** |
| GET · PUT | `/api/v1/uniforms/settings` | read / write |

**Familia** — `uniforms_own:read` / `uniforms_own:write`, otorgados al rol `guardian` (mismo
precedente que `attendance_own:*`, ADR 0006). Cada ruta valida el vínculo tutor→alumno vía
`students` **antes de tocar nada**, y responde **404 (no 403)** ante un id ajeno:

| Método | Ruta |
|---|---|
| GET | `/api/v1/family/uniforms/catalog?studentId=` (prendas del nivel del hijo + disponibilidad) |
| POST | `/api/v1/family/uniforms/orders` (con `Idempotency-Key` del cliente) |
| GET | `/api/v1/family/uniforms/orders` (solo hijos vinculados) |
| GET | `/api/v1/family/uniforms/orders/{id}` |
| POST | `/api/v1/family/uniforms/orders/{id}/cancel` (solo líneas aún no surtidas) |

Migración `iam/0014`: alta de los 4 permisos + `uniforms:*` al owner + `uniforms_own:*` al rol
`guardian`.

> **Decisión de seguridad a ratificar en el gate de seguridad:** `uniforms:read` expone nombres de
> alumnos y su historial de compra de todo el colegio. Por el criterio de `ownerOnlyPermissions`
> sería candidato a owner-only; se deja **otorgable** porque el panel de almacén no puede funcionar
> sin él. Queda registrado como decisión consciente.

**Búsqueda `q`**: si matchea `^U-\d{4}-\d{6}(-[A-Z])?$` es folio; si no, es nombre de alumno
(`ILIKE`, acotado a 100 caracteres, placeholders de GORM).
**Filtro `level`**: sobre el snapshot `student_level` de la propia tabla, sin fan-out a `students`.

## 8. Panel web (`apps/web/app/_screens/uniforms/`)

> **SUPERSEDIDO EN PARTE por la Fase 2b (R1–R8).** Donde esta sección y la Fase 2b se
> contradigan, **manda la Fase 2b**: la fila es la entrega (no el pedido), no hay fila
> expandible, Mostrador es pestaña propia (no FAB), las stat cards son otras, y tabs = estado
> mientras chips = nivel.
>
> **Estructura de pestañas (4):** `Pedidos` · `Mostrador` · `Inventario` · `Catálogo`.
> El alta del inventario vive en dos de ellas: **Catálogo** crea la *prenda* (nombre, nivel o
> "todos", precio, alta rápida de tallas) e **Inventario** administra las *existencias*
> (prenda × talla con disponible/físico/apartado, tab "Bajo mínimo · N", y el drawer de entrada
> de almacén con una columna de cantidades por talla y un solo guardar).

Sigue el sistema de diseño obligatorio (`.claude/docs/design/DESIGN-SYSTEM.md`): header,
stat cards, filter tabs, tabla de datos, quick-add drawer, FAB. Mínimo número de clics.

- **Stat cards, en este orden** (lo que espera acción mía hoy, no el total histórico):
  *Por surtir* → *Por vencer (≤3 días)* → *Listas por recoger* → *Vencidas hoy*.
- **Filter tabs** por nivel: Todos · Kínder · Primaria · Secundaria · Prepa.
- **Buscador único** (nombre de alumno o folio) + filtro por estatus + rango de fechas.
- **Tabla**: folio · alumno · nivel/grupo · prendas · total · estatus (badge **color + texto**,
  nunca solo color) · fecha · **días hábiles restantes** con semáforo. Ordenable por fecha,
  estatus y vencimiento. Orden estable por `(created_at, id)` para que paginar sea determinista.
- **Fila expandible**: líneas del pedido y sus entregas (`-A`, `-B`), cada una con su plazo.
- **Acciones**: Surtir (selección múltiple de líneas y pedidos, E4) · Despachar · Cancelar ·
  Prorrogar · Venta en mostrador (FAB). Confirmación en lo irreversible; un botón deshabilitado
  dice **por qué** lo está.
- **Pestaña Inventario**: prenda × talla con `disponible / físico / apartado`, alerta bajo
  `min_stock`, drawer de entrada de almacén.
- **Pestaña Catálogo**: alta/edición de prenda (nombre, nivel o "todos", precio) + **alta rápida
  de tallas** (rango 4–16 o lista CH/M/G de un golpe, E5).
- **Estados obligatorios** en cada superficie: cargando (skeleton), vacío (con CTA), error (con
  reintento), sin permiso, y **parcial** — el estado que hace que este módulo se sienta terminado:
  pedido con una entrega lista y otra pendiente, y talla bajo mínimo.

El tile `uniforms` ya existe en `apps/web/app/_design/data.ts` (`flag: module_uniforms`,
`perm: uniforms:read`); solo hay que darle ruta real.

## 9. App familia (Expo, `apps/mobile`)

> **SUPERSEDIDO EN PARTE por la Fase 2b (R4–R6, R8h–R8i).** Manda la Fase 2b en: estados del
> móvil (reusar `AsyncLoading`/`AsyncErrorState`/`GatedScreen`/`EmptyState`), los tres
> tratamientos del selector de talla, el carrito que parte el total, las cadenas de la cuenta
> regresiva, el selector de hijo, y el reemplazo de la tarjeta mock de `MoreScreen`.

- Entrada **Uniformes** detrás de `module_uniforms` + `uniforms_own:read`.
- Catálogo por hijo (solo prendas de su nivel). El selector de talla **muestra la disponibilidad
  explícitamente**: "Disponible" vs "Se solicita al colegio"; nunca una talla deshabilitada sin
  explicación.
- Carrito simple, confirmación con total, y `Idempotency-Key` generada en el cliente para que un
  doble tap o una red intermitente no creen dos pedidos.
- **Mis pedidos**: folio, estatus en lenguaje humano, y cuando hay entrega lista:
  *"Pasa a caja antes del viernes 12 de septiembre · te quedan 4 días hábiles"*.
  Un pedido partido se ve como dos tarjetas bajo el mismo folio.
- **El estatus se obtiene siempre por pull.** El push es refuerzo, no el canal: FCM real todavía es
  un stub de log (deuda de la Fase 9b) y el padre no puede depender de él.

## 10. Eventos y notificaciones

uniforms publica en `uniform-events`; comms consume y notifica a los tutores del alumno (targeting
de la Fase 9b):

| Evento | Notificación al tutor |
|---|---|
| `UniformBackorderFulfilled` | "Ya llegó la talla que pediste" |
| `UniformFulfillmentReady` | "Tu pedido está listo. Pasa a caja antes del \<fecha\>" |
| `ReservationExpired` | "Tu pedido venció y se liberó" |

Se agrega `uniform-events` a `consumedTopics` de comms + plantillas de render.

## 11. Vencimiento con scale-to-zero

Tres capas, porque las dos primeras fallan de formas distintas:

1. **Perezosa (en lectura).** Cualquier lectura de una entrega `READY_FOR_PICKUP` con
   `pickup_deadline < hoy` **ejecuta el vencimiento dentro de una transacción**: libera `reserved`
   de verdad, no solo pinta "vencido". Si solo etiquetara, el stock quedaría bloqueado en silencio
   hasta que corriera el job.
2. **Barrido programado.** `POST /jobs/expire-fulfillments` en uniforms, invocado por **Cloud
   Scheduler** con OIDC. Ingress interno **+** verificación de que el email del invocador sea la SA
   del scheduler (misma postura endurecida que el push de Pub/Sub, `pkg/pubsub/gcp/push.go`).
   Terraform: diario 06:00 `America/Mexico_City`. Lotes de 100, una transacción por entrega.
3. **Reconciliación (A2).** El mismo barrido revisa las entregas `READY_FOR_PICKUP` cuyo cargo ya
   está saldado en finanzas y las cierra. Sin esto, un evento `PaymentRegistered` perdido deja
   mercancía pagada apartada para siempre. También destraba entregas atoradas en `PICKING` (A1).

## 12. Hallazgos de la revisión ya incorporados

| id | Hallazgo | Cómo queda resuelto |
|---|---|---|
| **C1** | Sin venta en mostrador el inventario miente en una semana | §3.4 (D1 aceptado) |
| **A1** | "Surtir" no es atómico con la creación del cargo (no hay 2PC) | Estado `PICKING` + `idempotency_key` en `CreateCharge` + `ResumeStuckPicking` |
| **A2** | Evento de pago perdido ⇒ mercancía pagada apartada para siempre | Reconciliación en el barrido (§11.3) |
| **A3** | `finance.CancelCharge` cancela **aunque el cargo tenga pagos** (verificado en `services/finance/internal/domain/charge.go:120-161`) | Antes de vencer se consulta el cargo: `paid_cents > 0` ⇒ **no vencer**. Arreglar `Cancel()` en finanzas queda **fuera de alcance**, reportado al dueño |
| **S2** | El padre podría mandar `unit_price_cents` y comprar barato | El precio se toma **siempre** del catálogo del servidor; el campo del cliente se ignora. Prueba de regresión obligatoria |
| **S9** | Un tutor podría crear pedidos sin límite | `max_open_orders_per_student` (5) y `max_lines_per_order` (20) + rate limit en la ruta family |
| **H2** | FCM es un stub: el padre no se entera de que el reloj arrancó | Estatus por pull en la app; push como refuerzo (§9) |
| **H4** | El vencimiento perezoso solo etiquetaba, no liberaba | Ahora libera dentro de transacción (§11.1) |
| **E3** | Sin prórroga, el staff cancelaría y recrearía pedidos, corrompiendo la bitácora | `ExtendDeadline` con motivo, máx. 2 veces |
| **E4** | 130 pedidos en agosto surtidos de uno en uno es inusable | Selección múltiple en el panel |
| **E5** | Hora 1: 6 prendas × 8 tallas = 48 formularios a mano | `UpsertSizes` por rango o lista |
| **P1** | Fan-out a `students` al filtrar por nivel | Snapshot de `student_name` / `student_level` en el pedido |

## 13. Rebanadas de entrega (D6: dos etapas · una PR por rebanada · TDD estricto)

### ETAPA 1 — el colegio opera (~12 días humano)

Sin app de padres, sin entrega parcial, sin apartado ni vencimiento. El pedido nace y muere en
mostrador. **Es donde vive el riesgo cross-service, así que se valida primero.**

| # | Rebanada | Contenido | Días |
|---|---|---|---|
| 12a | **Finanzas: `external_ref`** | migración con índices únicos parciales · `OnConflict` con target explícito · `idempotency_key` = `external_ref` en `CreateChargeRequest` · **`CancelChargeIfUnpaid`** (A3/B5, atómica bajo row lock) · `UpdateChargeDueDate` (B7f) · evento `ChargeSettled` (B7e) · concepto y cuenta genérica "Mostrador" (D7) · **ADR 0008** · test de regresión: re-correr `GenerateCharges` da `created == 0` | 3 |
| 12b | **Contrato + datos** | proto de uniforms · `migrations/uniforms/0001..0003` (7 tablas, RLS, `stock_moves` con políticas por operación B7k) · `uniform_folio_counters` (B7m) · `iam/0014` con **`uniforms:stock` otorgable + `uniforms:orders` owner-only** (B6) · `docs/api/openapi.yaml` del módulo (B7b) · **ADR 0007** (desvío del layout thin) | 2.5 |
| 12c | **Dominio + adaptadores** | catálogo, inventario, reserva por sentencia condicional (B7h), bitácora, `SOLD_AT_COUNTER` terminal (B4) · postgres + integración + RLS · handler gRPC · clientes finance/students | 3 |
| 12d | **Gateway + panel** | rutas staff + permisos partidos · 4 pestañas (Pedidos vacío, Mostrador, Inventario, Catálogo) · alta rápida de tallas · entrada de almacén por columna de tallas · **un campo, dos caminos** en Mostrador (R7) · command palette | 3 |
| 12e | **Terraform + gates** | módulo `cloud_run` de uniforms · `internal_services` · `ENABLED_MODULES` + `UNIFORMS_BASE_URL` · runner de migraciones · gate de seguridad + tech-lead · flag ON en dev | 0.5 |

**Al final de la etapa 1 el colegio ya puede:** cargar su catálogo y tallas, registrar entradas de
almacén, ver qué tiene y qué falta, y vender en mostrador con el cobro en Finanzas y el corte
cuadrado.

### ETAPA 2 — el padre pide desde la app (~18 días humano)

| # | Rebanada | Contenido | Días |
|---|---|---|---|
| 12f | **Calendario + entregas** | calendario escolar en `config` (migración + RPC + pantalla) · máquina de estados de la entrega con `released`/`cancelled` separados, reingreso y `expiry_count` (B3) · par atómico `on_hand -= qty; reserved -= qty` en `PAID` (B2) · días hábiles | 4.5 |
| 12g | **Outbox + jobs** | `uniform_outbox` drenado post-commit + **cada 5 min** por scheduler (B7n) · consumidor push de `ChargeSettled` con suscripción y DLQ propios (B7e) · barrido de vencimiento + reconciliación · `UNIFORMS_JOBS_ENABLED` (B7c) · `PICKING → CANCELLED` tras N intentos | 4 |
| 12h | **Gateway family + endurecimiento** | rutas `/family/uniforms/*` con `student_id` obligatorio en el RPC (S-1) · **CSRF** (S-3) · rate limit por principal · topes de `qty` (B7i) · `Idempotency-Key` · **ADR 0009** (primera escritura de tutor en la plataforma) | 2.5 |
| 12i | **Panel: pedidos y entregas** | la fila es la entrega (R1) · stat cards con "Se pueden surtir hoy" y "Pagadas sin entregar" (R2) · tabs=estado / chips=nivel (R3) · selección en lote · prórroga · banner `degraded` · responsive <768px (H-D2) | 4 |
| 12j | **App familia** | selector de hijo (R8i) · catálogo por nivel con los 3 tratamientos de talla (R6) · carrito con total partido · mis pedidos con entregas · cadenas de la cuenta regresiva (R5) · reemplazo de la tarjeta mock de `MoreScreen` | 2.5 |
| 12k | **Comms + Terraform + gates** | topic `uniform-events` + DLQ + suscripción de comms · plantillas · Cloud Scheduler (`INGRESS_TRAFFIC_ALL`, SA dedicada — B7a) · `verifyCaller` extraído a `pkg/gcpauth` · k6 del pico de agosto · gates · docs | 2.5 |

**Total: ~30 días humano.** La etapa 1 es autónoma: si algo se atora, el colegio ya está operando.

## 14. Plan de pruebas

**Unitarias (dominio, sin I/O)**
- Tabla de transiciones de la entrega: 9 válidas y ~20 inválidas.
- Derivación del estatus del pedido a partir de líneas y entregas (tabla de verdad).
- Días hábiles: fin de semana, día no hábil, puente de 3 días, cruce de mes, cambio de año, y
  **zona horaria**: surtido a las 23:50 del viernes en `America/Mexico_City` no pierde un día.
- Invariantes de stock: `reserved <= on_hand`, sin doble descuento, liberación idempotente.
- Asignación FIFO al recibir stock, incluido el empate por `created_at`.

**Integración (testcontainers + Postgres real)**
- Dos padres apartan la última pieza a la vez → uno gana (`SELECT … FOR UPDATE`).
- **Carrera vencimiento vs cobro**: el barrido corre en el mismo instante en que caja registra el
  pago → nunca queda mercancía cobrada y liberada, ni cargo cancelado con pagos.
- `PaymentRegistered` entregado dos veces → un solo descuento.
- Pago parcial → la entrega **no** se libera; segundo pago que salda → `PAID`.
- Fallo entre `CreateCharge` y el guardado → reintentar **no** crea dos cargos.
- Rollback completo: si la línea 3 de 3 no tiene stock, las líneas 1 y 2 no quedan reservadas
  fuera del pedido.
- **Aislamiento RLS** en las 6 tablas nuevas: tenant A no lee ni escribe lo de B.
- Vencimiento perezoso libera stock cuando el job nunca corrió.

**Contrato** — cada ruta contra `docs/api/openapi.yaml`.

**Flag** — suite con `module_uniforms` ON y OFF; OFF ⇒ 404 en las 15 rutas.

**Autorización**
- Tutor A no lee ni cancela el pedido del alumno de tutor B → **404**, sin filtrar existencia.
- Un tutor no puede pedir para un alumno al que no está vinculado.
- **Precio manipulado**: el cliente manda `unit_price_cents: 1` → el pedido cuesta lo del catálogo.
- Rol sin `uniforms:write` no despacha (403). Rol sin `finance:write` no hace venta en mostrador.
- `POST /jobs/expire-fulfillments` sin OIDC de la SA correcta → 403.

**Web (RTL)** — cada estado (cargando, vacío, error, sin permiso, **parcial**), filtros, orden,
acciones en lote, y el semáforo de vencimiento con color **y** texto.

**E2E** — (1) flujo feliz completo; (2) flujo vencido; (3) **flujo partido**: 3 prendas, una sin
stock, entrega A cobrada y despachada, llega stock, entrega B cobrada y despachada;
(4) venta en mostrador.

## 15. Observabilidad

- **Logs estructurados** en cada transición: `order_id`, `folio`, `fulfillment_id`, `from`, `to`,
  `actor`, `reason`. **Nunca nombres** (PII).
- **Métricas día 1**: entregas por estado · unidades apartadas · unidades vencidas por semana ·
  antigüedad de la línea más vieja en `awaiting_stock` · **entregas atoradas en `PICKING`**
  (debe ser 0; si no, finanzas está fallando) · edad del último barrido exitoso.
- **Alertas**: entrega en `PICKING` > 15 min · barrido sin correr > 26 h · cualquier violación de
  invariante de stock · `PaymentRegistered` sin entrega > 5/día.
- **Runbook**: entrega atorada en `PICKING` (botón Reintentar) · conteo físico que no cuadra
  (ajuste con nota) · scheduler caído (invocar el job a mano con OIDC).

## 16. Despliegue

- Migraciones **100% aditivas**: 6 tablas nuevas en uniforms, 1 en config, 1 en iam. Ninguna toca
  datos existentes (D4 eliminó el backfill de `students`).
- Orden: migrar → desplegar → flag OFF → humo → flag ON en dev.
- Ventana de código mixto: los campos nuevos del evento `PaymentRegistered` son aditivos; comms los
  ignora.
- **Rollback**: flag OFF, segundos, sin deploy. Las tablas nuevas quedan y no estorban.
- **Verificación post-deploy (5 min)**: health de uniforms · `GET /uniforms/items` responde 200 con
  flag ON y 404 con OFF · un pedido completo de prueba en dev · el barrido invocado a mano regresa
  200 con 0 vencidos.
- **Reversibilidad 5/5**: rebanada vertical detrás de un flag, sin cambios destructivos en otros
  módulos. D4 eliminó la única puerta de una sola dirección que tenía el plan.

## 17. Fuera de alcance (a TODOS.md)

- Pago en línea del uniforme desde la app.
- Reconciliación de conteo físico con UI de inventario cíclico (la bitácora ya da la auditoría).
- Sugerencia de reposición por demanda (la alerta de `min_stock` cubre el 80%).
- Guía de tallas con medidas o fotos.
- Bloquear pedidos si el alumno tiene adeudo vencido (decisión de política del dueño).
- Devoluciones y cambios de talla post-entrega.
- Órdenes de compra a proveedor.
- CFDI del uniforme (vive en M11, fuera del MVP).
- Código de barras / escaneo en almacén.
- **Arreglar `finance.Charge.Cancel()` para que rechace cargos con pagos** (hallazgo A3: es de otro
  módulo y necesita el visto bueno del dueño).
- Matrícula / número de alumno (D4: descartado por ahora).

---

# GSTACK REVIEW REPORT — /autoplan

Modo: **SELECTIVE EXPANSION**. Fase 1 (CEO) cerrada con el gate de premisas contestado (§0).
Voces duales: **codex no disponible** en esta máquina (binario no instalado) ⇒ `[subagent-only]`.

## FASE 2 — DESIGN REVIEW (7 pasos)

Calificación inicial de completitud de diseño del plan: **6.5/10**. Un 10 aquí es: cada
superficie con su copy literal en español, el pedido partido resuelto visualmente, y el panel
usable **desde el teléfono del encargado de almacén** — que es donde de verdad se va a usar.

### Paso 1 — Arquitectura de información · 7/10

Lo que ve el usuario, en orden: (1) cuántos pedidos esperan acción mía hoy, (2) cuáles vencen
pronto, (3) la tabla. El orden de stat cards del plan es correcto. Dos huecos:

- **H-D1 (alto): el pedido partido no está resuelto visualmente.** "Fila expandible" no dice qué
  se ve. Propuesta concreta: la fila del pedido es **el pedido**, y su celda de estatus muestra
  las entregas como píldoras encadenadas, no una sola:

```
  FOLIO           ALUMNO            NIVEL      PRENDAS  TOTAL    ESTATUS                  VENCE
  ─────────────────────────────────────────────────────────────────────────────────────────────
  U-2026-000123   Zoe Villalobos    Primaria   3        $1,450   ●-A Lista  ●-B Por surtir  4 d
    └ expandida ─────────────────────────────────────────────────────────────────────────
        Entrega A · $800   Playera T8 · Pantalón T8     Lista · pasa a caja antes del 12 sep
        Entrega B · $650   Suéter T10                   Por surtir · sin talla desde el 2 sep
```
  La columna **VENCE** muestra siempre el plazo **más próximo** de las entregas vivas.
  Ordenar por "vence" ordena por ese mínimo.

- **H-D2 (alto): falta el panel en teléfono.** El encargado de almacén surte con el celular en la
  mano, entre los anaqueles. En <768 px la tabla debe colapsar a tarjetas de una columna con la
  acción primaria (Surtir) como botón de ancho completo, y el buscador fijo arriba. El plan no
  dice nada de responsive. **Es el hueco más grande del diseño.**

### Paso 2 — Cobertura de estados · 6/10

El plan **enumera** los estados pero no los **especifica**. Un estado sin copy literal termina
siendo "No items found." Tabla obligatoria (el implementador copia y pega):

| Superficie | Cargando | Vacío | Error | Parcial |
|---|---|---|---|---|
| Panel · pedidos | skeleton de 6 filas | "Todavía no hay pedidos. Los pedidos aparecen aquí cuando una familia pide desde la app, o cuando registras una venta en mostrador." + botón *Venta en mostrador* | "No pudimos cargar los pedidos." + *Reintentar* | fila con píldoras `-A` / `-B` (H-D1) |
| Panel · inventario | skeleton de tabla | "Aún no tienes prendas. Crea la primera y genera sus tallas de un golpe." + botón *Nueva prenda* | ídem | talla bajo mínimo: píldora ámbar "Quedan 2 · mínimo 5" |
| Panel · catálogo | skeleton | "Crea tu primera prenda" | inline en el formulario, no toast | — |
| Venta en mostrador | botón en *Cobrando…* deshabilitado | — | inline con el error de caja, **sin perder el carrito** | — |
| App · catálogo | skeleton de tarjetas | "El colegio aún no publica uniformes para \<nombre\>." | "No pudimos cargar el catálogo." + *Reintentar* | talla agotada = *Se solicita al colegio* (Paso 5) |
| App · mis pedidos | skeleton | "Aún no has hecho pedidos." + botón *Ver uniformes* | ídem | dos tarjetas bajo un folio |
| Cualquiera sin permiso | — | "Este módulo no está disponible para tu cuenta." | — | — |

### Paso 3 — Recorrido y arco emocional · 7/10

| # | El padre hace | Siente | ¿El plan lo sostiene? |
|---|---|---|---|
| 1 | Abre Uniformes | curiosidad | Sí — catálogo del nivel de su hijo |
| 2 | Ve que SÍ hay su talla | **alivio** | Sí — disponibilidad explícita |
| 3 | Ve que NO hay su talla | frustración | Parcial — decir *"Se solicita al colegio · te avisamos cuando llegue"* convierte la frustración en espera con contrato |
| 4 | Confirma | confianza | Sí — total visible + idempotencia |
| 5 | **Espera** | **ansiedad ← punto frágil** | Parcial — el estatus por pull lo cubre; falta que la tarjeta diga *desde cuándo* espera |
| 6 | "Ya está listo" | urgencia clara | Sí, si el copy trae la fecha (Paso 4) |
| 7 | Paga y recoge | cierre | Sí |
| 8 | Se le pasó el plazo | culpa/enojo | **Hueco**: el mensaje de vencido debe ser sin regaño y ofrecer *Volver a pedir* en un toque |

Para el encargado de almacén el arco es otro: 130 pedidos en dos días. Su emoción dominante es
**"¿ya acabé?"** → el panel necesita que *Por surtir* llegue a cero visiblemente (estado de
felicitación cuando el contador queda en 0, no una tabla vacía y muda).

### Paso 4 — Riesgo de UI genérica · 8/10

Bajo: el proyecto ya tiene sistema de diseño real (Poppins display + DM Sans, tokens propios,
`Icon.tsx`, `Btn`), así que no hay riesgo de plantilla SaaS morada. Dos vigilancias:
- Las 4 stat cards son el patrón sancionado del proyecto (App UI, no landing) — están bien.
- **El contador de días no puede quedar genérico.** "Expires in 4 days" es slop. Ver Paso 5.

### Paso 5 — Alineación con el sistema de diseño · 8/10

El plan cita `DESIGN-SYSTEM.md` pero no nombra los componentes. Concreción obligatoria:

- **Reusar tal cual:** `Btn`, `Icon` (ya trae el ícono `uniforms`), `TopBar`, `Toast`, `Avatar`,
  `StatusPill`, `FilterTabs`, `FilterChips`, `SlideOver`, `Fab`, `DataTable`, `StatCard`.
- **Nuevo, y solo esto:** `DeadlinePill` (semáforo de días hábiles: color **+ texto**) y
  `FulfillmentChain` (las píldoras `-A`/`-B` de H-D1). Nada más se inventa.
- Barra de selección múltiple = la banda lima del punto 6 del sistema de diseño, sin variantes.
- Paginación = el footer del punto 7b, con "Mostrando X–Y de N" y píldoras 20/50/100.

### Paso 6 — Responsive y accesibilidad · 4/10 ← el peor paso

- **Responsive**: no especificado. Fix en H-D2 (tarjetas en <768 px, acción primaria de ancho
  completo, buscador fijo).
- **Color solo no basta**: el semáforo de vencimiento lleva texto ("Vence en 4 días hábiles"),
  no solo verde/ámbar/rojo. Ya está en el plan; se refuerza con el componente `DeadlinePill`.
- **Objetivos táctiles ≥44 px** en la app y en el panel móvil (los botones circulares de acción
  por fila del sistema de diseño se quedan cortos en móvil → en <768 px se vuelven botones con
  etiqueta).
- **Teclado**: la tabla necesita foco visible por fila, `Enter` para abrir la ficha, y `Espacio`
  para marcar el checkbox. La venta en mostrador debe poder completarse **sin mouse**.
- **Lector de pantalla**: la píldora de estatus necesita `aria-label` con el texto completo
  ("Entrega A, lista, vence el 12 de septiembre"), no solo el color.

### Paso 7 — Decisiones de diseño sin resolver (si se difieren, esto pasa)

| Decisión | Si se difiere |
|---|---|
| Copy exacto del contador de días hábiles | El ingeniero escribe "Vence en 4 días" y el padre cuenta sábados |
| Cómo se ve una talla agotada en el selector del padre | Se ship-ea deshabilitada y gris, y el padre cree que la app está rota |
| Qué pasa cuando *Por surtir* llega a 0 | Tabla vacía muda en vez de cierre satisfactorio |
| Panel en teléfono | La tabla se scrollea horizontal y el almacén usa papel |
| Mensaje de pedido vencido | Suena a regaño y el padre no vuelve a pedir |

**Copy resuelto aquí (va al plan, no al criterio del implementador):**

```
  día 10 →  "Pasa a caja antes del viernes 12 de septiembre · te quedan 10 días hábiles"
  día 3  →  "Pasa a caja antes del viernes 12 de septiembre · te quedan 3 días hábiles"
  día 1  →  "Último día hábil: mañana, viernes 12 de septiembre"
  día 0  →  "Hoy es el último día para pasar a caja"
  vencido→  "El apartado se liberó el 12 de septiembre.  [ Volver a pedir ]"
```

**Talla agotada en el selector del padre** (nunca deshabilitada y muda):
```
   Talla   6      8      10     12
          ┌────┐ ┌────┐ ┌────┐ ┌────┐
          │ 6  │ │ 8  │ │ 10 │ │ 12 │
          └────┘ └────┘ └────┘ └────┘
           hay    hay   ┌ se solicita ┐  hay
                        └ al colegio  ┘
   Al elegir 10:  "No hay talla 10 ahora. Si la pides, el colegio la surte
                   y te avisamos cuando llegue."   [ Pedirla de todos modos ]
```

**Puntuación del paso 2 de diseño (scorecard):** jerarquía 7 · estados 6 · recorrido 7 ·
especificidad 8 · sistema de diseño 8 · móvil 4 · accesibilidad 4.

## FASE 3 — ENG REVIEW

### Sección 1 — Arquitectura (hallazgos nuevos sobre los ya incorporados en §12)

**E1 (crítico) — El estado de las líneas después de que vence una entrega no está definido.**
Traza: pedido con L1, L2, L3. L1 y L2 se surten en la entrega A; L3 queda `awaiting_stock`.
La entrega A vence. ¿Qué son L1 y L2 ahora? ¿Se pueden volver a surtir? El plan no lo dice, y de
esa ambigüedad salen dos bugs opuestos: mercancía apartada dos veces, o mercancía perdida.

**Regla que se fija:**
```
  reserved ──(entra a entrega)──► reserved ──(PAID)──►  sold      terminal
                                     │
                                     └──(EXPIRED/CANCELLED)──► released   terminal
```
- `released` es **terminal**. Una línea liberada NO se vuelve a surtir: la familia vuelve a pedir.
  Es lo único honesto — el colegio ya reclamó la mercancía y pudo habérsela vendido a otro.
- El vencimiento de la entrega A **no** mata a L3. L3 sigue `awaiting_stock` y se puede surtir en
  una entrega B cuando llegue stock. Ajustable con `cancel_order_on_expiry` (default `false`).
- El pedido se cierra (`closed_at`) cuando no quedan líneas vivas ni entregas vivas.
- Una prenda desactivada en el catálogo **sí** se puede seguir surtiendo si ya estaba pedida
  (el precio va en snapshot; desactivar solo la quita del catálogo del padre).

**E2 (crítico) — La venta en mostrador descuenta inventario ANTES de confirmar el dinero.**
El §3.4 hace: TX1 (crea y **vende**) → CreateCharge → RegisterPayment → TX2. Si el proceso muere
después de TX1, hay mercancía descontada sin cobro registrado, y hay que arreglarlo a mano.
**Fix: la venta en mostrador usa exactamente la misma máquina de estados que el flujo normal**,
sin atajos:
```
  TX1: crea order+líneas+fulfillment en PICKING  y  RESERVA (no vende)
  RPC: CreateCharge(idempotency_key = fulfillment_id)
  RPC: RegisterPayment(idempotency_key = fulfillment_id)
  TX2: PICKING ─► PAID (aquí y solo aquí on_hand -= qty) ─► DISPATCHED
```
Puntos de muerte y estado resultante, todos recuperables sin intervención manual:

| Muere en | Estado | Quién lo repara |
|---|---|---|
| después de TX1 | entrega en `PICKING`, stock **apartado** (no vendido) | `ResumeStuckPicking` o el cajero al reintentar |
| después de `CreateCharge` | cargo creado sin pagar, entrega en `PICKING` | reintento: la clave de idempotencia reusa el cargo |
| después de `RegisterPayment` | dinero cobrado, entrega en `PICKING` | el evento `PaymentRegistered` la cierra; el barrido es la red |
| después de TX2 | correcto | — |

Con esto **el inventario nunca se descuenta antes de que el dinero esté confirmado**, y el atajo
de "vender directo" desaparece: hay una sola máquina de estados, no dos.

**E3 (alto) — Pago que llega DESPUÉS de que la entrega venció.**
El barrido corre a las 06:00 y libera el apartado; a las 09:00 el padre paga en caja el cargo que
nadie canceló (o cuya cancelación falló). Hoy el plan no tiene respuesta: el evento llegaría a una
entrega `EXPIRED` y se ignoraría en silencio, con el dinero cobrado y sin mercancía asignada.
**Fix:** campo `requires_attention BOOLEAN` + `attention_reason` en la entrega. El consumidor,
al recibir un pago de una entrega `EXPIRED`, **no** descuenta stock: marca
`requires_attention = 'pago recibido de un apartado vencido'` y publica el evento. El panel tiene
un filtro **"Requieren atención"** que el staff resuelve (re-apartar si hay stock, o devolver el
dinero). Silencio cero: es el Prime Directive #1.

**E4 (alto) — El vencimiento perezoso escribe dentro de una lectura.**
Hacer `UPDATE` dentro de un `GET` vuelve la lectura no idempotente, la hace lenta y la puede
poner en deadlock con el barrido. **Fix matizado:** la lectura **calcula** el vencido para pintar,
y dispara la liberación en una transacción corta aparte, **best-effort y sin bloquear la
respuesta** (si falla, se registra y el barrido lo recoge). La verdad la sigue teniendo el barrido;
la lectura es solo una red adicional. Así se conserva H4 sin ensuciar el camino de lectura.

**E5 (medio) — `config` caído justo al surtir.**
El plan degrada a lunes–viernes sin festivos. Es la decisión correcta (no se puede bloquear al
almacén por el calendario), pero deja una fecha límite equivocada en un cargo que el padre va a
ver. **Fix:** marcar `deadline_estimated = true`; el panel muestra el aviso; y el barrido
**recalcula** los plazos estimados cuando `config` vuelve, extendiendo (nunca acortando) el plazo.
Acortar un plazo ya comunicado al padre sería indefendible.

### Sección 2 — Calidad de código

- **El desvío de layout necesita el ADR 0007 antes de escribir código**, no después. Sin él, un
  revisor tiene razón en bloquear el PR por violar `CLAUDE.md`.
- **DRY real:** la validación "este tutor es tutor de este alumno" ya existe en
  `services/gateway/internal/handler/family.go`. Extraerla a un helper compartido antes de usarla
  por tercera vez, no copiarla.
- **El ledger `processed_events` se copia del molde probado** (`migrations/comms/0001_init.sql:49`)
  — misma forma, tabla propia de uniforms, RLS incluida.
- La derivación del estatus del pedido y la tabla de transiciones viven en funciones puras del
  dominio con tabla de verdad. Prohibido escribir `status` desde el repositorio.

### Sección 3 — Pruebas (lo que falta sobre §14)

11. **Línea liberada no se vuelve a surtir** (E1): vence A, llega stock, intentar re-surtir L1 → error.
12. **L3 sobrevive al vencimiento de A** (E1): sigue `awaiting_stock` y se puede surtir en B.
13. **Muerte en cada uno de los 4 puntos de la venta en mostrador** (E2): 4 pruebas, cada una
    verifica que reintentar deja el sistema correcto y sin cargos ni descuentos duplicados.
14. **Pago después del vencimiento** (E3): no descuenta stock, marca `requires_attention`,
    aparece en el filtro del panel.
15. **`config` caído al surtir** (E5): el plazo se calcula degradado, queda `deadline_estimated`,
    y el barrido lo recalcula extendiéndolo cuando config vuelve.
16. **Lectura de una entrega vencida** (E4): la respuesta no se bloquea aunque la liberación falle.
17. **Cargo cancelado con pagos** (A3): el barrido nunca cancela un cargo con `paid_cents > 0`.

### Sección 4 — Rendimiento

Ya cubierto en la Sección 7 del CEO (índices, snapshot para evitar fan-out, lotes de 100,
`Preload` explícito, caché del calendario). Sin hallazgos nuevos: a 130 alumnos y ~150 pedidos por
temporada nada de esto es un cuello de botella; los índices están ahí para que a 10x tampoco lo sea.

### Honestidad de la estimación

18.5 días humano es **optimista** con lo que se agregó en la revisión (panel responsive, venta en
mostrador por la máquina única, pedido partido, calendario en `config`, `requires_attention`).
Estimación honesta: **21–23 días humano**, ~6.5–7 h de CC. Se ajusta en §13 al aprobar.

## FASE 2b — VOZ INDEPENDIENTE DE DISEÑO (subagent) · aceptada

Calificación de la voz: jerarquía **4/10** · estados **3/10** · recorrido **5/10** ·
especificidad **3/10** · sistema de diseño **5/10** · móvil **4/10** · accesibilidad **3/10**.

Diagnóstico que da en el clavo: **§3 y §11 deciden cosas difíciles; §8 y §9 eran el índice del
sistema de diseño con verbos.** Las siguientes revisiones **sustituyen** lo que decían §8 y §9.

### R1 (crítico) — La fila de la tabla es la **entrega**, no el pedido

El plan hacía fila = pedido, cuando el pedido es explícitamente *un contenedor sin estado*.
La consecuencia no es estética: **la columna de vencimiento no puede existir en una fila de
pedido** (¿el plazo de A o el de B?), y cualquier agregado es una mentira que el cajero lee en
voz alta. Además, un acordeón dentro de una tabla paginada rompe el "Mostrando X–Y de N" del
sistema de diseño, que no tiene patrón de acordeón en ninguna superficie.

```
│ U-2026-000123-A  Ana Ruiz Márquez  Primaria 3°B  2 prendas  $890  ●Lista para recoger  12 sep · 4 d
│ U-2026-000123    Ana Ruiz Márquez  Primaria 3°B  1 prenda   $450  ●Por surtir          sin plazo
```
La fila de lo pendiente **no lleva plazo** — nada está apartado, el reloj no arrancó — y eso solo
ya explica el modelo. Se atan con la barra de acento izquierda del mismo color por pedido
(el DS ya la tiene en `StuRow`/`FinRow`) y con el folio en dos pesos: `U-2026-000123` en
`text-faint`, `-A` en `--text`/700. **Nunca renderizar `-A` solo:** el padre lee el folio completo
en la ventanilla.
Clic en la fila abre la **ficha del pedido** (drawer, DS §9) con líneas, entregas, línea de tiempo
y acciones. Se elimina la fila expandible.

### R2 (alto) — Stat cards: falta el número que da vergüenza

*Listas por recoger* depende del padre (el colegio no hace nada con él) y *Vencidas hoy* es
forense. Faltaba **pagadas sin entregar**: un padre ya pagó y la mercancía sigue en el estante.
Es el peor fallo del módulo y se cierra con un clic.

| # | Card | Color | Sub-label |
|---|---|---|---|
| 1 | **Se pueden surtir hoy** ← héroe | lime | "de 34 líneas pendientes" |
| 2 | **Pagadas sin entregar** | red | "El padre ya pagó" |
| 3 | **Vencen en ≤3 días** | yellow | "Llamar o prorrogar" |
| 4 | **Sin existencia (backorder)** | orange | "La más antigua: 23 días" |

El héroe **no** es "por surtir" en crudo (incluye backorders sobre los que nadie puede actuar y
nunca baja: eso es ruido). Es *líneas `awaiting_stock` cuya talla tiene `available > 0`*.
**Las cuatro cards son clicables y aplican filtro.** *Listas* y *Vencidas* bajan a filter tabs.

### R3 (medio) — Tabs = estado, chips = nivel (no al revés)

`StudentsScreen.tsx` fija el vocabulario: tabs con contador = estado
(`Todos · Inscritos · Trámite · Bajas`), chips = nivel. El plan lo tenía invertido. Se corrige
para que dos módulos hermanos no se manejen distinto sin razón.

### R4 (alto) — Estados con **copy literal**, no una casilla marcada

Además de lo ya escrito en la Fase 2, se agregan los que faltaban por completo:

- **Tres vacíos distintos en el panel**, no uno: día 1 sin pedidos (CTA *Venta en mostrador*) ·
  filtro sin resultados (CTA *Limpiar filtros*, pareja que `StudentsScreen` ya tiene) ·
  cero pendientes (celebración, como el "¡Todo en orden!" de `FinanceScreen`).
- **"Parcial" no es un estado de UI**: es un dato (una fila más, R1). Llamarlo estado produce un
  test RTL que no prueba nada. Se elimina de la lista de estados.
- **`degraded` — el estado que nadie había diseñado.** Si `config` no responde, el propio backend
  fabrica un plazo sin calendario. Banner ámbar fijo bajo el header. **Ver T2: decisión abierta
  sobre si además bloquea "Surtir".**
- **Permiso parcial, no "sin permiso".** Quien no tiene `uniforms:read` nunca llega a la pantalla
  (`moduleHasPerm` oculta el tile). El caso real es **`uniforms:write` sin `finance:write`**:
  el cajero captura 6 líneas y come un 403. La entrada a Mostrador aparece **deshabilitada con la
  razón**: *"Necesitas permiso de cobros para vender en mostrador"*. (Es la primera vez que se
  instancia la regla "un botón deshabilitado dice por qué", que el plan citaba sin aplicar.)
- **Inventario**: "bajo mínimo" es una **tab con contador** ("Bajo mínimo · 7"), no decoración de
  fila — así se usa: el encargado entra a ver qué reponer.
- **Catálogo**: éxito parcial del alta por rango — *"8 tallas creadas, 2 ya existían"*, no upsert
  silencioso.
- **Mostrador · éxito**: un toast de 2.6 s no basta para una operación que movió dinero.
  Folio grande, total, cambio, y la pantalla lista para la siguiente venta.
- **Mostrador · fallo a la mitad**: si `RegisterPayment` falla después de `CreateCharge`, el
  cajero **ya tiene el efectivo en la mano**. Estado propio con el folio a la vista:
  *"Cargo creado, pago sin confirmar — Reintentar"*. Nunca un toast rojo genérico.
- **Móvil (superficie completa que el plan no mencionaba)**: reusar `AsyncLoading` /
  `AsyncErrorState` / `GatedScreen` / `EmptyState` que ya existen — que 12f no invente. Y los
  propios del módulo: catálogo vacío *para el nivel de ese hijo* (≠ catálogo vacío) · carrito
  vacío · **la talla se agotó entre agregar y confirmar** (es el caso real, no el raro) ·
  envío en curso con botón bloqueado · **límite alcanzado** (`max_open_orders_per_student`,
  `max_lines_per_order`) avisado **antes** de confirmar, no con un 4xx · mis pedidos vacío ·
  **estado vencido**, que es el único que le duele y no tenía tratamiento.
- **`PICKING`**: si puede durar 15 minutos, alguien lo va a ver. Píldora gris "Procesando…" sin
  acciones + botón **Reintentar** (`ResumeStuckPicking`) **expuesto en la fila**, no solo en el
  runbook.

### R5 (alto) — La cuenta regresiva tenía bug en los dos extremos

Con la plantilla única del plan, el día 10 decía "te quedan 10 días hábiles" (alarma sin
informar) y el día 0 decía "te quedan 0 días hábiles", que es ilegible. **Nunca "0 días",
nunca "1 días".** La fecha es la constante, el número la variable, y el tono escala:

| Momento | App del padre | Chip | Panel |
|---|---|---|---|
| día 10 | "Ya está lista. Recógela antes del viernes 12 de septiembre." | ok | `12 sep · 10 d` |
| día 3 | "Te quedan 3 días hábiles · hasta el viernes 12 de septiembre" | warn | `12 sep · 3 d` |
| día 1 | "Último día: mañana, viernes 12 de septiembre" | warn | `12 sep · mañana` |
| día 0 | "Hoy es el último día para recoger tu pedido" | bad | **`Hoy`** rojo/700 |
| vencido | "El apartado venció el 12 de septiembre y se liberó. Puedes volver a pedirlo." | neutral | `Venció 12 sep`, fila atenuada |

- A 10 días no se cuenta; a 3 aparece el número; **el día 1 es el único que se gana un push**.
- El vencido lleva CTA **"Pedir de nuevo"** — acción que el plan no tenía (el padre solo podía
  cancelar, nunca recrear en un toque).
- **Reconciliación de la aritmética**: "4 días hábiles" y "viernes 12" se contradicen a ojos de un
  padre que cuenta 6 días. Una línea, una sola vez: *"(no contamos sábados, domingos ni festivos)"*.
- **Prórroga**: `extended_days` existía en el modelo con **cero copy**. Si la fecha cambia sola, el
  padre concluye que la app le mintió → *"Se amplió tu plazo al viernes 19 de septiembre."*
- **El cálculo no se hace en el cliente.** El servidor manda `remainingBusinessDays` y
  `deadlineLabel` ya resueltos en `America/Mexico_City`. El plan acertaba en el dominio y no
  cerraba el contrato hacia la UI.

### R6 (alto) — Selector de talla: tres tratamientos, y el carrito parte el total

"Se solicita al colegio" **no es un estado de la talla: es otro producto con otra promesa**
(sin fecha, sin apartado, sin plazo), y el plan solo se lo decía al padre *después*, como estatus.

- **Disponible** → chip normal. **Últimas piezas** (`available ≤ 2`) → chip + punto ámbar.
  **Sin existencia** → chip con **borde punteado y número tachado**; nunca gris deshabilitado,
  nunca oculto; **sigue siendo seleccionable**.
- La explicación **no vive en el chip**: una sola línea bajo el picker que cambia con la selección
  (un solo lugar que leer): *"Disponible · se aparta al confirmar"* / *"Quedan 2 · se aparta al
  confirmar"* / *"Sin existencia · la pedimos al proveedor y te avisamos cuando llegue.
  No tiene fecha estimada."*
- **El carrito parte el total** — aquí es donde el plan no conectaba el modelo con la pantalla:
  *"Pagas al recoger: $890 · Por encargo (se cobra cuando llegue): $450"*. Sin partirlo, el padre
  cree que el viernes paga $1,340. Es la entrega parcial vista desde el otro lado.
- Distinguir **talla inexistente en catálogo** (no se pinta) de **existente sin stock** (punteada).
- A11y: `accessibilityLabel="Talla 8, sin existencia, por encargo"`. El plan exigía "color + texto"
  en el panel y lo olvidaba en móvil.

### R7 (crítico) — Venta en mostrador: el FAB estaba mal y faltaba el caso más frecuente

- **El FAB es el control equivocado.** En este sistema el FAB es la acción de creación primaria
  *de la superficie*; en el panel de pedidos ese slot lo ocupa Surtir/Despachar. Además obliga a
  cargar una tabla paginada de cientos de filas antes de vender un suéter, con un padre de pie.
  → **Mostrador es pestaña propia de primer nivel**, es la **pestaña de aterrizaje por defecto**
  para un usuario cuyo único rol es Caja/Almacén, y se registra en el **command palette que ya
  existe** (`apps/web/app/command/registry.ts`) — infraestructura que el plan ignoraba.
- **Objetivo de clics explícito** (el plan no fijaba ninguno): venta típica
  (1 prenda, 1 talla, efectivo, sin alumno) = **4 interacciones**. Con alumno, +1.
  **Efectivo preseleccionado** (P1; `PayDrawer` de Finanzas ya arranca en `cash`).
  **Referencia solo si el método no es efectivo.** Cantidad por defecto 1.
- **El selector de prenda es una rejilla de las prendas activas** (una tienda escolar tiene 6–12
  SKU, no un marketplace), con las tallas como fila de chips al elegir. Con teclado: `sue` filtra,
  Enter selecciona, `M` elige talla, Enter cobra.
- **`studentId` opcional era una trampa de dinero.** El cajero se lo va a saltar *siempre* porque
  es el paso más lento, y ahí se rompe en silencio P2 ("el cobro vive en el estado de cuenta").
  → El alumno viene **activado por defecto**, su buscador es el primer campo con foco, y
  *"Venta sin alumno"* es un toggle explícito que **cambia visiblemente la tarjeta**
  (*"Esta venta no quedará en ningún estado de cuenta"*) y **exige nota**.
- **Falta el cambio**: campo "Recibí" que calcula *"Cambio: $110"*. Es el error humano más común
  de una caja y cuesta un input.
- **Comprobante**: o la pantalla de éxito ofrece Imprimir/Exportar, o se declara explícitamente
  "sin comprobante en este alcance" — y entonces hay que decírselo al padre. **Se declara fuera de
  alcance** (§17) y la pantalla de éxito muestra el folio para anotarlo.
- **El caso más frecuente de un mostrador escolar no estaba**: el padre que llega **con un pedido
  ya listo**. Eso no es vender, es despachar, y el plan lo dejaba en otra pestaña, obligando al
  cajero a saber de antemano qué caso tiene enfrente con la fila esperando.
  → La pestaña Mostrador arranca con **un solo campo**: *"Folio o nombre del alumno"*.
  Si hay entregas listas → tarjeta con **"Cobrar y entregar"** en un clic (`PAID → DISPATCHED`).
  Si no hay → cae a la rejilla de venta nueva. **Un campo, dos caminos.**
  Es el único momento del módulo en que un humano espera parado frente a otro.

### R8 — Bugs de plan y omisiones del sistema de diseño (todos aceptados)

| # | Hallazgo | Corrección |
|---|---|---|
| a | **La columna "nivel/grupo" no se puede pintar**: §3.2 hace snapshot de `student_name` y `student_level`, pero **el grupo no viaja** | Se agrega `student_grade_group` al snapshot |
| b | Falta la **barra de selección masiva** (DS §6) aunque el plan pide selección múltiple | Se usa la banda lima del DS, sin variantes |
| c | Falta la **paginación en pantalla** (DS §7b, "toda tabla larga la lleva") | Footer idéntico al de `FinanceScreen`/`StudentsScreen` |
| d | Falta el **`Avatar`** en la fila (toda tabla con alumno de este sistema lo lleva) | Avatar + nombre + folio en `text-faint` |
| e | Las **pestañas de sección no existen en este sistema** (Students/Finance/Attendance no las usan) | Se declara como decisión nueva y consciente: 4 pestañas (Pedidos · Mostrador · Inventario · Catálogo) |
| f | **Despachar** no necesita confirmación, necesita **deshacer** | Toast con *Deshacer* durante 6 s; confirmación solo en Cancelar entrega y Ajustar inventario |
| g | **Drawer de entrada de almacén** mal dimensionado: llega una caja con 40 piezas en 6 tallas | Una prenda → **columna de cantidades por talla** → un solo guardar |
| h | **Móvil: no se dice dónde vive Uniformes** | Reemplaza la tarjeta mock de `MoreScreen` (`MODULE_FLAGS.uniformes`); **no** se gana un tab (los 5 están tomados) |
| i | **Móvil: el catálogo es "por hijo" y no hay selector de hijo** — bloqueante silencioso de 12f | `AppState` ya tiene `children` + `selectChild`: si hay 1 hijo se omite; si hay 2+, chips de hijo arriba del catálogo |
| j | `uniforms:read` otorgable deja ver el historial de compra de todo el colegio | Mitigación barata sin tocar permisos: Pedidos muestra por defecto **solo entregas vivas**; el histórico exige rango de fechas explícito |
| k | Falta superficie para el padre que **no alcanza a pasar a caja** | Ver **T3** (decisión abierta) |

## FASE 3b — VOZ INDEPENDIENTE DE INGENIERÍA (subagent)

Veredicto de la voz: **6/6 dimensiones en NO**. Recomienda **no aprobar 12a** hasta resolver
4 puntos. Los hallazgos de abajo se **verificaron contra el código**, no se aceptaron de palabra.

### B1 (CRÍTICO · VERIFICADO) — D2 (entrega parcial) es imposible contra el `finance` que existe

`migrations/finance/0001_init.sql` declara en `charges`:
```sql
UNIQUE (tenant_id, student_id, cycle_id, concept_id)
```
El plan crea **un cargo por entrega**, todas con el concepto "Uniformes", el mismo alumno y el
mismo ciclo. La entrega `-A` cobra bien; **`-B` falla con conflicto**. También falla un segundo
pedido del mismo alumno en el mismo ciclo, y una venta en mostrador a un alumno que ya compró.
La afirmación de §4/§16 de que el cambio a finanzas es "aditivo" era **falsa**.

Y quitar la restricción a secas es peor: `CreateChargesBulk` usa
`clause.OnConflict{DoNothing: true}` **sin target** (`repository.go:217`), así que al desaparecer
la restricción la cláusula deja de deduplicar **en silencio** y `GenerateCharges` vuelve a
facturar el colegio entero cada vez que se reejecuta. **Doble cobro de colegiaturas a todo el
padrón, sin error.**

**Fix propuesto (requiere decisión del dueño — ver gate):**
1. `charges` gana `external_ref text NOT NULL DEFAULT ''`.
2. La restricción se vuelve parcial:
   `CREATE UNIQUE INDEX … ON charges (tenant_id, student_id, cycle_id, concept_id) WHERE external_ref = '';`
   — la colegiatura conserva su idempotencia de "uno por ciclo".
3. `CREATE UNIQUE INDEX … ON charges (tenant_id, external_ref) WHERE external_ref <> '';`
   — el mismo patrón parcial que ya se usó en `0002_payment_idempotency.sql`.
4. `OnConflict` de `CreateChargesBulk` recibe **target explícito**, con test de regresión que
   reejecute `GenerateCharges` y exija `created == 0`.
5. `idempotency_key` **es** `external_ref`; no se inventan dos mecanismos.
   Bonus: le da a la reconciliación (§11.3) una consulta real en vez de N `GetCharge`.

### B2 (CRÍTICO) — La invariante de inventario, como está escrita, viola el `CHECK`

§3.3 dice *"`on_hand -= qty` ocurre exactamente una vez, en la transición a `PAID`"* y §3.1 declara
`CHECK (reserved <= on_hand)`. Con `on_hand = 1, reserved = 1`, pasar a `PAID` deja
`on_hand = 0, reserved = 1` → **la transacción aborta**. Como esa transacción consume el evento
`PaymentRegistered`, el evento se reintenta 5 veces (`max_delivery_attempts = 5` en `pubsub.tf`)
y cae al DLQ: **mercancía pagada, entrega nunca liberada, stock congelado — en el caso más común
del módulo, la última talla.**
**Fix:** la transición a `PAID` es un par atómico `on_hand -= qty; reserved -= qty` por línea, en
una sola sentencia, probado con `on_hand = reserved = 1`.

### B3 (ALTO) — Las líneas liberadas quedan muertas, y el pedido se cierra mintiendo

Mi corrección E1 declaraba `released` terminal. La voz demuestra que eso produce **una falla que
se ve exitosa**: tras vencer la entrega A, L3 se surte y despacha; el estatus derivado responde
"sin líneas pendientes, sin entregas vivas" → **`Cerrado`**. El pedido se cierra habiendo
entregado 1 de 3 prendas y ni el panel ni la app pueden expresarlo.
Además `released` está sobrecargado: significa a la vez "el tutor la canceló", "venció y ya no la
quiere" y "venció pero la sigue queriendo" — tres cosas con tres acciones distintas.
**Fix:** enum de línea `('awaiting_stock'|'reserved'|'sold'|'released'|'cancelled')`, limpiar
`fulfillment_id` al liberar, cuarto eje en el estatus derivado (`Cerrado parcial`), y camino de
reingreso explícito (`released → awaiting_stock` o `→ reserved`) como acción de staff con botón
propio. Más `expiry_count` con tope: sin él, un tutor aparta la última pieza, la deja vencer, la
vuelve a pedir y **bloquea un SKU indefinidamente sin pagar un peso**.

### B4 (CRÍTICO) — `CounterSale` sin idempotencia, y su ruta de recuperación **destruye inventario**

§7 concede `Idempotency-Key` solo a la ruta family. `counter-sale` — la única operación que
descuenta stock físico y mueve dinero en una acción — no la tiene. Doble clic = dos descuentos.

Peor, el punto de crash C1 (tras TX1, antes de `CreateCharge`): la entrega queda en `PICKING`,
`ResumeStuckPicking` la empuja a `READY_FOR_PICKUP` con plazo de 10 días, y a los 10 días el
barrido la vence y **"devuelve al inventario" una prenda que físicamente ya no existe**.
El mecanismo de recuperación corrompe el inventario.
**Fix:** `Idempotency-Key` obligatoria generada al **abrir** el drawer; y las ventas de mostrador
nacen en un estado terminal propio **`SOLD_AT_COUNTER` que ningún barrido puede tocar**, con el
cobro resuelto por outbox. El movimiento `kind='sell'` en la bitácora es la verdad; finanzas es
eventualmente consistente sobre él. `method` se valida en uniforms, no en finance: descubrir en el
paso 3 que el método es inválido significa que la prenda ya salió.

> Corrige mi E2: yo había propuesto que mostrador usara la misma máquina que el flujo normal.
> Es lo contrario de lo correcto — **precisamente por usar la misma máquina, el barrido trata una
> venta consumada como un apartado.** En mostrador la entrega física ya ocurrió: el descuento no
> es una promesa, es el registro de un hecho, y revertirlo automáticamente sería mentir sobre el
> almacén.

### B5 (CRÍTICO) — Carrera vencimiento↔cobro: el orden de operaciones es el peor posible

El plan hacía `leer cargo` → `liberar reserved` → `CancelCharge`, y dejaba **fuera de alcance**
arreglar `Charge.Cancel()`. Si el cajero cobra en esa ventana: cargo pagado, entrega `EXPIRED`,
stock liberado y potencialmente re-vendido, y cargo cancelado con pagos encima.
El caso perezoso es peor y más probable: **el padre abre la app haciendo fila en caja el día del
vencimiento**; la lectura dispara el vencimiento, libera el stock y cancela el cargo; el cajero
cobra y `derivePaidStatus` responde *"cannot pay a cancelled charge"*. Dinero en la mano, sistema
que se niega.
Y §14 prometía un test de esta carrera que **es intestable por diseño**: dos bases de datos sin
primitiva compartida, solo se puede escribir *flaky*.
**Fix:** `finance.CancelChargeIfUnpaid(charge_id)` — atómica bajo el row lock del cargo, misma
forma que el knot de pago del ADR 0002. uniforms libera stock **solo si esa llamada tuvo éxito**.
Eso convierte la carrera en un resultado observable y el test en algo escribible.
**A3 no puede quedar fuera de alcance: es el eje de la corrección del módulo.**

### B6 (ALTO) — `uniforms:read` otorgable publica el padrón del colegio

El criterio literal de `ownerOnlyPermissions` (`permission.go:23-38`) es *"tenant-wide sensitive
reads … with no per-object scoping"*. `ListOrders` devuelve nombre + nivel + historial de compra
de **todo el colegio**, y el plan *snapshotea* los nombres justo para que esa lista sea barata.
Califica sin discusión. Y el **grant-subset gap sigue abierto**: cualquiera con `roles:write`
puede fabricarse un rol con cualquier permiso no-owner-only. "Otorgable" aquí significa, en la
práctica, "legible por quien pueda editar roles".
**Fix — partir el permiso:** `uniforms:stock` (catálogo, inventario, recepción, ajuste, mostrador;
**sin un solo nombre de alumno**) otorgable, que es exactamente el rol Caja/Almacén que D1 quiere;
y `uniforms:orders` (la lista con nombres) **owner-only** hasta que exista scoping por objeto.
La disyuntiva que planteaba el plan era falsa.

### B7 (ALTO) — Otros hallazgos verificados

| # | Hallazgo | Fix |
|---|---|---|
| a | **`INGRESS_TRAFFIC_INTERNAL_ONLY` rompe Cloud Scheduler**: origina desde infraestructura pública de Google, no desde la VPC — es literalmente por eso que `services.tf` pone comms en `INGRESS_TRAFFIC_ALL`. Cada invocación sería 403, y el vencimiento perezoso lo enmascararía **durante semanas** | uniforms en `INGRESS_TRAFFIC_ALL`, `allow_unauthenticated=false`, SA `scheduler` con `run.invoker` solo sobre uniforms, OIDC con audience = URI del servicio. Y **extraer `verifyCaller` de `pkg/pubsub/gcp/push.go` a `pkg/gcpauth`** antes de copiarlo por tercera vez |
| b | **`docs/api/openapi.yaml` NO EXISTE** (verificado). La línea de pruebas de contrato era decorativa, y la canon exige spec-first | O la rebanada 12a lo crea (trabajo no presupuestado, es el contrato del módulo entero), o se declara explícitamente que no hay OpenAPI |
| c | **El rollback prometido no existe** (verificado): los flags se leen de `ENABLED_MODULES` en `gateway/cmd/main.go:76`, no de la base ⇒ apagar un flag es **una revisión nueva de Cloud Run**, no "segundos sin deploy". Y el flag apaga **solo las rutas del gateway**: el barrido y la suscripción push siguen vivos venciendo entregas y llamando a `CancelCharge` con el módulo "apagado" | `UNIFORMS_JOBS_ENABLED` honrado dentro del servicio + runbook: pausar el job del scheduler y desconectar la suscripción push |
| d | **uniforms NO está en Terraform** (verificado, `services.tf:170`). 12g presupuestaba 1.5 d para lo que en realidad es: módulo `cloud_run` completo, `internal_services`, topic + DLQ, **segunda suscripción push sobre `payment-events`** (primera vez en la plataforma), SA scheduler, `cloudscheduler.googleapis.com`, `ENABLED_MODULES` + `UNIFORMS_BASE_URL`, runner de migraciones | 12g sube a 4 d. (Cloud Scheduler da 3 jobs gratis/mes: el barrido diario no cuesta nada) |
| e | **Mejor contrato de evento**: `charge_status` como string duplica estado derivado en el cable e invita a tratar el evento como fuente de verdad sobre dinero. Además uniforms se suscribiría a **todos** los pagos del colegio para descartar el 95% | finance publica **`ChargeSettled { charge_id, student_id, tenant_id, external_ref, settled_at }`** cuando `DeriveStatus` cruza a `paid`. Sin aritmética en el consumidor, sin ambigüedad de orden, y un pago parcial simplemente no lo emite |
| f | **La prórroga rompe dos cosas**: no existe `UpdateCharge`, así que `ExtendDeadline` mueve el plazo en uniforms y deja el cargo con la fecha vieja ⇒ aparece vencido en `ListOverdue` mientras el panel dice "prorrogado". Y la segunda `UniformFulfillmentReady` la **descarta el ledger de comms** por clave repetida ⇒ el padre nunca sabe la fecha nueva | `UpdateChargeDueDate` en finance + discriminador `:<extension_seq>` en la clave del evento |
| g | **El snapshot de `student_name`/`student_level` se pudre**: un alumno transferido de nivel conserva el snapshot ⇒ **el filtro por nivel miente** | Consumir un evento de perfil, o aceptarlo y documentarlo |
| h | **Deadlock en pedidos multi-línea**: varios locks sobre `uniform_stock` sin orden determinista. Y `FOR UPDATE` es innecesario | Una sola sentencia condicional: `UPDATE uniform_stock SET reserved = reserved + $1 WHERE id = $2 AND on_hand - reserved >= $1`, decidir por `RowsAffected`. Atómica, sin lock explícito, sin deadlock |
| i | **Sin tope de `qty` por línea**: 5 pedidos × 20 líneas × qty ilimitado = un tutor aparta el inventario entero de una talla 10 días hábiles, gratis. Y el rate limit propuesto es **por IP**: las familias tras el NAT del colegio comparten cubeta y el atacante rota IP | Tope de unidades por línea y de unidades reservadas por alumno; rate limit por **principal**, no por IP |
| j | **Falta CSRF en el gateway** (`middleware/` solo tiene auth, cors, featureflag, ratelimit). Hasta hoy la superficie de escritura del tutor era nula; `uniforms_own:write` sería **la primera escritura de un tutor en la plataforma** y crea una obligación financiera | CSRF en endpoints de estado; y el permiso de escritura familiar merece **ADR**, no una fila de migración |
| k | **`uniform_stock_moves` no es inmutable**: la política RLS estilo `USING + WITH CHECK FOR ALL` del repo **permite UPDATE y DELETE** | Políticas por operación (`FOR SELECT`, `FOR INSERT`), ninguna de update/delete, y test que lo pruebe |
| l | **Son 7 tablas, no 6** — §14 y §16 dejaban `uniform_settings` fuera del test de RLS por un error de conteo | Corregido a 7 |
| m | **El folio no es implementable**: una `SEQUENCE` no es por tenant ni respeta RLS. Y `^U-\d{4}-\d{6}(-[A-Z])?$` **limita un pedido a 26 entregas** | `uniform_folio_counters(tenant_id, year, next)` con `UPDATE … RETURNING`; sufijo numérico |
| n | **`PICKING` no tiene salida hacia atrás**: si `CreateCharge` falla de forma permanente, la entrega queda apartada **para siempre**. Y el barrido diario no puede resolver en <24 h una alerta que dispara a los 15 min | `PICKING → CANCELLED` tras N intentos + **outbox drenado cada 5 min** por scheduler, no diario |
| o | **`AdjustStock` es el endpoint de merma** — el que se usa para tapar un faltante — bajo el `uniforms:write` genérico | Permiso propio, nota obligatoria, alerta por ajuste negativo |
| p | **Prioridad FIFO vs mostrador sin definir**: mostrador puede llevarse stock que FIFO le debe a una familia que lleva 3 semanas esperando | El backorder tiene prelación; mostrador vende sobre `available` neto de compromisos |

### Estimación honesta

| Rebanada | Plan | Real | Por qué |
|---|---|---|---|
| 12a | 2.5 | **4** | + la migración de finance con ADR |
| 12b | 3.5 | **4.5** | tras rediseñar la máquina de estados (~1 d de diseño) |
| 12c | 3.5 | **6** | 7 tablas + 3 clientes + outbox + 2 jobs + suites de integración/RLS |
| 12d | 2 | **2.5** | + endurecimiento family (S-1, CSRF) |
| 12e | 3.5 | **6** | 4 pantallas, lote, 5 estados cada una |
| 12f | 2 | **2.5** | — |
| 12g | 1.5 | **4** | el inventario real de Terraform |
| **Total** | **18.5** | **~30** | ≈6 semanas de una persona |

## Tablas de consenso (voces duales · `[subagent-only]`, codex no disponible)

```
CEO — CONSENSO
════════════════════════════════════════════════════════════════
  Dimensión                             Claude  Codex  Consenso
  ───────────────────────────────────── ─────── ────── ─────────
  1. Premisas válidas                     Sí     N/A   sin par
  2. Problema correcto                    Sí*    N/A   sin par   (*tras aceptar D1)
  3. Calibración de alcance               No     N/A   sin par   (18.5 d era irreal)
  4. Alternativas exploradas              Sí     N/A   sin par
  5. Riesgos de mercado                   N/A    N/A   no aplica (software interno de un colegio)
  6. Trayectoria a 6 meses                Sí     N/A   sin par
════════════════════════════════════════════════════════════════

DISEÑO — CONSENSO
════════════════════════════════════════════════════════════════
  Dimensión                    Principal  Voz indep.  Consenso
  ──────────────────────────── ────────── ─────────── ─────────
  1. Jerarquía de información     7/10       4/10     CONFIRMA hueco (la voz es más dura y tiene razón)
  2. Cobertura de estados         6/10       3/10     CONFIRMA hueco
  3. Coherencia del recorrido     7/10       5/10     CONFIRMA hueco
  4. Especificidad                8/10       3/10     DISCREPA → gana la voz (§8/§9 eran relleno)
  5. Sistema de diseño            8/10       5/10     DISCREPA → gana la voz (tabs/chips invertidos, faltan 3 componentes)
  6. Intención móvil              4/10       4/10     CONFIRMA
  7. Accesibilidad                4/10       3/10     CONFIRMA
════════════════════════════════════════════════════════════════

INGENIERÍA — CONSENSO
════════════════════════════════════════════════════════════════
  Dimensión                     Principal  Voz indep.  Consenso
  ───────────────────────────── ────────── ─────────── ─────────
  1. Arquitectura sólida           Sí         No       DISCREPA → gana la voz (B1 verificado)
  2. Cobertura de pruebas          Sí         No       DISCREPA → gana la voz (faltaban los 3 tests que matan el diseño)
  3. Riesgos de rendimiento        Sí         No       DISCREPA → gana la voz (índices no declarados, sin k6)
  4. Amenazas de seguridad         Parcial    No       CONFIRMA hueco (B6, CSRF, tope de qty)
  5. Rutas de error                Sí         No       DISCREPA → gana la voz (B4, B5)
  6. Riesgo de despliegue          Sí         No       DISCREPA → gana la voz (B7a, B7c, B7d verificados)
════════════════════════════════════════════════════════════════
```

**Temas que aparecieron en 2+ fases de forma independiente** (señal de alta confianza):
1. **La integración con finanzas está subestimada** — la fase CEO la marcó como acoplamiento a
   vigilar (A1/A2/A3), la de ingeniería demostró que es un 2PC no reconocido y que la restricción
   del libro mayor la hace imposible tal cual (B1). Es *el* módulo, no una viñeta de §4.
2. **La venta en mostrador es el punto más frágil** — diseño la calificó de crítica por el flujo
   humano (R7), ingeniería por la corrupción de inventario en la recuperación (B4). Coinciden sin
   haberse visto.
3. **Lo que el plan llamaba "estados" no estaba diseñado ni probado** — diseño lo puntuó 3/10,
   ingeniería encontró que el estado `degraded` que el propio backend fabrica no existía en ningún
   lado.

## Decisiones auto-decididas (principios 1–6)

| # | Decisión | Clasificación | Principio | Por qué |
|---|---|---|---|---|
| 1 | Aprobar el enfoque C (B + mostrador + calendario compartido) | mecánica | P1 completitud | Sin mostrador el inventario miente |
| 2 | `A3` **entra en alcance** como `finance.CancelChargeIfUnpaid` | mecánica | P1 + P5 | B5 demuestra que es el eje de la corrección; sin ella el test de la carrera es teatro |
| 3 | Partir `uniforms:read` en `uniforms:stock` (otorgable) + `uniforms:orders` (owner-only) | mecánica | seguridad | El criterio escrito de `ownerOnlyPermissions` aplica sin discusión (B6) |
| 4 | `config` caído al surtir ⇒ **falla el pick**, no degrada | mecánica | P1 | Diseño e ingeniería coincidieron: degradar acorta el plazo del padre en silencio |
| 5 | La fila de la tabla es la **entrega**, no el pedido; fuera la fila expandible | mecánica | P5 explícito | La columna de vencimiento no puede existir en una fila sin estado (R1) |
| 6 | Mostrador es **pestaña propia** + command palette, no FAB | mecánica | P5 | El FAB obliga a cargar la tabla entera con un padre de pie (R7) |
| 7 | Mostrador nace en `SOLD_AT_COUNTER`, terminal, intocable por el barrido | mecánica | P1 | Revierte mi E2: la misma máquina hacía que el barrido "devolviera" prendas ya entregadas (B4) |
| 8 | Outbox real drenado cada 5 min, no barrido diario | mecánica | P1 | Alertar a los 15 min sobre algo que tarda 24 h en resolverse es fatiga de alerta (B7n) |
| 9 | Tabs = estado, chips = nivel (como `StudentsScreen`) | mecánica | P4 DRY | Dos módulos hermanos no se manejan distinto sin razón (R3) |
| 10 | Reserva por sentencia condicional, sin `FOR UPDATE` | mecánica | P5 | Elimina el deadlock de pedidos multi-línea (B7h) |
| 11 | `ChargeSettled` en vez de `charge_status` en el evento | mecánica | P5 explícito | No duplicar estado derivado en el cable; evita suscribirse a todos los pagos del colegio (B7e) |
| 12 | E6–E9 diferidos a TODOS.md | mecánica | P3 pragmatismo | No bloquean el flujo |
