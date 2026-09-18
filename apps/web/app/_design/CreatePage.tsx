"use client";

import type { ReactElement, ReactNode } from "react";

/**
 * La página de un alta que ya no cabe en un cajón.
 *
 * Carbon lo dice sin rodeos: si la altura del modal grande no alcanza,
 * probablemente se necesita una página completa. El tabulador de colegiaturas
 * son catorce campos en cinco secciones, y durante meses se capturó por un cajón
 * que —para que cupieran— fluía en dos columnas tipo periódico. El corte caía
 * donde alcanzara la altura, así que la forma se reacomodaba mientras alguien
 * escribía. Eso no se arregla con un cajón más ancho.
 *
 * Una columna, el cuerpo es lo único que se desplaza, y dos cosas que NUNCA se
 * van de la vista: el resumen de lo que se está armando y el botón de guardar.
 */
export interface CreatePageProps {
  breadcrumb: readonly string[];
  title: string;
  subtitle?: string;
  /**
   * El resumen en vivo, pegado a un lado. Es lo que se revisa antes de guardar,
   * y separarlo de los números que lo producen obliga a ir y venir para entender
   * de dónde salió — que es por lo que esta página no tiene pasos.
   */
  aside?: ReactNode;
  footer: ReactNode;
  children: ReactNode;
}

export function CreatePage({
  breadcrumb,
  title,
  subtitle,
  aside,
  footer,
  children,
}: CreatePageProps): ReactElement {
  return (
    <div className="create-page">
      <header style={{ padding: "22px 26px 18px" }}>
        <p style={{ fontSize: 13, color: "var(--text-muted)", marginBottom: 4 }}>
          {`${breadcrumb.join(" › ")} ›`}
        </p>
        <h1 style={{ fontSize: 26, fontWeight: 800, color: "var(--text)", lineHeight: 1.15 }}>
          {title}
        </h1>
        {subtitle !== undefined && (
          <p style={{ fontSize: 14.5, color: "var(--text-muted)", marginTop: 4 }}>{subtitle}</p>
        )}
      </header>

      {/* El ancho y las columnas viven en globals.css, donde una @media sí puede
          alcanzarlos. Este repo ya pagó dos veces la lección de ponerlos en
          línea: un estilo en línea le gana a cualquier media query. */}
      <div className="create-page-grid">
        <div className="create-page-body">{children}</div>
        {aside !== undefined && (
          <aside data-testid="create-page-aside" className="create-page-aside">
            {aside}
          </aside>
        )}
      </div>

      <div data-testid="create-page-footer" className="create-page-footer">
        {footer}
      </div>
    </div>
  );
}
