# Parent mobile app — design source & scope

**Source of truth:** Claude Design handoff at
`.claude/docs/design/garcia-lorca-mobile-prototype/` (files in `project/`, intent
in `chats/chat1.md`). Prototype is HTML/CSS/React-web shown in iOS/Android device
frames — recreate in the production stack (`apps/mobile`, **Expo / React Native**)
using **native mobile patterns**, not web CSS or a shrunk desktop layout.

## What it is
The **parent/guardian** app (the surface explicitly removed from the web portal —
see [[web-frontend-design]]). Native iOS + Android. Spanish (es-MX). **Single-child**
layout. Audience: non-technical parents → thumb-friendly, fewest taps.

## Navigation
Bottom tab bar, 5 destinations: **Inicio, Asistencia, Calificaciones, Cuenta/Saldo,
Avisos**. A 6th **"Más"** opens a grid: Documentos, Uniformes, Facturas, Becas
(mirrors the web module grid).

## Screens
- **Acceso (Login)** — consistent with web portal login.
- **Inicio** — child context in header; tappable summary cards (asistencia de hoy,
  avisos no leídos, saldo con semáforo, próximo evento) straight to detail.
- **Asistencia** — month calendar/list, present/absent/excused, one-tap "Justificar".
- **Calificaciones** — by subject/term per level (Kínder cualitativo
  "Logrado/En progreso"; Primaria/Secundaria/Prepa numérico 6–10); promedio visible.
- **Cuenta/Saldo** — large balance, charges/payments list, paid/overdue traffic-light,
  "Pagar ahora" (live updates balance).
- **Avisos** — messages list with read/unread; "Marcar leídos".
- **Más** — module grid.

## Visual language (identical to web portal)
Logo palette (lime #7AC400 primary, forest #0A7A2F, yellow #F2D600, orange #F39200,
red #E00000 alerts-only, purple #8E2E8E). Light + dark themes (deep-forest dark with
luminous accents). Rounded line icons. **Type: Baloo 2 (display) + Nunito (UI)** —
warm/rounded family (note: differs from the web's Poppins+DM Sans, by design — warmer
for families).

## Skin decision (from chat — binding)
Two skins exist: **"Moderno"** (default, flat surfaces, hairline borders, big bold
titles, app-like bottom bar, story-ring avatar — "estilo apps de Meta") and "Clásico"
(shadowed cards). **Build the Moderno skin** as the primary; Clásico is optional/legacy.

## Interactions to preserve
Child context persistent in header; one-tap Justificar on absence; Pagar ahora updates
balance live + toast; read/unread avisos with badges; level toggle flips grades
numeric↔cualitative; friendly empty states; pull-to-refresh / natural gestures.
Bottom sheets for excuse/pay/subject detail.

Every module surface still ships behind its `module_<name>` flag (default OFF).
