"use client";

import {
  useEffect,
  useRef,
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
  /**
   * Si un clic en el velo y la tecla Esc cierran. `true` por omisión.
   *
   * Se apaga donde hay algo que perder: una forma de cobro a medio capturar no
   * puede irse por un clic fuera de lugar en el mostrador. Es SÓLO para esas
   * dos vías — el botón ✕ y "Cancelar" siguen cerrando siempre, porque un
   * control que no responde es peor que un cierre accidental.
   */
  dismissible?: boolean;
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
  dismissible = true,
  children,
}: DrawerShellProps): ReactElement | null {
  useEffect(() => {
    if (!dismissible) return;
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose, dismissible]);

  /**
   * El contrato de diálogo de WAI-ARIA APG: el foco ENTRA al abrir, se QUEDA
   * dentro mientras está abierto, y VUELVE a donde estaba al cerrar.
   *
   * Las tres fallaban, y las tres fallan en silencio: con el ratón no se nota
   * nada. Quien capturaba con el teclado abría el cajón y seguía tecleando en la
   * página de atrás; al cerrar con Esc el foco caía en <body> y la siguiente
   * tecla no iba a ningún lado.
   *
   * Vive aquí, una sola vez, porque trece pantallas montan esta cáscara y
   * ninguna debería tener que acordarse.
   */
  const panelRef = useRef<HTMLElement | null>(null);

  /*
   * A dónde vuelve el foco al cerrar, capturado EN EL PRIMER RENDER.
   *
   * Se leía dentro del efecto, que corre después del commit en el que React
   * aplica `autoFocus`: en un cajón que enfoca su propio campo al abrir —el de
   * cobro lo hace— se guardaba ese campo, que al cerrar ya no existe, y el foco
   * caía en <body>. Justo la falla que este contrato vino a arreglar.
   */
  const returnToRef = useRef<HTMLElement | null>(null);
  if (returnToRef.current === null && typeof document !== "undefined") {
    returnToRef.current = document.activeElement as HTMLElement | null;
  }

  // El portal necesita un document, que en el render del servidor no existe.
  // Vive aquí arriba porque el contrato de foco depende de él: mientras esto
  // sea false no hay panel al que meter el foco.
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);

  useEffect(() => {
    if (!mounted) return;

    const focusables = (): HTMLElement[] => {
      const root = panelRef.current;
      if (root === null) return [];
      return Array.from(
        root.querySelectorAll<HTMLElement>(
          'input:not([type="hidden"]), select, textarea, button, a[href], [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((el) => !el.hasAttribute("disabled"));
    };

    /*
     * El primer CAMPO, no el primer elemento enfocable.
     *
     * Abrir con el foco en el botón de cerrar le pone a quien captura un control
     * destructivo bajo el pulgar antes de que haya escrito nada. Se usa en los
     * dos lugares que ponen foco —al abrir y al recuperarlo— para que el cajón
     * no tenga dos opiniones sobre dónde empieza.
     */
    const firstField = (): HTMLElement | undefined => {
      const list = focusables();
      return list.find((el) => el.tagName !== "BUTTON") ?? list[0];
    };

    firstField()?.focus();

    const onTab = (e: KeyboardEvent): void => {
      if (e.key !== "Tab") return;
      const current = focusables();
      if (current.length === 0) return;
      const first = current[0];
      const last = current[current.length - 1];
      if (first === undefined || last === undefined) return;

      /*
       * Si el foco YA se fue del panel, se trae de vuelta.
       *
       * El ciclo sólo se cerraba en los bordes de adentro, así que bastaba con
       * que el foco estuviera fuera —un clic en el velo con el cierre por velo
       * apagado lo deja en <body>— para que Tab lo manejara el navegador y se
       * fuera derecho a la página de atrás.
       */
      if (panelRef.current?.contains(document.activeElement) !== true) {
        e.preventDefault();
        (firstField() ?? first).focus();
        return;
      }

      if (document.activeElement === (e.shiftKey ? first : last)) {
        e.preventDefault();
        (e.shiftKey ? last : first).focus();
      }
    };

    /*
     * WCAG 2.2 · 2.4.11 (Focus Not Obscured): el pie es pegajoso y puede quedar
     * encima del campo que acaba de recibir el foco. `nearest` no mueve nada si
     * el campo ya se ve — sólo corrige cuando de verdad quedó tapado.
     */
    const onFocusIn = (e: FocusEvent): void => {
      const target = e.target as HTMLElement | null;
      if (target === null || panelRef.current?.contains(target) !== true) return;
      target.scrollIntoView({ block: "nearest", behavior: "smooth" });
    };

    window.addEventListener("keydown", onTab);
    window.addEventListener("focusin", onFocusIn);
    return () => {
      window.removeEventListener("keydown", onTab);
      window.removeEventListener("focusin", onFocusIn);
      returnToRef.current?.focus();
    };
  }, [mounted]);

  // La página de atrás no se desplaza mientras el cajón está abierto: rodar la
  // lista por debajo hace perder el renglón que se estaba atendiendo.
  useEffect(() => {
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, []);

  if (!mounted) return null;

  const panelStyle: CSSProperties = {
    /*
     * La altura la pone la FORMA, no la pantalla.
     *
     * Con `height: 100%` un alta de tres campos —un feriado, un ciclo, un nivel—
     * quedaba flotando arriba de un metro de blanco, con Guardar clavado hasta
     * abajo: el ojo recorría el monitor entero para nada y la forma parecía
     * incompleta. El techo de 85vh mantiene el desplazamiento dentro del cuerpo,
     * que es lo que pide Carbon: encabezado y pie fijos, y si aun así no cabe,
     * es señal de que eso debía ser una página.
     */
    height: "auto",
    maxHeight: "85vh",
    background: "var(--surface)",
    border: "1px solid var(--border)",
    // Redondeado por los cuatro lados: ya no se apoya en ningún borde.
    borderRadius: "var(--r-xl)",
    boxShadow: "var(--shadow-pop)",
    display: "flex",
    flexDirection: "column",
    /*
     * `pop`, no `slideIn`.
     *
     * `slideIn` NO EXISTE como keyframe en globals.css, y llevaba tiempo así: el
     * panel pedía una animación que el navegador no encontraba y aparecía de
     * golpe. Ahora entra creciendo desde su propio centro, que además es de
     * donde viene — deslizarse desde la derecha ya no querría decir nada.
     */
    animation: "pop .2s var(--ease) both",
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
        className={padded ? "drawer-body" : "drawer-body drawer-body-bare"}
        /*
         * El `display` NO va en línea. Iba, y por eso las dos columnas del cajón
         * grande nunca se activaron: un estilo en línea le gana a la regla de
         * globals.css, y `column-count` ni siquiera aplica a un contenedor flex.
         * Es la misma lección que este trabajo escribió sobre los anchos de
         * página, repetida sin darse cuenta unas horas después.
         */
        style={padded ? { flex: 1, overflowY: "auto", padding: "22px 26px" } : { flex: 1, minHeight: 0 }}
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
      onClick={dismissible ? onClose : undefined}
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 70,
        background: "rgba(6,20,12,.5)",
        backdropFilter: "blur(4px)",
        display: "flex",
        /*
         * Al CENTRO, en los dos ejes.
         *
         * Esto nació anclado a la derecha, como cajón, y mientras midió toda la
         * pantalla se leía bien. Cuando pasó a medir lo que mide su forma, el
         * mismo anclaje dejó un panel chico flotando contra un borde con el
         * resto de la pantalla vacío: ni panel ni diálogo.
         *
         * El relleno impide que toque los bordes en una pantalla angosta, donde
         * `width: min(560px, 100%)` llegaría de lado a lado.
         */
        justifyContent: "center",
        alignItems: "center",
        padding: 24,
      }}
    >
      {onSubmit !== undefined ? (
        <form
          ref={panelRef as React.RefObject<HTMLFormElement>}
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
          ref={panelRef as React.RefObject<HTMLDivElement>}
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
