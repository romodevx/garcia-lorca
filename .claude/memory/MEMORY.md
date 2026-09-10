# Project memory index

One line per memory file. Load this first; open the file when relevant.

- [project-status](project-status.md) — **EMPIEZA AQUÍ**: progreso real (M01✅ M02✅ M03🚧), qué falta y el próximo paso. Snapshot 2026-07-02.
- [build-lessons](build-lessons.md) — gotchas no obvios (GitGuardian runtime-creds, gen/go tracking, go 1.24 pin, RLS app-role, buf/i18n quirks, flujo de PR).
- [web-frontend-design](web-frontend-design.md) — design source, palette/type/themes, and the web=admin+teacher-only scope decision.
- [mobile-app-design](mobile-app-design.md) — parent (padres) Expo/RN app design: nav, screens, Baloo2+Nunito, "Moderno" skin.
- [package-manager-pnpm](package-manager-pnpm.md) — pnpm is the ONLY package manager; npm/yarn forbidden.
- [i18n-bilingual](i18n-bilingual.md) — bilingual es-MX (default) + en; UI translated, official docs (boletas/constancias) Spanish-only, backend returns codes not text.
- [command-palette](command-palette.md) — Cmd+K omnidirectional search (navigate + execute, min clicks); the per-module registration CONTRACT every module must feed.
- [ux-minimum-clicks](ux-minimum-clicks.md) — UX north star: everything easy, fewest clicks, command-palette-first; an acceptance criterion, not decoration.
- [module-docs-deactivated](module-docs-deactivated.md) — Documentos (M09/`module_docs`) desactivado del panel (web+mobile) por decisión del colegio; NO borrado, reactivable quitando `hidden`.
- [Corte a Identity Platform (2026-09)](identity-platform-cutover-2026-09.md) — dev entra por el proveedor; los 3 interruptores, y el rol a medida que `-target=module.iam` no crea (401 mudo).
- [Migraciones de dev y fallos silenciosos](dev-migrations-and-silent-failures.md) — el deploy ya migra; cómo diagnosticar deriva de esquema; y por qué un fallo nunca se cuenta como error del usuario.
