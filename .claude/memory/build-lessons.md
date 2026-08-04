# Lecciones de build — gotchas y patrones (no están en CLAUDE.md)

Cosas aprendidas implementando el spine que NO son obvias del repo ni de CLAUDE.md.
Consúltalas antes de re-pisar el mismo rastrillo.

## Secretos / GitGuardian (gatea el merge)
- **GitGuardian bloquea el PR si detecta credenciales hardcodeadas**, incluidas las de tests.
- Patrón: **generar credenciales de test en runtime con `crypto/rand`** (passwords de DB en
  testcontainers, secretos JWT de test con `strings.Repeat("gl-key-", 6)`, placeholders). Nunca
  literales tipo `"postgres"` / `"password"` en el árbol, ni siquiera en `_test.go`.

## Código generado (`gen/`)
- **Nadie edita `gen/`**. Cambias `.proto` → `buf generate` → Go+TS reciben tipos.
- `gen/` está en `.gitignore` **EXCEPTO** `gen/go/go.mod` + `gen/go/go.sum` (sí se trackean, vía
  excepción en `.gitignore`). En CI se regenera todo.
- El módulo `github.com/garcia-lorca/gen/go` se resuelve por **`go.work` (workspace mode)** — los
  servicios NO ponen `require` a gen/go (como iam/gateway). Si un test falla con "client importa
  gen/go vacío", corre `buf generate` en el worktree primero.

## Go
- **Fijar `go 1.24`** en go.mod/go.work. Las toolchains bumpean solo a `go 1.25.0`; revertir siempre.
- **RLS: probar como el rol `app` no-owner NOBYPASSRLS** en testcontainers (no como owner, o las
  policies no se ejercen). RLS **tenant-scoped** vía `app.current_tenant` (SET LOCAL por transacción,
  helper `pkg/authctx.WithTenantContext`). Ver `ADR 0001`.
- JWT: `golang-jwt/jwt/v5`, HS256, `WithValidMethods` (anti alg-confusion), `WithTimeFunc` en tests.

## buf / proto
- **buf lint STANDARD exige respuestas RPC envueltas** (`<Method>Response`), no tipos desnudos.
- La config **`managed` va en `buf.gen.yaml`**, NO en `buf.yaml`.

## Frontend
- **next-intl (web) y react-i18next (mobile) tratan `.` como anidamiento.** Catálogos ICU se
  mantienen **planos**; se aplica un transform **`toNested`** al bindear. El validador de paridad
  detecta **colisión leaf/branch** (una key no puede ser hoja y prefijo a la vez).
- next-intl App Router necesita `app/i18n/request.ts` + `createNextIntlPlugin` en next.config.js.
- Hydration: `<html>` lleva `suppressHydrationWarning` (el script de theme setea `data-theme` pre-hydration).
- **Build de producción sin source maps**; CI falla si aparecen `*.map` / `sourceMappingURL`.
- **`hidden` (módulo desactivado del producto) ≠ flag OFF (`Próximamente`).** Ver [[module-docs-deactivated]].

## pnpm (regla dura)
- **Solo `pnpm`** (`npm`/`yarn` prohibidos; guard `only-allow pnpm`). Única lockfile `pnpm-lock.yaml`.
- Nunca `git add -A` a lo bruto: una vez se coló `.pnpm-store` (21k archivos). Está gitignored + volumen.

## Flujo de PR (probado y funciona)
worktree (`git worktree add -b <rama> ../<dir> master`) → TDD (rojo→verde) → `tsc`+tests+lint verdes
→ push → `gh pr create` → **esperar GitGuardian** → `gh pr merge --squash --delete-branch` →
`git worktree remove` + `prune`. El repo remoto es `romodevx/garcia-lorca`.
