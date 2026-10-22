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
  module_scholarships: false, // T18: becas show on the statement; the "Más" tile stays hidden (mock).
  // The virtual card (task 13): ON since its gates passed (2026-10-21). Cuenta
  // offers "Credencial" and the floating button opens it from every tab.
  module_student_card: true,
  // Payment vouchers (ADR 0040): ON in this build, which carries the native
  // pickers; the gateway's flag still decides (OFF ⇒ 404 ⇒ Cuenta hides it).
  module_payment_vouchers: true,
  // module_billing stays OFF (out of MVP) → "Facturas" tile hidden.
};
