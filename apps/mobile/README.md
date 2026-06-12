# @garcia-lorca/mobile

Expo (Expo Router) mobile surface for García Lorca (parent/student facing).

## Scripts

- `start` / `ios` / `android` — Expo dev.
- `build` — `expo export --platform all` (NOTE: never add `--source-maps`).
- `test` — Jest + React Native Testing Library.
- `typecheck` — `tsc --noEmit`.

## Build hardening (non-negotiable)

- **Hermes** JS engine on both platforms (`app.json` + `app.config.ts`) —
  production JS ships as bytecode, not readable source.
- **Source maps are never shipped.** Expo only emits maps when
  `expo export --source-maps` is passed; the `build` script does NOT pass it,
  and `app.config.ts` registers no sourcemap-upload hook, so maps never leave
  the build machine.
- Only `EXPO_PUBLIC_*` env vars reach the bundle. No secrets/internal URLs.
  `EXPO_PUBLIC_API_URL` → gateway base URL.

## Feature flags

Same UX-only gating as web, via `@garcia-lorca/ui/flags` (`FlagsProvider` at the
root — see `app/_layout.tsx` — plus `useFlag` / `FeatureGate`). Backend is the
security boundary. Tabs: `asistencia` (module_attendance), `calificaciones`
(module_grades), `estado-cuenta` (module_finance).
