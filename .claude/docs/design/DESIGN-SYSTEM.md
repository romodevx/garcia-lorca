# García Lorca — Sistema de Diseño (mandato para todos los módulos)

> **Fuente de verdad:** proyecto Claude Design `7162d625-0893-4d92-aa4f-a7cfc5281951`
> (leer con el MCP `DesignSync`). El `globals.css` del web **ya es** el `styles.css`
> del diseño (tokens idénticos), las fuentes (Poppins display + DM Sans UI), el set
> completo de `_design/Icon.tsx` y los controles base (`_design/Controls.tsx` = `Btn`,
> `ThemeToggle`) también. **La base está lista; el trabajo es que cada superficie de
> módulo use este lenguaje.** No inventar estilos nuevos: usar los tokens. La única
> capa nueva aprobada es el **look refresh** (sección al final), y vive en tokens.

## Regla de oro
**Menor cantidad de clics posible.** Alta rápida en un solo drawer con campos
mínimos y valores por defecto; selectores por chips (no dropdowns cuando se puede);
guardar en 1 clic; FAB siempre visible; acciones por fila. El sistema más amigable
y fácil de usar posible.

## Anatomía de una superficie de módulo (patrón obligatorio)
Referencia canónica: `screen_students.jsx` del proyecto de diseño (Estudiantes).

1. **Volver al inicio** (link con ícono `back`, `text-muted`).
2. **Header:** `ModuleHeaderIcon` (cuadro de 56px, radio 18px, color sólido del
   módulo con halo `--shadow-icon`; tintado con el look refresh, ver abajo)
   + `<h1>` 30px/800 + subtítulo `text-muted` ("N alumnos · Ciclo 2025–2026").
   A la derecha: `Btn kind="ghost" icon="download"` (Exportar) + `Btn icon="plus"`
   (acción primaria, lima).
3. **Stat cards:** fila flex de tarjetas (`--r-lg` = 22px; 16px con el look
   refresh — antes este documento decía 18px, D16 — y `--shadow-card`), cada una con
   cuadro de ícono en color suave (`color-mix(... 16%, --surface)`), label, número
   grande (Poppins 22/700) y sub-label `text-faint`.
4. **Filtros:** grupo de tabs tipo píldora con contadores (fondo `--surface-3`,
   activo = `--surface` + `--shadow-card`) + búsqueda pill + toggles.
5. **Chips de filtro** (nivel/grupo): píldoras con borde; activo = borde de color +
   `color-mix(... 12%, --surface)`. Botón "Limpiar" cuando hay filtro activo.
6. **Barra de selección** (cuando hay filas marcadas): banda lima con acciones masivas.
7b. **Paginación** (footer de la tabla, `borderTop`): "Mostrando X–Y de N" +
   selector "Mostrar 20/50/100" (píldoras) + botones Anterior/Siguiente + "pág / total".
   Reset a la página 1 al cambiar filtros o tamaño de página. Toda tabla larga la lleva.
7. **Tabla de datos** (`--r-xl`, `--shadow-card`): header en mayúsculas `text-faint`;
   filas con barra de acento a la izquierda (color por estado), checkbox, `Avatar`
   (inicial en círculo de color), nombre + id `text-faint`, columnas, **píldora de
   estado** (punto + label, `--ok/warn/bad-bg/fg`), y botones de acción circulares
   (mensaje + chevron). Fila hover = `--surface-2`, click abre ficha (drawer).
8. **Drawer de alta rápida** (slide-over derecho, `animation: slideIn`): fondo
   `rgba(6,20,12,.5)` + blur; panel `--surface`; header con ícono; campos mínimos;
   **nivel/grupo/beca como chips**; nota de "qué pasa al guardar"; footer Cancelar +
   Guardar (1 clic).
9. **Ficha rápida** (drawer derecho más angosto): avatar + estado + filas clave +
   acciones (Documentos / Contactar).
10. **FAB** flotante abajo-derecha (píldora lima con sombra glow) para la acción primaria.
11. **Toast** de confirmación (píldora `--forest`, `animation: pop`) tras cada acción.

## Componentes reutilizables a construir en `apps/web/app/_design/`
Portar del diseño (mismos patrones de inline-style + tokens):
- `Avatar` (inicial/es en círculo de color) — del `shared.jsx`.
- `IconBtn` (botón circular con badge).
- `StatCard`, `EnrollPill`/`StatusPill`, `FilterTabs`, `FilterChips`, `SlideOver`
  (drawer), `Fab`, `DataTable` genérica.
Reutilizar lo existente: `Btn`, `ThemeToggle`, `Icon`, `TopBar`, `Emblem`, `Toast`.

## Estados / colores
- Estado inscripción → píldora: `active`→Inscrito (forest/ok), `pending`→Trámite
  (yellow/warn), `inactive`→Baja (red/bad).
- Beca → píldora morada (`--purple`) con ícono `scholarship`.
- Acento primario = `--lime`/`--primary`; secundario = `--forest`.

## Datos reales (adaptar el diseño a lo que existe)
El diseño usa campos demo (phone, scholarship, matrícula GL-xxxx). Mapear al modelo
real: `name`=firstName+lastName, `id`=CURP (o id corto), `level`, `grade`=gradeGroup,
`guardian`=guardianName, estado=admissionStatus. Campos inexistentes (teléfono, beca)
se muestran como "—" u se omiten hasta que exista el backend.

## Aplicar a estos módulos (mismo patrón)
Estudiantes (referencia) → Finanzas → Config (catálogo) → Asistencia →
Calificaciones → Administración → Dashboard. Cada uno: header + stats + filtros +
tabla/lista + drawer de alta + FAB, siempre con el mínimo de clics.

## Look refresh (`module_ui_refresh`)

Estilo "Captions adaptado" (PLAN-redesign-home §3.3, decisiones D2–D3) para **toda la
app**, detrás de un solo flag (default OFF). Vive en `globals.css` bajo
`html[data-look="refresh"]`, dentro de `@media screen` (lo impreso queda como hoy).
`<LookAttribute/>` pone el atributo según el flag y el script previo al pintado lo
repite desde `localStorage` `gl-look`. Con el flag apagado no aplica ni una regla
(`look.off.characterization.test.tsx`); lo que sí aplica lo prueba
`look.refresh.test.tsx`, en claro y oscuro.

**Regla para componentes:** nada de estilos en línea salvo propiedades personalizadas
(`--mod-c`, `--icon-c`, `--kpi-c`…): un estilo en línea le gana a la capa y la tarjeta
se queda con el look viejo. Las sombras van en tokens (`shadows.test.ts`).

### Tokens nuevos o redefinidos

| Token | Claro | Oscuro | Uso |
|---|---|---|---|
| `--bg` | `#FBFBF8` (papel) | `#07150D` (igual) | fondo de página |
| `--bg-grad` | grano SVG (`feTurbulence`, opacidad .05) + lavado verde arriba | grano .07 + lavado forest | `.app-root`; sin `background-attachment: fixed` |
| `--hair` | `#E2E9DF` | `#1F3828` | línea fina de tarjetas y cápsula. Si el proyector las pierde → `--border-strong` [D13] |
| `--shadow-card` | `none` | `none` | las tarjetas descansan planas sobre `--hair` |
| `--halo` | `0 14px 34px -12px` lima al 38% | igual, `rgba(122,196,0,.10)` | sombra de hover de tarjeta. **Nunca** con desplazamiento 0 [D4] |
| `--r-lg` | 16px | 16px | radio de tarjeta (sin el flag: 22px) |
| `--topbar-h` | 78px | 78px | alto de la barra (ya existía, F2a) |
| `--topbar-float` | 12px | 12px | distancia de la cápsula a los bordes |
| `--topbar-clearance` | `--topbar-h` + 12px desde 560px de alto; 0 abajo | igual | lo que tapa la barra pegada (la barra "Paso N de 8" se pega ahí) |
| `--icon-tint` / `--icon-ink-share` | 13% / 60% | igual | cuadro tintado (abajo) |
| `--focus-ring` | `var(--forest)` | `var(--lime)` | anillo de foco |
| `--font-serif` | Libre Baskerville 400 (`next/font`, sin precarga) | igual | sólo el saludo, con `.look-greeting` (F3) |

### Piezas

- **Tarjeta (módulo y KPI):** borde `1px solid var(--hair)`, sin sombra. Hover:
  `box-shadow: var(--halo)` + borde `color-mix(in oklab, <color de la tarjeta> 45%, var(--hair))`.
  F3 cambia el color de la tarjeta por el del **área**.
- **Barra superior:** cápsula flotante a 12px de los bordes, radio 28px, borde `--hair`,
  `blur(28px) saturate(180%)`; con `@supports not (backdrop-filter…)` es opaca
  (`--surface`). Pegada (`top: 12px`) sólo con `min-height: 560px`; ahí también
  `html { scroll-padding-top: calc(var(--topbar-h) + 16px) }` (WCAG 2.4.11).
- **Cuadro de ícono tintado** (`ModuleHeaderIcon`, ícono de tarjeta y de KPI): fondo
  `color-mix(in oklab, color 13%, --surface)`, glifo `color-mix(in oklab, color 60%, --text)`,
  sin halo. El glifo se jala 40% hacia el texto porque el color puro sobre su tinte no
  llega a 3:1 (lima, amarillo y naranja en claro; forest y morado en oscuro). El gris
  neutro se queda `--surface-3` + `--text-muted`.
- **Anillo de foco:** `outline: 2px solid var(--focus-ring); outline-offset: 2px`.
  ≥ 3:1 contra `--surface` y `--bg` en los dos temas (WCAG 1.4.11) [D10]. Sin el flag
  sigue el de hoy (lima al 55%, ~1.6:1).
- **Colores forzados:** `@media (forced-colors: active)` → borde `1px solid CanvasText`
  en tarjetas, en reposo y en hover.

### Regla del lima [D4]

Lima = **lo primario** (botón primario, FAB y su resplandor), **lo seleccionado** y
**el halo de hover**. Nada más: el borde de hover usa el color de la tarjeta, el lavado
del fondo es verde forest y el anillo de foco en claro es `--forest`.
