# García Lorca — Sistema de Diseño (mandato para todos los módulos)

> **Fuente de verdad:** proyecto Claude Design `7162d625-0893-4d92-aa4f-a7cfc5281951`
> (leer con el MCP `DesignSync`). El `globals.css` del web **ya es** el `styles.css`
> del diseño (tokens idénticos), las fuentes (Poppins display + DM Sans UI), el set
> completo de `_design/Icon.tsx` y los controles base (`_design/Controls.tsx` = `Btn`,
> `ThemeToggle`) también. **La base está lista; el trabajo es que cada superficie de
> módulo use este lenguaje.** No inventar estilos nuevos: usar los tokens.

## Regla de oro
**Menor cantidad de clics posible.** Alta rápida en un solo drawer con campos
mínimos y valores por defecto; selectores por chips (no dropdowns cuando se puede);
guardar en 1 clic; FAB siempre visible; acciones por fila. El sistema más amigable
y fácil de usar posible.

## Anatomía de una superficie de módulo (patrón obligatorio)
Referencia canónica: `screen_students.jsx` del proyecto de diseño (Estudiantes).

1. **Volver al inicio** (link con ícono `back`, `text-muted`).
2. **Header:** cuadro redondeado (`--r-lg/18px`) con ícono del módulo en `--forest`
   + `<h1>` 30px/800 + subtítulo `text-muted` ("N alumnos · Ciclo 2025–2026").
   A la derecha: `Btn kind="ghost" icon="download"` (Exportar) + `Btn icon="plus"`
   (acción primaria, lima).
3. **Stat cards:** fila flex de tarjetas (`--r-lg`, `--shadow-card`), cada una con
   cuadro de ícono en color suave (`color-mix(... 16%, --surface)`), label, número
   grande (Poppins 22/700) y sub-label `text-faint`.
4. **Filtros:** grupo de tabs tipo píldora con contadores (fondo `--surface-3`,
   activo = `--surface` + `--shadow-card`) + búsqueda pill + toggles.
5. **Chips de filtro** (nivel/grupo): píldoras con borde; activo = borde de color +
   `color-mix(... 12%, --surface)`. Botón "Limpiar" cuando hay filtro activo.
6. **Barra de selección** (cuando hay filas marcadas): banda lima con acciones masivas.
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
