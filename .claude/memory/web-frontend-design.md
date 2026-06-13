# Web frontend design — source & scope

**Source of truth:** Claude Design handoff bundle at
`.claude/docs/design/garcia-lorca-prototype/` (project files under `project/`,
intent in `chats/chat1.md`). The React+Babel files there are **prototypes** —
recreate them in the production stack (`apps/web`, Next.js App Router + TS),
matching visual output, not copying prototype structure.

## Scope decision (from the design chat — binding)

This **web** system is for **administración + maestros only**. The parent/tutor
experience is a **separate mobile app** (`apps/mobile`), NOT part of the web.
The prototype's parent screens (`screen_parent.jsx`, `lib/ios-frame.jsx`) and the
"Padre" login role were removed and are out of scope for the web build.

## Design system essentials

- **Palette (from school logo):** lime `#7AC400` (primary action), forest `#0A7A2F`,
  yellow `#F2D600`, orange `#F39200`, red `#E00000` (alerts/destructive ONLY),
  purple `#8E2E8E`. Each module is keyed to a logo color for fast recognition.
- **Type:** Poppins (display/headings) + DM Sans (UI). **Icons:** rounded line icons
  (24px grid, `currentColor` stroke).
- **Themes:** live light/dark toggle (persisted to `localStorage` as `gl-theme`),
  driven by `data-theme` on `<html>`. Dark = deep forest/near-black with luminous accents.
- **Tokens** live in CSS custom properties (see prototype `styles.css`).

## Screens implemented for the web

1. **Login** — split layout, forest brand panel + decorative rosette, role segmented
   control (Administración / Maestro), one-click entry.
2. **Dashboard** — greeting + 3 KPI cards + app-launcher grid of all 11 modules.
3. **Finanzas** — student list with paid/por-vencer/vencido traffic-light, filter tabs
   with counts, level pills, search, one-click "Registrar pago" + confirmation toast.
4. **Module placeholder** — friendly stub so the other modules never dead-end.

## UX principles to preserve

Fewest clicks; one-click primary actions (large lime buttons); strong hierarchy
(most-used bigger/higher); friendly empty states; generous legible type for
non-technical users; consistent rounded icons.

See [[feature-flags]] context: every module screen still ships behind its
`module_<name>` flag (default OFF) per project rules.
