import type { ReactNode } from "react";
import { AppShell } from "./AppShell";

/**
 * Web entry point. The full administración + maestros experience (login →
 * dashboard → finanzas/módulos) runs inside the client-side <AppShell>, matching
 * the design handoff's SPA flow. The parent/tutor experience is a separate
 * mobile app and is intentionally NOT part of this web build.
 */
export default function HomePage(): ReactNode {
  return <AppShell />;
}
