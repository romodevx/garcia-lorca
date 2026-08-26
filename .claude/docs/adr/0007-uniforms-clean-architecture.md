# ADR 0007 — El módulo de uniformes usa Clean Architecture, no el layout *thin*

- **Estado:** aceptado (2026-08-25)
- **Contexto del módulo:** M12 Uniforms · rebanada 12b

## Contexto

`CLAUDE.md` clasifica los servicios en dos layouts, y la regla que los separa es explícita:
**"la arquitectura se gana, no se impone"**.

- **Clean Architecture completa** (`domain/`, `usecase/`, `ports/`, `adapters/`) para dominios
  ricos: `finance`, `billing`, `grades`, `scholarships`, `students`, `iam`.
- **Thin** (`handler → service → repo`) para CRUD e I/O: `attendance`, `docs`, `comms`,
  **`uniforms`**.

Esa clasificación se hizo en FASE 0, cuando uniforms era un stub y "tienda de uniformes" sonaba a
un catálogo con existencias. El módulo que salió de la revisión no se parece a eso.

## Lo que uniforms resultó tener

1. **Una máquina de estados con siete estados y transiciones prohibidas.** La entrega se surte, se
   cobra, vence, se despacha o se cancela, y `sold_at_counter` es terminal desde que nace
   precisamente para que ningún barrido la toque.
2. **Invariantes de inventario que cruzan tablas.** `reserved <= on_hand` obliga a que la venta
   mueva los dos contadores en una sola sentencia; el descuento físico ocurre exactamente una vez;
   la liberación es idempotente. Son reglas del dominio, no del repositorio.
3. **Aritmética de días hábiles** contra el calendario escolar y una zona horaria, con un plazo
   que solo puede extenderse, nunca acortarse.
4. **Asignación FIFO** de la mercancía que llega contra la cola de líneas en espera.
5. **Un estatus derivado** que se calcula de líneas y entregas y jamás se almacena.
6. **Coordinación con otro contexto acotado** (finanzas) sin transacción distribuida.

Nada de eso es CRUD. Puesto en un layout *thin*, todo termina en el `service`, que es donde el
dominio va a morir: mezclado con GORM, con clientes RPC y con `context`, imposible de probar sin
base de datos, y con las invariantes repartidas entre el repositorio y el handler.

## Decisión

`services/uniforms` adopta **Clean Architecture completa**:

```
internal/
  domain/     máquina de estados, invariantes de stock, días hábiles, FIFO,
              derivación de estatus. Sin GORM, sin proto, sin context de red.
  ports/      las costuras que el caso de uso consume (repositorio, lectores de
              finance/students/config, publicador de eventos).
  usecase/    orquesta: valida referencias cross-service, abre transacciones,
              y le pide las decisiones al dominio.
  adapters/
    postgres/ GORM vive solo aquí. Doble modelo + mapper.
    grpc/     traduce el contrato generado.
    pubsub/   traduce eventos.
    financeclient/ studentsclient/ configclient/
```

## Consecuencias

- Las reglas más delicadas del módulo (no perder mercancía, no cobrar dos veces) se prueban
  **unitariamente, sin Docker y sin reloj**, que es la única forma de probarlas exhaustivamente.
- `CLAUDE.md` queda desactualizado en su tabla de layouts. **Se actualiza en la misma rebanada**;
  una regla que el código contradice es peor que no tenerla.
- El costo real es un puñado de tipos de mapeo. Con la máquina de estados en el dominio, ese costo
  se paga solo la primera vez que hay que responder "¿por qué se liberó esta pieza?".
- El precedente queda: `attendance` fue thin con razón y sigue siéndolo. La clasificación de FASE 0
  no es un contrato, es una hipótesis que cada módulo confirma o refuta al construirse.

## Alternativa descartada

**Quedarse en thin y "tener disciplina".** Es la opción que siempre gana en el papel y siempre
pierde en el segundo mes: sin una frontera que lo impida, el `service` acaba importando GORM porque
en ese momento era lo más corto. La frontera es el mecanismo; la disciplina no lo es.

## Relacionado

- `.claude/docs/PLAN-m12-uniforms.md` §6.
- `CLAUDE.md`, sección "Arquitectura por servicio".
