# Command Palette — el buscador con superpoderes (cross-cutting, SIEMPRE presente)

**Decisión de producto (binding, del usuario):** el buscador de la app NO es una caja de
texto que filtra una lista — es un **Command Palette omnidireccional**: el centro de
**navegación + ejecución de acciones** con el **mínimo de clics**. El usuario no entra a
"Estudiantes" y luego "Crear alumno": escribe `crear alumno` y ya. Busca `pago Juan` y lo
cobra. **Todo está a UN buscador.** Ver [[ux-minimum-clicks]].

> **Regla permanente:** cada módulo y cada funcionalidad nueva DEBE registrar sus entradas
> en el Command Palette como parte de su build. No es opcional ni "después". Si un módulo
> no aparece en el buscador, no está terminado.

## Atajo y comportamiento
- Global **Cmd+K / Ctrl+K** (y el buscador del TopBar abre el mismo palette).
- Búsqueda **fuzzy** (`cal` → "Calificaciones"; `est` → "Estudiantes").
- 100% navegable con teclado (↑↓ / Enter / Esc), máx ~8–10 resultados, agrupados con
  **iconos y colores del dashboard** (fuente de verdad: `apps/web/app/_design/data.ts`).
- **Historial** de recientes (acciones y registros consultados).
- Mobile responsive (Expo: equivalente nativo, modal de búsqueda; web es la superficie primaria).

## Modelo de resultados en dos capas
- **Capa A — Comandos estáticos (instantáneo, client-side, sin red):** módulos +
  acciones de **crear** + acciones **rápidas**. Vienen de un registro tipado; se hace
  match fuzzy local. Cada módulo aporta su `ModuleCommands`.
- **Capa B — Registros (server-backed, debounced ~150ms):** alumnos, pagos, avisos,
  facturas, etc. El **gateway** federa un read-model `/search` que reparte a los servicios
  dueños de cada tipo y mezcla. **Filtrado por permisos (IAM) + RLS**: solo lo que el
  principal puede ver (un maestro no ve registros admin).

## Orden de prioridad de resultados (binding)
1. **Módulo** (navegación) que matchea fuerte.
2. **Acciones de CREAR** relevantes (ejecución rápida).
3. **Recientes** (historial / acceso frecuente).
4. **Registros** por coincidencia (fuzzy / full-text).
5. Sub-acciones y opciones secundarias.

Ejemplo `alumno` → [👤 Módulo Estudiantes] · [➕ Crear alumno] · [🕐 últimos consultados] · [🔄 Ver todos].
Ejemplo `juan` → [👤 Juan Pérez (perfil)] · [📋 sus calificaciones] · [💰 sus adeudos] · [📊 su asistencia] · [📄 sus documentos].

## EL CONTRATO — qué registra cada módulo (lo que se mantiene al crecer)
Cada slice de frontend de un módulo exporta un `ModuleCommands`:
```ts
{
  module: 'students',
  flag: 'module_students',                 // si el flag está OFF → 0 comandos (server autoritativo)
  nav:    { titleKey, icon, color, route },                 // "estudiantes" → listado
  create: [{ titleKey, keywordsKey, icon, route|action }],  // "crear alumno", "nuevo estudiante"…
  quick:  [{ titleKey, keywordsKey, icon, action }],        // "tomar asistencia", "generar constancia"…
  records?: {                              // Capa B (server-backed)
    type: 'student',
    toResult: (r) => ({ title, subtitle, icon, route }),    // cómo se pinta un registro
    quickActions?: [{ titleKey, icon, action }],            // "ver adeudos", "calificaciones"…
  }
}
```
Y cada **servicio backend** dueño de registros buscables expone un RPC `Search(query, ctx)`
que el gateway agrega — resultados tipados y **scoped por permisos/RLS**.

`titleKey`/`keywordsKey` son claves de **[[i18n-bilingual]]** (`@garcia-lorca/i18n`): títulos
y **sinónimos de búsqueda en es + en** ("crear alumno"/"nuevo estudiante"/"agregar"/"add student").
El backend devuelve **códigos/registros**, el palette pinta el texto localizado.

## Dónde vive
- Primitivo `<CommandPalette>` + `useCommandRegistry` en `@garcia-lorca/ui` (reemplaza el
  buscador cosmético del TopBar).
- El registro se ensambla en el composition root desde el `ModuleCommands` de cada módulo.
- `/search` read-model en el **gateway** (territorio M02 + read-models M13) para la Capa B.

## Mapa por módulo (semilla — crece con cada módulo; ejemplos del usuario)
Para el catálogo completo de búsquedas por módulo (estudiantes, finanzas, asistencia,
calificaciones, avisos, becas, documentos, uniformes, facturación, reportes, configuración)
ver `.claude/docs/design/PLAN-command-palette.md`. Cada módulo añade las suyas al construirse.
