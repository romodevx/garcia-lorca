"use client";

import type { ReactElement, ReactNode } from "react";

/**
 * El contenedor de toda pantalla: un ancho, un margen y un relleno.
 *
 * Existe porque había CINCO anchos de página distintos y ninguna regla que los
 * explicara — 820 en Avisos, 900 en Ventanilla, 1160 en Asistencia y
 * Calificaciones, 1180 en Uniformes, 1320 en el resto — copiados a mano en doce
 * pantallas. En un monitor de 2560px, Avisos usaba el 32% del ancho.
 *
 * Y, más importante: esos anchos estaban en ESTILOS EN LÍNEA, que le ganan a
 * cualquier @media. Mientras vivieran en el JSX no había responsive posible,
 * sólo media queries que no se aplicaban. Los números ahora viven en
 * globals.css (--page-max), donde el monitor puede opinar.
 */
export interface PageProps {
  /**
   * `wide` (por omisión) para tableros, tablas y catálogos: usa todo el ancho
   * que el monitor permita.
   *
   * `reading` para el cuerpo de un aviso y la ventanilla de cobro: la PÁGINA
   * sigue usando todo el monitor, pero la columna que se lee se acota. Un
   * párrafo de 1800px hace que el ojo pierda el renglón al regresar.
   */
  variant?: "wide" | "reading";
  children: ReactNode;
}

export function Page({ variant = "wide", children }: PageProps): ReactElement {
  if (variant === "reading") {
    return (
      <div className="page page-reading">
        <div className="page-column">{children}</div>
      </div>
    );
  }
  return <div className="page">{children}</div>;
}
