import type { FlagSet } from "@garcia-lorca/ui/flags";

/**
 * Demo flag seed for the parent app.
 *
 * Flags default OFF. On boot the real app will fetch the resolved map from the
 * gateway (`GET <EXPO_PUBLIC_API_URL>/features`) — the BACKEND gateway is the
 * security boundary (a disabled module's routes return 404). These frontend
 * checks are UX-only: they decide which tabs/modules render and which chunks
 * are reachable.
 *
 * This seed turns ON only the module surfaces backed by a real service in this
 * build (attendance, grades, finance, comms) so the demo shows the parent
 * experience end to end. Modules with NO backend stay OFF — their "Más" tiles
 * are not rendered at all (mock content kept future-ready in `mock.ts`, just
 * unreachable) until the backend lands.
 */
export const demoFlags: FlagSet = {
  module_attendance: true,
  module_grades: true,
  module_finance: true,
  module_comms: true, // avisos / announcements (notifications are real — keep ON)
  // No-backend modules: OFF so their "Más" tiles are hidden (not shown as mock).
  module_docs: false, // school doesn't offer Documents; kept future-ready.
  module_uniforms: false, // uniforms service is a shell — hide until backend lands.
  module_scholarships: false, // scholarships service pending — hide until backend lands.
  // module_billing stays OFF (out of MVP) → "Facturas" tile hidden.
};
