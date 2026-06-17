# Plan técnico — `@garcia-lorca/i18n` (paquete compartido de i18n)

> Infraestructura cross-cutting del monorepo (NO un módulo, sin feature flag —
> como pnpm). Consumido por `apps/web` (next-intl) y `apps/mobile` (react-i18next).
> Rama: `chore/i18n-package`. TDD estricto. pnpm únicamente.
>
> **Revisado por `/plan-eng-review` (2026-06-16).** Ver `## GSTACK REVIEW REPORT` al final.

## Decisiones de checkpoint (2026-06-16)
1. **`auth` se siembra con claves de UI solamente.** El proto de iam aún NO está
   congelado (solo `Health`; `Login/ValidateToken/CheckPermission` son `TODO(M01)`,
   sin códigos de error). Los `error.auth.*` se agregan al congelar el contrato en M01.
2. **Type-safety por unión derivada de las claves.** Ver A2 (resuelto): `keyof typeof
   <catálogo-json-plano>` con `resolveJsonModule`. Cero codegen, catálogos en `.json`.
3. **Helper ICU framework-agnóstico: DECISIÓN ABIERTA — investigar.** Se prototipa
   y mide el costo real (mantener vs reusar) antes de comprometerlo como API pública.
   Spike timeboxed = primer paso del build. Estructurado para que mantener/eliminar
   sea un cambio de un archivo. Ver "Decisiones sin resolver".
4. **Locale ids: `es` (default) / `en`.** Region documentada, no ramificada. PERO el
   *locale de formato* que se alimenta al motor ICU es `es-MX` (correcto para MXN /
   fechas cuando lleguen `finance`/`invoices`). Catalog id ≠ locale de formato.
5. **Hermes Intl: verificar + polyfill ahora** en PASO C mobile (riesgo nuevo, abajo).

## Hallazgo colateral
`.claude/memory/i18n-bilingual.md` está indexado en `MEMORY.md` pero NO existe en
disco. Se recrea como parte de este trabajo (cierra el puntero muerto del índice).

## PASO A — preguntas resueltas

### A1. Un catálogo → dos bindings (next-intl web + react-i18next mobile)
- El paquete exporta **mensajes ICU MessageFormat crudos** (JSON plano por
  idioma/namespace) + **tipos** + (pendiente de spike) un **helper de interpolación
  ICU framework-agnóstico**.
- **NO importa** `next-intl` ni `react-i18next` (son dependencias de nivel app).
- ICU MessageFormat es el formato común. **Contrato de binding (correctitud, no opcional):**
  - **Web (next-intl):** consume ICU nativo. Sin configuración extra de interpolación.
  - **Mobile (react-i18next):** `i18next` usa por defecto interpolación `{{doble-llave}}`
    + `escapeValue:true`, que PELEA con el ICU de una sola llave. El binding mobile
    **debe** desactivar la interpolación nativa de i18next y poner `escapeValue:false`
    para que `i18next-icu` sea el único que formatea. (Hallazgo outside-voice — sin esto,
    el mismo mensaje rinde distinto en web vs mobile.)
- **Pinning de versiones:** `intl-messageformat` y `@formatjs/*` se declaran y pinnean
  en el paquete para que el formatjs que trae next-intl y el que trae `i18next-icu`
  coincidan. Un skew reintroduce divergencia de render.

### A2. Type-safety (resuelto — sin contradicción)
- Catálogos en `.json` **planos** (`{"button.save": "Guardar"}`, NO anidados).
- `resolveJsonModule: true`. La unión de claves se deriva con
  `type CommonKey = keyof typeof commonEs` — TS conoce las claves literales del JSON.
  (Los *valores* se ensanchan a `string`; no importa para seguridad de claves.)
- **Regla dura: claves planas**, verificada por el validador de paridad. Anidar
  dispararía "type instantiation excessively deep" cuando se multipliquen namespaces.
- Una clave inexistente falla en **compilación** (`MessageKey`), no en runtime.
- **Resolución de módulo:** el paquete define un `exports` map explícito y se
  **verifica que Metro (Expo) resuelva** los `.json` + helper, no solo Next/webpack.
  (La analogía con `packages/domain` es incompleta: domain no exporta JSON ni runtime.)

### A3. Namespaces por dominio
- `src/messages/<locale>/<namespace>.json`.
- Sprint actual: `common` (completo) + `auth` (solo UI).
- Futuro: cada módulo aporta su namespace (`finance`, `attendance`, `grades`, …).
- **Regla de propiedad de mensajes de validación:** los textos de validación de
  formularios viven en el namespace del módulo dueño (p.ej. `auth`), NO duplicados en
  `common`. `common` = navegación, botones genéricos, estados vacíos, toasts genéricos.

## PASO B — entregables (TDD: rojo → verde → refactor)

0. **Spike del helper (timeboxed, primer paso).** Prototipar el helper y medir el costo
   de mantenerlo en sync vs reusar el motor de cada framework. Decidir keep/drop.
   Reportar antes de finalizar la API pública.
1. `packages/i18n/` con:
   - `package.json` → `@garcia-lorca/i18n` con `exports` map explícito y
     `intl-messageformat` + `@formatjs/*` pinneados.
   - `tsconfig.json` (extiende `tsconfig.base.json`, `resolveJsonModule: true`).
   - tooling idéntico a `packages/domain`: ESM, `type:module`, `tsc --noEmit`,
     vitest, eslint 10.4.1, `@garcia-lorca/config`.
2. Catálogos ICU (JSON planos):
   - `src/messages/es/common.json`, `es/auth.json`
   - `src/messages/en/common.json`, `en/auth.json`
   - Default `es`; `en` secundario.
   - `common`: nav, botones (guardar/cancelar/buscar/…), estados vacíos, toasts.
   - `auth`: claves de UI del login (email, password, entrar, recordar, roles
     admin/maestro, errores de validación de formulario). **Sin `error.auth.*`** aún.
3. Tipos + helper:
   - `MessageKey` (unión de claves vía `keyof typeof`) + tipos de locale/namespace.
   - Helper `formatMessage(msg, values, locale)` — **sujeto al spike (paso 0)**;
     si se conserva, vía `intl-messageformat` con locale de formato `es-MX`/`en`.
4. **Validador de paridad (test) — alcance completo:**
   - Falla si una clave existe en `es` y no en `en` (o viceversa).
   - Falla si los **argumentos ICU** difieren entre idiomas (mismo set de
     `{var}`, mismo tipo: un `{count, plural}` en `es` debe ser `plural` en `en`,
     mismas ramas `select`).
   - Falla si un mensaje **no parsea** como ICU (`@formatjs/icu-messageformat-parser`).
   - Falla si falta la rama `other` en cualquier `plural`/`select` (ICU la exige).
   - Falla ante valores vacíos / solo-espacios.
   - Falla si alguna clave es **anidada** (regla de claves planas).

## PASO C — bindings por app (mínimo viable, no UI completa)
5. `apps/web`: setup de `next-intl` cargando desde `@garcia-lorca/i18n` + toggle de
   idioma (es ⇆ en), persistido. Locale de formato `es-MX`/`en`.
6. `apps/mobile`: setup de `react-i18next` (+ `i18next-icu`) cargando los mismos
   mensajes + toggle. **Contrato:** interpolación nativa de i18next OFF, `escapeValue:false`.
   - **Hermes Intl (riesgo nuevo, mitigado AHORA):** verificar la config Intl de Hermes
     en Expo SDK 56; agregar polyfill si faltan `Intl.PluralRules`/`Intl.NumberFormat`;
     test (jest-expo / on-device) que confirme que un mensaje **plural** rinde igual
     que en web. Cierra el riesgo antes de que `finance`/`grades` dependan de él.

## Cobertura de pruebas (objetivo 100% de codepaths)
```
[+] src/parity (validador)
  ├── key-parity es⇄en                    [TEST]
  ├── ICU arg-signature parity            [TEST]  (placeholders + plural/select)
  ├── ICU-parse válido                    [TEST]
  ├── rama `other` presente               [TEST]
  ├── no-vacíos / no-solo-espacios        [TEST]
  └── claves planas (no anidadas)         [TEST]
[+] src/keys (derivación de tipos)
  └── bad-key-fails-compilation           [TEST]  (// @ts-expect-error)
[+] src/format (helper — si el spike lo conserva)
  ├── interpolación simple                [TEST]
  ├── rama plural/select                  [TEST]
  └── mismo string ↔ ambos motores        [TEST → vitest, nivel formatjs]
[+] apps/mobile binding
  └── plural rinde en Hermes == web       [TEST → jest-expo / device]
```

## Riesgos
- **R-i18n-0 (NUEVO, alto):** Hermes (RN) sin Intl completo → plural/number/date
  truenan en dispositivo. Mitigado: verificar + polyfill + test plural en PASO C.
- R-i18n-1: i18next doble-llave/escape pelea con ICU → contrato de binding mobile
  (interpolación nativa OFF, `escapeValue:false`) + test cross-engine.
- R-i18n-2: `as const`/`keyof` profundo infla el tipo → regla de claves planas
  forzada por el validador.
- R-i18n-3: PASO C toca `apps/web` y `apps/mobile` (ya scaffolded) → cambios mínimos,
  sin romper el gate de source maps ni el arranque existente.
- R-i18n-4: resolución de `.json`/`exports` difiere entre Metro y Next → `exports`
  map explícito + verificación en ambos bundlers.

## Decisiones sin resolver (no defaultear en silencio)
- **Helper `formatMessage` keep/drop** — pendiente del spike (PASO B-0). Tensión
  cross-model: el usuario lo pidió (PASO B-3); la outside-voice lo considera un tercer
  motor ICU redundante. Resolución: investigar/prototipar y decidir con datos.

## Fuera de alcance (deferred, con razón)
- **UI completa de las apps** — solo binding + toggle mínimo viable.
- **`error.auth.*` y demás `error.*` codes** — se agregan al congelar cada contrato
  (iam en M01). El proto de iam hoy no está congelado.
- **Locale de formato es-MX en finance/invoices** — la regla queda documentada; el
  cableado de MXN/fechas se ejerce cuando esos namespaces lleguen (M05+), no ahora.
- **Cualquier feature flag** — i18n es cross-cutting, sin flag (como pnpm).
- **Namespaces de otros módulos** (`finance`, `grades`, …) — los aporta cada módulo.

## What already exists (reuso, no rebuild)
- `packages/domain/` — plantilla de tooling (ESM, `tsc --noEmit`, vitest, eslint
  10.4.1, `@garcia-lorca/config`). Se clona; cero tooling nuevo.
- `@garcia-lorca/ui` (`FlagsProvider`/`useFlag`) — el toggle de idioma sigue la misma
  forma provider/hook; no se inventa un patrón de estado nuevo.
- `pnpm-workspace.yaml` ya globa `packages/*` — autoload sin cambios.

## Build outcome (2026-06-16) — PASO B + C complete, all green

Built on worktree branch `chore/i18n-package` with TDD throughout. Final state:

| Package | Tests | Typecheck | Lint |
|---------|-------|-----------|------|
| `@garcia-lorca/i18n` | **23** (parity 11, helper 5, nest 4, catalogs 3) | ✅ | ✅ |
| `@garcia-lorca/web` (next-intl binding) | **20** (5 i18n) | ✅ | ✅ |
| `@garcia-lorca/mobile` (react-i18next + Hermes) | **21** (5 i18n) | ✅ | ✅ |

**Discoveries during build (folded in):**
- **`toNested` transform added** — both next-intl AND react-i18next treat `.` as
  nesting and cannot consume flat dotted keys (confirmed in next-intl docs). The
  package keeps flat catalogs (type-safe + parity) and exposes a framework-agnostic
  flat→nested transform both bindings use.
- **Leaf/branch collision check added to the parity validator** — surfaced by the web
  binding when `label.language` was both a leaf and a prefix of `label.language.es`.
  Now a parity error (it breaks the nested transform), caught at package level via test.
- **Helper decision RESOLVED → kept** (PASO B-0 spike): shares the same pinned
  `intl-messageformat` as both apps, so not a divergent engine; ~0 marginal cost.
- **Hermes Intl: polyfilled** — `@formatjs/intl-pluralrules` (+ getcanonicallocales,
  locale) forced with es/en data, loaded before i18n init; plural-render test green.

**Pre-existing fresh-install fixes this branch surfaces (flagged, not i18n-specific):**
- `apps/mobile`: added `@react-native/jest-preset@0.86.0` (jest-expo 56 peer) — without
  it NO mobile test runs on a fresh strict-pnpm install.
- `apps/mobile`: added `globals@17.6.0` devDep — its `eslint.config.mjs` imports `globals`
  but never declared it; lint failed to load on a fresh install.

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| CEO Review | `/plan-ceo-review` | Scope & strategy | 0 | — | — |
| Codex Review | `/codex review` | Independent 2nd opinion | 0 | — | — |
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 1 | issues_open | 3 arch/quality/test findings + 8 outside-voice; all folded or surfaced |
| Design Review | `/plan-design-review` | UI/UX gaps | 0 | — | — |
| DX Review | `/plan-devex-review` | Developer experience gaps | 0 | — | — |

- **CODEX:** not installed — outside voice ran via Claude subagent (independent context).
- **CROSS-MODEL:** subagent challenged the central premises. Convergent fixes folded
  in (parity = arg-signature+parse+`other`; i18next interpolation contract; dep pinning;
  `exports`/Metro; format-locale `es-MX`). Divergent point (helper keep/drop) surfaced
  to user → user chose to investigate (spike). JSON-vs-literal-types "contradiction"
  resolved: flat JSON + `keyof typeof` gives literal keys with zero codegen.
- **VERDICT:** ENG CLEARED — PASO B + C built and green (23 + 20 + 21 tests, typecheck +
  lint clean across all three packages). Helper spike resolved (kept). No global flag
  flips; this is cross-cutting infra (no module gate).

NO UNRESOLVED DECISIONS
