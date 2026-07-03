# Módulo Documentos (M09 / `module_docs`) — DESACTIVADO del panel (no borrado)

**Decisión de producto (binding, del usuario, 2026-07-02):** el colegio **no requiere** el
módulo de **Documentos** (constancias, kardex, boletas). Se **desactiva del panel** en toda la
UI, pero **NO se borra nada**: si en el futuro lo quieren, se reactiva quitando un marcador.

> **Regla:** "desactivado del producto" (`hidden`) ≠ "flag OFF" (`Próximamente`). Un módulo
> `hidden` **desaparece** del panel; un módulo con flag OFF se muestra **disponible-pero-deshabilitado**.
> Documentos es `hidden`, no solo flag OFF.

## Qué se conserva (future-ready, intacto)
- `services/docs/` (servicio), `proto/garcialorca/docs/v1/docs.proto` (contrato) — sin tocar.
- La flag `module_docs` sigue en el registro (`.claude/docs/FLAGS.md`) y en el gateway.
- Las entradas de datos del módulo (web `MODULES`, mobile `modules`) — se conservan, solo marcadas.

## Cómo se implementó la desactivación
- **Web** (`apps/web`):
  - `app/_design/data.ts`: `ModuleDef.hidden?: boolean`; entrada `documents` → `hidden: true`.
  - `app/_screens/Dashboard.tsx`: el grid filtra `MODULES.filter((m) => !m.hidden)`.
  - `app/command/registry.ts`: se salta `module.hidden` en nav **y** acciones → el Command
    Palette nunca expone Documentos, **aunque `module_docs` esté ON**. Ver [[command-palette]].
- **Mobile** (`apps/mobile`):
  - `src/data/mock.ts`: `ModuleTile.hidden?: boolean`; tile `documentos` → `hidden: true`.
  - `src/screens/MoreScreen.tsx`: el grid "Más" filtra `modules.filter((m) => !m.hidden)`.
  - `src/flags/bootstrap.ts`: `module_docs: false` (coherente con "no se ofrece").

## Cómo REACTIVARLO en el futuro
Quitar `hidden: true` de la entrada `documents` (web) y `documentos` (mobile), y poner la flag
`module_docs` en ON donde corresponda. Nada más — servicio, proto y contrato ya están.

Regresiones cubiertas: `Dashboard.test.tsx` (oculto no se renderiza), `registry.test.ts`
(no aparece ni con flag ON), `MoreScreen.test.tsx` (tile ausente en "Más").
