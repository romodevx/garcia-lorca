import type { ReactNode } from "react";
import { AppFlagsProvider } from "./flags-provider";

export const metadata = {
  title: "García Lorca",
  description: "Sistema de Gestión Escolar García Lorca"
};

export default function RootLayout({
  children
}: {
  children: ReactNode;
}): ReactNode {
  return (
    <html lang="es">
      <body>
        <AppFlagsProvider>{children}</AppFlagsProvider>
      </body>
    </html>
  );
}
