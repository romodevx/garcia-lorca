import { type ReactNode } from "react";
import { GatedScreen } from "@/src/components/GatedScreen";
import { AnnouncementsScreen } from "@/src/screens/AnnouncementsScreen";

/** Avisos tab — gated by `module_comms` (UX only). */
export default function AvisosRoute(): ReactNode {
  return (
    <GatedScreen flag="module_comms">
      <AnnouncementsScreen />
    </GatedScreen>
  );
}
