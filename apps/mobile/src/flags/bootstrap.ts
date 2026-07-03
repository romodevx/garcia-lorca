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
 * This seed turns ON exactly the module surfaces implemented in this build so
 * the demo shows the parent experience end to end. It mirrors the
 * `preset_family_app` intent plus the "Más" module tiles.
 */
export const demoFlags: FlagSet = {
  module_attendance: true,
  module_grades: true,
  module_finance: true,
  module_comms: true, // avisos / announcements
  module_docs: false, // Deactivated: school doesn't offer Documents; tile hidden from "Más" (kept future-ready).
  module_uniforms: true, // Más → Uniformes
  module_scholarships: true, // Más → Becas
  // module_billing stays OFF (out of MVP) → "Facturas" tile renders disabled.
};
