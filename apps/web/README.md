# @garcia-lorca/web

Next.js (App Router) web surface for García Lorca.

## Scripts

- `dev` / `build` / `start` — Next.js.
- `test` — Vitest + React Testing Library.
- `typecheck` — `tsc --noEmit`.
- `check:no-sourcemaps` — **CI hardening gate**, run after `next build`.

## Build hardening (non-negotiable)

- `next.config.js` sets `productionBrowserSourceMaps: false` explicitly.
- `console.*` (except error/warn) stripped in production via the compiler.
- Minify + tree-shake are on by default in `next build`.
- `scripts/check-no-sourcemaps.mjs` fails CI if any `*.map` file or
  `sourceMappingURL` comment exists in the **browser-shipped** output
  (`.next/static`). Server/SSR maps under `.next/server` are never sent to a
  client and are intentionally out of scope. Wire it into CI:

  ```sh
  npm run build && npm run check:no-sourcemaps
  ```

## Feature flags

UX-only gating via `@garcia-lorca/ui/flags` (`FlagsProvider` at the root —
see `app/flags-provider.tsx` — plus `useFlag` / `FeatureGate`). Flags are
fetched once on boot from the gateway `/features` endpoint. The backend is the
security boundary. Route groups: `(auth)`, `(admin)`, `(maestro)`.

## Env vars

Only `NEXT_PUBLIC_*` vars reach the client bundle. No secrets/internal URLs.
`NEXT_PUBLIC_API_URL` → gateway base URL.
