# Inventario de sombras (F2a, rediseño del Inicio)

- **Fecha:** 2026-10-11 · rama `feature/redesign-home`.
- **Cómo se hizo:** por grep, no a mano. Toda expresión `boxShadow:` en `apps/web/app/**/*.{ts,tsx}` (sin pruebas) cuyo texto lleva px, más las declaraciones `box-shadow:` de `globals.css`. Línea base: el commit `685f1969`, antes de tocar nada.
- **Quién lo vigila:** `apps/web/app/_design/shadows.test.ts` falla si aparece una sombra con px fuera de la familia (c). Las sombras que ya usan token (`var(--shadow-card)`, `--shadow-hover`, `--shadow-pop`) no entran en el conteo.

## Conteo por familia

| Familia | Qué es | Antes (línea base) | Después | Dónde vive ahora |
|---|---|---|---|---|
| (a) halo de ícono | `0 6px 16px color-mix(<color> 38%/40%, transparent)` bajo el cuadro de color | **17** en 17 archivos (16 encabezados de módulo + tarjeta de módulo del inicio) | 0 en componentes | `--shadow-icon`, declarado en `.module-header-icon` y `.module-card-icon` |
| (b) elevación | FAB, botón primario, botón "Registrar pago", burbuja de ayuda, logo sobre fondo oscuro | **8** en 8 archivos | 0 en componentes | tokens en `:root`, ver abajo |
| (c) marcas, no profundidad | franjas `inset` de fila y anillos `0 0 0 Npx` de estado | **12** en 12 archivos (4 franjas, 8 anillos) | 12, sin cambio | literales, a propósito |
| **Total** | | **37** en 28 archivos | 12 (sólo familia c) | |

## Familia (b): token por valor

| Token | Valor (idéntico al de antes) | Usos |
|---|---|---|
| `--shadow-fab` | `0 10px 30px color-mix(in oklab, var(--lime) 45%, transparent)` | FAB de Estudiantes, Egresos, Becas, Actividades |
| `--shadow-primary` | `0 4px 14px color-mix(in oklab, var(--lime) 40%, transparent)` | `Btn` primario (`Controls.tsx`) |
| `--shadow-primary-sm` | `0 3px 10px color-mix(in oklab, var(--lime) 36%, transparent)` | "Registrar pago" (`FinanceScreen.tsx`) |
| `--shadow-tip` | `0 8px 24px rgba(0,0,0,.18)` | burbuja de `HelpTip` |
| `--shadow-logo-badge` | `0 6px 20px rgba(0,0,0,.18)` | `LogoBadge` (`Brand.tsx`) |

`--shadow-pop` ya existía con otro valor y cambia con el tema (`0 12px 40px rgba(16,40,24,.16)` en claro). Fundir la burbuja o el logo en él sería un cambio visual, así que eso le toca a F2b.

## Familia (c): se queda literal

| Tipo | Sitios |
|---|---|
| Franja `inset 3px 0 0` / `inset 4px 0 0` | `ExpensesScreen`, `ScholarshipsTable`, `ActivitiesTable`, `PassSheetRow` |
| Anillo de punto de estado `0 0 0 3px color-mix(… 22%)` | `StudentsScreen`, `FinanceScreen`, `finance/parts`, `AdminScreen` |
| Anillo de campo inválido `0 0 0 3px color-mix(var(--red) 22%)` | `Field`, `InlineCreateRow` |
| Anillo de separación `0 0 0 2px var(--surface)` / `0 0 0 3px …, 0 0 0 5px var(--lime)` | insignia de `IconBtn` (`Controls`), avatar con anillo (`Brand`) |

La prueba reconoce la familia (c) por su forma y no por archivo: cada capa es `inset …` o `0 0 0 <n>px …`. Una capa con desplazamiento o desenfoque no pasa, aunque venga junto a un anillo.

El velo del escáner QR (`0 0 0 100vmax`) no lleva px y queda fuera del conteo.
