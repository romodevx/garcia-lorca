# ADR 0010 — La primera escritura de un tutor en la plataforma

- **Estado:** aceptado (2026-08-25)
- **Contexto del módulo:** M12 Uniforms · permisos `uniforms_own:write`

## Contexto

Hasta hoy, **un tutor no escribe nada** en este sistema. La app de familia lee: avisos,
calificaciones, asistencia, estado de cuenta. Incluso marcar un aviso como leído lo ejecuta el
gateway en su nombre, deliberadamente, para no tener que otorgarle `comms:write`
(está comentado así en `services/gateway/internal/handler/family.go`).

El módulo de uniformes rompe eso: el padre **crea un pedido desde la app**, y ese pedido
**aparta mercancía física y genera un cargo en su estado de cuenta**. Es una obligación
financiera creada por un principal que hasta ahora solo tenía permisos de lectura.

Eso no es una fila más en una migración de permisos. Es un cambio en el modelo de confianza.

## Decisión

Se crean dos permisos para la superficie de familia:

- **`uniforms_own:read`** — ver los pedidos de *sus* hijos.
- **`uniforms_own:write`** — crear un pedido y cancelar líneas que todavía no se surten.

Ambos se otorgan al rol `guardian`. Y se aceptan **cuatro condiciones**, no negociables:

1. **El permiso no dice de quién.** Dice "este principal puede actuar sobre los pedidos de *sus*
   hijos". La pertenencia la impone el vínculo tutor→alumno en cada llamada, igual que
   `/api/v1/family/*` (ADR 0004). Un id ajeno responde **404, nunca 403**: un 403 confirma que el
   pedido existe.
2. **El alcance se impone en el servicio, no solo en el borde.** `GetOrder` y `CancelOrderLines`
   reciben el conjunto de hijos del principal como parámetro **obligatorio** del RPC. Si la
   defensa fuera un `if` en el gateway, el servicio serviría cualquier pedido por id a quien
   tuviera credencial de servicio.
3. **El precio nunca viene del cliente.** El cuerpo del request lleva prenda, talla y cantidad;
   el importe se toma **siempre** del catálogo del servidor. Hay una prueba de regresión dedicada.
4. **Hay techos, y son configurables.** Pedidos abiertos por alumno, líneas por pedido, unidades
   por línea, y vencimientos por línea antes de dejar de ser reservable. Sin ellos, un tutor aparta
   el inventario completo de una talla durante diez días hábiles, gratis, y lo puede repetir.

Además, la ruta de creación exige **CSRF** (la web usa sesión por cookie) y **rate limit por
principal, no por IP** — las familias detrás del NAT del colegio comparten IP, y un atacante con
datos móviles la rota.

## Consecuencias

- La app de familia deja de ser de solo lectura. La siguiente escritura que alguien quiera agregar
  ya tiene precedente y checklist: alcance en el RPC, 404 en vez de 403, precio del servidor, techos.
- `uniforms_own:*` es **grantable** a propósito: un colegio puede tener más de un rol con forma de
  tutor. No es owner-only porque no resuelve datos de ningún alumno que no sea el propio.
- Queda pendiente el hueco conocido de *grant-subset* (quien tiene `roles:write` puede fabricarse
  cualquier permiso no owner-only). Este ADR no lo cierra; lo hace más urgente y lo deja anotado.

## Relacionado

- ADR 0004 (lecturas self-scoped de familia) — de ahí viene la postura de 404 sobre 403.
- ADR 0006 (alcance docente en asistencia) — el precedente de permisos `_own`.
- `.claude/docs/PLAN-m12-uniforms.md` §7 y hallazgos S-1, S-2, S-3, S9.
