# Command Palette — el buscador con superpoderes (cross-cutting, SIEMPRE presente)

**Decisión de producto (binding, del usuario):** el buscador de la app NO es una caja de
texto que filtra una lista — es un **Command Palette omnidireccional**: el centro de
**navegación + ejecución de acciones** con el **mínimo de clics**. El usuario no entra a
"Estudiantes" y luego "Crear alumno": escribe `crear alumno` y ya. Busca `pago Juan` y abre
el cobro prellenado. **Todo está a UN buscador.** Ver [[ux-minimum-clicks]].

> **Regla permanente:** cada módulo y cada funcionalidad nueva DEBE registrar sus entradas
> en el Command Palette como parte de su build (DoD). Si un módulo no aparece en el buscador,
> no está terminado.

> **Diseño revisado por `/plan-eng-review` (2026-06-17).** Las decisiones de abajo superan
> al draft original (ya NO hay federación por-servicio ni registro 100% cliente).

## Atajo y comportamiento
- Global **Cmd+K / Ctrl+K** (el buscador del TopBar abre el mismo palette).
- Construido sobre **`cmdk`** (lib headless estándar: teclado/foco/ARIA/fuzzy probados); se
  envuelve con el diseño García Lorca. NO hand-rollear el a11y.
- Búsqueda **fuzzy con folding de acentos/diacríticos** (`maria`→María, `nino`→niño, `cal`→
  Calificaciones). Esto es obligatorio: es lo #1 que rompe la búsqueda en español.
- 100% navegable por teclado (↑↓/Enter/Esc), máx ~8–10 resultados, agrupados, con **iconos
  y colores del dashboard** (fuente: `apps/web/app/_design/data.ts`; keying canónico por
  flag/`module`, no por el `id` del grid).
- Estados de **vacío / sin-resultados / error** diseñados explícitamente.
- Mobile responsive (Expo: modal nativo). El **core (tipos del registro + motor de match)
  es agnóstico de plataforma** y vive en un paquete compartido; cada app solo aporta su UI.

## Modelo de matching (simple, sin tarpit de sinónimos)
- Se matchea contra el **título i18n ya traducido** + un **set chico de alias fijos** por
  comando, y el fuzzy hace el resto. NO listas grandes de sinónimos bilingües por comando
  (driftan de las etiquetas reales y no escalan). Alias solo donde el fuzzy no alcanza.

## Modelo de resultados en dos capas
- **Capa A — Comandos (instantáneo):** módulos + acciones de **crear** + acciones **rápidas**.
  **El registro se ensambla SERVER-SIDE (RSC) por usuario + flags + rol** y se envía solo lo
  permitido. NO es un registro 100% cliente: eso filtraría en el bundle las rutas/acciones de
  módulos que el usuario no puede ver (billing, permisos…). El match fuzzy sí corre en cliente
  sobre lo ya autorizado.
- **Capa B — Registros (server-backed, debounced ~150ms):** alumnos, pagos, avisos, etc.
  **Vía un ÍNDICE de búsqueda propio** que mantiene el gateway/read-model (Postgres FTS /
  `pg_trgm`), **alimentado por eventos** de los servicios. Ranking con escala única, RLS y
  filtrado por permiso en UN solo lugar, latencia sub-10ms. **NO federación por-servicio**
  (fan-out por tecla = N+1, cold-start con scale-to-zero, fallos parciales, ranking sin escala
  común). **Los módulos NO exponen `Search` RPC — emiten eventos al índice.**

## Seguridad (no negociable)
- **Registro Capa A por-flag/rol server-side** (arriba): el bundle nunca lleva el mapa de
  acciones/rutas de módulos que el usuario no puede ver.
- **Historial de recientes**: guarda solo IDs de comando/navegación, **nunca datos de
  registros** (un usuario que pierde permiso no debe conservarlos en localStorage). Los
  "registros recientes" se renderizan server-side con chequeo de permiso vivo.
- **Búsqueda de registros filtrada por IAM + RLS server-side**: un maestro que busca un alumno
  que no puede ver obtiene **nada** (no "existe pero sin acceso"); cuidar fugas por conteo,
  timing y fragmentos de autocomplete. **Rate-limit** (vector de enumeración/DoS).
- **El palette NUNCA ejecuta mutaciones.** Una acción **navega al form prellenado** (ej.
  `pago Juan` abre el cobro con Juan prellenado; el usuario confirma ahí). Cero movimientos de
  dinero/datos desde un match fuzzy. Sigue siendo ~2 clics, pero seguro y auditable.

## Orden de prioridad de resultados
1. **Módulo** (navegación) que matchea fuerte. 2. **Crear** relevante. 3. **Recientes**.
4. **Registros** por coincidencia. 5. Sub-acciones. (Las acciones de crear se muestran, pero
ejecutar = navegar al form; nunca mutar directo.)

## EL CONTRATO — qué registra cada módulo
Cada slice de frontend exporta un `ModuleCommands`. La parte **estática (nav/create/quick) es
firme** (deriva de `data.ts`). La parte de **`records` es PROVISIONAL**: se **extrae** del
primer módulo real (students/M03) y se congela ahí, NO se diseña a ciegas antes de tener un
consumidor.
```ts
{
  module: 'students',
  flag: 'module_students',                 // ensamblado server-side; OFF → no se envía
  nav:    { titleKey, icon, color, route },                 // "estudiantes" → listado
  create: [{ titleKey, aliases?, icon, route }],            // navega a un form (nunca muta)
  quick:  [{ titleKey, aliases?, icon, route }],            // navega a la acción prellenada
  records?: {  /* PROVISIONAL hasta M03 */                  // alimenta el índice vía eventos
    type: 'student',
    toResult: (hit) => ({ title, subtitle, icon, route }),  // cómo se pinta un hit del índice
    quickActions?: [{ titleKey, icon, route }],             // "ver adeudos" → navega
  }
}
```
`titleKey`/`aliases` son claves de **[[i18n-bilingual]]** (`@garcia-lorca/i18n`): título + alias
es+en. El backend devuelve **códigos/registros**; el palette pinta el texto localizado.

## Secuencia (del review)
- **CP-1 (ahora):** shell `cmdk` + registro server-assembled + Capa A (nav/create/quick estáticos
  desde `data.ts`) + fuzzy con folding + i18n + estados. Reemplaza el buscador del TopBar.
- **CP-2 (con M03):** índice de búsqueda + records de **students** (una fuente) → aquí se
  **extrae y congela** la parte `records` del contrato.
- **CP-n:** cada módulo emite sus eventos al índice + registra sus comandos (parte de su DoD).

Catálogo de búsquedas por módulo (semilla): `.claude/docs/design/PLAN-command-palette.md`.
