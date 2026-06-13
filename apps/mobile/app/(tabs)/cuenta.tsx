import { type ReactNode } from "react";
import { GatedScreen } from "@/src/components/GatedScreen";
import { AccountScreen } from "@/src/screens/AccountScreen";

/** Cuenta / Saldo tab — gated by `module_finance` (UX only). */
export default function CuentaRoute(): ReactNode {
  return (
    <GatedScreen flag="module_finance">
      <AccountScreen />
    </GatedScreen>
  );
}
