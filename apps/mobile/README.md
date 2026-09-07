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

  Identity Platform sign-in (off unless BOTH are set):
  `EXPO_PUBLIC_AUTH_PROVIDER=identity-platform` plus
  `EXPO_PUBLIC_FIREBASE_API_KEY`, `EXPO_PUBLIC_FIREBASE_AUTH_DOMAIN`,
  `EXPO_PUBLIC_FIREBASE_PROJECT_ID`. With the flag off the app signs in through
  `/api/v1/auth/mobile/login` exactly as before. None of these is a secret — a
  Firebase web config is public by design and the gateway is the security
  boundary — but they ARE build-time, so turning the provider on or off is a
  rebuild, not a setting.

## Feature flags

Same UX-only gating as web, via `@garcia-lorca/ui/flags` (`FlagsProvider` at the
root — see `app/_layout.tsx` — plus `useFlag` / `FeatureGate`). Backend is the
security boundary. Tabs: `asistencia` (module_attendance), `calificaciones`
(module_grades), `estado-cuenta` (module_finance).
