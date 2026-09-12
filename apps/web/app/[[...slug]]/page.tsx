import type { ReactNode } from "react";

import { AppShell } from "../AppShell";

/**
 * Punto de entrada de la web, en CUALQUIER ruta del shell.
 *
 * Es un catch-all opcional a propósito: `/`, `/finanzas`, `/alumnos` y las
 * demás son la misma aplicación, y quien decide qué pintar es <AppShell> al
 * leer la dirección (app/routing/routes.ts). Antes vivía todo en "/" con la
 * pantalla en un useState, y por eso el botón "atrás" del navegador sacaba del
 * sitio y F5 devolvía al tablero.
 *
 * Las rutas más específicas que ya existen — /activar, /healthz, /api/* —
 * ganan sobre este catch-all; así resuelve Next y por eso no hay conflicto.
 *
 * La experiencia del padre/tutor es la app móvil y NO vive aquí.
 */
export default function HomePage(): ReactNode {
  return <AppShell />;
}
