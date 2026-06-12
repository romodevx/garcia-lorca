import type { ReactNode } from "react";

/**
 * Login page stub. Real auth (IAM module) lands behind `module_iam`. This stub
 * exists so the `(auth)` route group and `/login` route are wired in FASE 0.
 */
export default function LoginPage(): ReactNode {
  return (
    <main>
      <h1>Iniciar sesión</h1>
      <p>Stub — la autenticación llega con el módulo IAM.</p>
    </main>
  );
}
