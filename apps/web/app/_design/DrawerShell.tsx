"use client";

import {
  useEffect,
  useState,
  type CSSProperties,
  type FormEvent,
  type ReactElement,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";

import { Icon, type IconName } from "./Icon";

/**
 * La cáscara de TODO cajón del sistema: el velo, el panel, el encabezado, Esc,
 * el bloqueo de desplazamiento y el portal.
 *
 * Existe porque llegó a haber NUEVE cajones — ocho copiados a mano del primero —
 * con cinco anchos distintos (520, 520, 520, 460, 520, 480, 520, 460 y 560). No
 * era sólo repetición: el defecto que el colegio vio en Configuración el
 * 11-sep-2026 vivía en una copia mientras las demás se salvaban por casualidad.
 *
 * SE MONTA EN document.body, POR PORTAL, y eso es load-bearing. Con
 * `position: fixed; inset: 0` esto debería anclarse al viewport; en
 * Configuración no lo hacía, porque sus paneles nacen dentro de un
 * `<div className="fade-up">` y una animación de `transform` CREA UN BLOQUE
 * CONTENEDOR para los descendientes fijos. `inset: 0` pasaba a significar "esa
 * columna de 1320px": el velo no cubría la barra superior —se podía cerrar
 * sesión con el cajón "modal" abierto— y el cajón no llegaba al borde derecho
 * del monitor. Depender de dónde alguien puso un `<div>` no es una garantía;
 * desde `document.body` ningún ancestro puede volver a capturarlo.
 *
 * El ancho vive en globals.css (.drawer-sm/md/lg), no aquí: un estilo en línea
 * le gana a cualquier @media, y este repo ya lo aprendió con .topbar-search.
 *
 * Dos envoltorios la usan: `SlideOver`, que arma el pie estándar de
 * Cancelar/Guardar, y los cajones con pie propio, que pasan `footer`.
 */

/**
 * Cuánto espacio pide la forma que va dentro.
 *
 * `sm` fichas y lectura · `md` altas de un puñado de campos · `lg` formularios
 * grandes, que además se acomodan en dos columnas en un monitor ancho.
 */
export type DrawerSize = "sm" | "md" | "lg";

const closeButton: CSSProperties = {
  width: 40,
  height: 40,
  borderRadius: "50%",
  display: "grid",
  placeItems: "center",
  background: "var(--surface-3)",
  border: "1px solid var(--border)",
  color: "var(--text-muted)",
  cursor: "pointer",
};

export interface DrawerShellProps {
  title: string;
  subtitle?: string;
  icon: IconName;
  size?: DrawerSize;
  onClose: () => void;
  /**
   * Cuando se da, el panel ES un <form>: Enter guarda y el navegador hace el
   * trabajo de foco. Sin él es un <div>, para los cajones que sólo muestran.
   */
  onSubmit?: () => void;
  footer?: ReactNode;
  /**
   * Encabezado propio, para el cajón que no se presenta con ícono y título —
   * la ficha de un alumno abre con su avatar y su estado de admisión. Sustituye
   * al encabezado estándar; el botón de cerrar corre por cuenta de quien lo da.
   */
  header?: ReactNode;
  /**
   * Cuando es false, el cuerpo no lleva relleno ni desplazamiento propio: el
   * hijo manda sobre esa caja. Lo necesita el cobro, que alterna entre una
   * forma y un recibo y cada uno trae su propio pie — anidar dos zonas de
   * desplazamiento deja la de afuera rodando sobre la de adentro.
   */
  padded?: boolean;
  children: ReactNode;
}

export function DrawerShell({
  title,
  subtitle,
  icon,
  size = "md",
  onClose,
  onSubmit,
  footer,
  header,
  padded = true,
  children,
}: DrawerShellProps): ReactElement | null {
  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  // La página de atrás no se desplaza mientras el cajón está abierto: rodar la
  // lista por debajo hace perder el renglón que se estaba atendiendo.
  useEffect(() => {
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, []);

  // El portal necesita un document, que en el render del servidor no existe.
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);
  if (!mounted) return null;

  const panelStyle: CSSProperties = {
    height: "100%",
    background: "var(--surface)",
    borderLeft: "1px solid var(--border)",
    boxShadow: "var(--shadow-pop)",
    display: "flex",
    flexDirection: "column",
    animation: "slideIn .28s var(--ease) both",
  };

  const chrome = (
    <>
      {header ?? (
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 14,
          padding: "22px 26px",
          borderBottom: "1px solid var(--border)",
        }}
      >
        <div
          style={{
            width: 48,
            height: 48,
            borderRadius: 14,
            display: "grid",
            placeItems: "center",
            background: "var(--forest)",
            color: "#fff",
          }}
        >
          <Icon name={icon} size={26} />
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <h2 style={{ fontSize: 21, fontWeight: 700, color: "var(--text)" }}>{title}</h2>
          {subtitle !== undefined && (
            <p style={{ fontSize: 13.5, color: "var(--text-muted)", marginTop: 2 }}>{subtitle}</p>
          )}
        </div>
        <button type="button" onClick={onClose} aria-label="Cerrar" style={closeButton}>
          <Icon name="close" size={19} />
        </button>
      </div>
      )}

      <div
        className="drawer-body"
        style={
          padded
            ? { flex: 1, overflowY: "auto", padding: "22px 26px", display: "flex", flexDirection: "column", gap: 18 }
            : { flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }
        }
      >
        {children}
      </div>

      {footer !== undefined && (
        <div
          style={{
            display: "flex",
            gap: 12,
            alignItems: "center",
            padding: "18px 26px",
            borderTop: "1px solid var(--border)",
            background: "var(--surface-2)",
          }}
        >
          {footer}
        </div>
      )}
    </>
  );

  const stop = (e: { stopPropagation: () => void }): void => e.stopPropagation();
  const className = `drawer drawer-${size}`;

  return createPortal(
    <div
      data-testid="drawer-backdrop"
      onClick={onClose}
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 70,
        background: "rgba(6,20,12,.5)",
        backdropFilter: "blur(4px)",
        display: "flex",
        justifyContent: "flex-end",
      }}
    >
      {onSubmit !== undefined ? (
        <form
          onClick={stop}
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            onSubmit();
          }}
          role="dialog"
          aria-modal="true"
          aria-label={title}
          className={className}
          style={panelStyle}
        >
          {chrome}
        </form>
      ) : (
        <div
          onClick={stop}
          role="dialog"
          aria-modal="true"
          aria-label={title}
          className={className}
          style={panelStyle}
        >
          {chrome}
        </div>
      )}
    </div>,
    document.body,
  );
}
