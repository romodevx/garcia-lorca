"use client";

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactElement,
} from "react";
import { createPortal } from "react-dom";

import { Icon } from "../Icon";

/**
 * El `(?)` que va junto a la etiqueta de un campo.
 *
 * Existe porque el sistema pintaba VALORES DE EJEMPLO dentro de los campos
 * ("ADA", "3200.00", "XXXX000000XXXXXX00"). En gris claro, dentro del recuadro,
 * son indistinguibles de un dato ya capturado: quien captura duda, y en el peor
 * caso guarda sin escribir creyendo que el campo ya traía algo. La explicación
 * no se perdió, se mudó aquí.
 *
 * Lo que un lector de pantalla oye NO depende de ningún gesto: el texto vive
 * siempre en el DOM, en el `<span id={id}>` de abajo, y ese id es el
 * `aria-describedby` DEL CAMPO — no de este botón. Al llegar al campo con Tab la
 * ayuda se lee sola, esté la burbuja abierta o cerrada. La burbuja es sólo para
 * el ojo, y por eso va `aria-hidden`: si no, el texto se leería dos veces.
 *
 * WCAG 2.2 · 1.4.13 pide tres cosas de algo que aparece al señalar, y las tres
 * están aquí: se descarta con Esc, se puede señalar (el cursor pasa del `(?)` a
 * la burbuja gracias al respiro de `GRACIA_MS`) y persiste hasta que uno se va.
 *
 * La burbuja se pinta en el `body` por portal. `position: fixed` NO basta: un
 * ancestro con `transform` —como el cajón que anima su entrada— se vuelve el
 * bloque contenedor, y entonces "fijo respecto a la ventana" pasa a ser "fijo
 * respecto al cajón". Pasó de verdad: la ayuda de CURP salía encima de
 * Apellidos. Por el portal los eventos siguen subiendo por el árbol de React,
 * así que el Esc y el ratón se comportan igual que si fuera hija.
 */
export interface HelpTipProps {
  /** El id que el CAMPO usará en su `aria-describedby`. */
  id: string;
  /** Cómo se llama el campo. Da el nombre accesible: "Ayuda sobre CURP". */
  label: string;
  /** Qué hay que capturar aquí. */
  text: string;
}

/** Fuera de la vista, dentro del árbol de accesibilidad. */
const soloParaLector: CSSProperties = {
  position: "absolute",
  width: 1,
  height: 1,
  padding: 0,
  margin: -1,
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
  whiteSpace: "nowrap",
  border: 0,
};

/**
 * El botón mide 16px pero el área que responde mide 44: el padding transparente
 * cumple el mínimo táctil sin engordar el renglón de la etiqueta, porque el
 * contenedor lo compensa con márgenes negativos.
 */
const ANCHO_BURBUJA = 260;
const SEPARACION = 8;
/** Con cuánto espacio abajo se conforma antes de voltearse hacia arriba. */
const ALTO_ESTIMADO = 120;
/**
 * El respiro entre salir del `(?)` y apagar la burbuja.
 *
 * Sin él, WCAG 2.2 · 1.4.13 "señalable" es inalcanzable: al salir del botón la
 * burbuja se desmonta en el acto, y el `mouseover` que venía en camino aterriza
 * en un nodo ya despegado del DOM. El cursor nunca logra llegar a la burbuja
 * que quería leer.
 */
const GRACIA_MS = 120;

const botonStyle: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  width: 44,
  height: 44,
  margin: -14,
  padding: 0,
  border: "none",
  background: "transparent",
  color: "var(--text-faint)",
  cursor: "help",
  lineHeight: 0,
};

interface Posicion {
  top: number;
  left: number;
  /** Cuando no cabe abajo, la burbuja se ancla por su borde inferior. */
  arriba: boolean;
}

export function HelpTip({ id, label, text }: HelpTipProps): ReactElement {
  // Dos estados y no uno: `fijada` es el clic o el toque (se queda aunque el
  // cursor se vaya, para poder leer con lupa o copiar), `pasajera` es el cursor
  // encima o el foco. Con un solo booleano, el clic del ratón la cerraría en el
  // acto — para cuando llega el `click`, el `mouseenter` ya la abrió.
  const [fijada, setFijada] = useState(false);
  const [pasajera, setPasajera] = useState(false);
  const [pos, setPos] = useState<Posicion | undefined>(undefined);
  const cajaRef = useRef<HTMLSpanElement>(null);
  const botonRef = useRef<HTMLButtonElement>(null);
  const burbujaRef = useRef<HTMLSpanElement>(null);
  const graciaRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const abierta = fijada || pasajera;

  const cancelarGracia = useCallback(() => {
    if (graciaRef.current !== undefined) clearTimeout(graciaRef.current);
    graciaRef.current = undefined;
  }, []);

  /** El cursor entra: al `(?)` o a la burbuja, da igual cuál de los dos. */
  const señalar = useCallback(() => {
    cancelarGracia();
    setPasajera(true);
  }, [cancelarGracia]);

  /** El cursor sale, pero con respiro para que pueda ir hacia la burbuja. */
  const dejarDeSeñalar = useCallback(() => {
    cancelarGracia();
    graciaRef.current = setTimeout(() => setPasajera(false), GRACIA_MS);
  }, [cancelarGracia]);

  /** Esc y el clic de cerrar: sin respiro, es una orden. */
  const cerrar = useCallback(() => {
    cancelarGracia();
    setFijada(false);
    setPasajera(false);
  }, [cancelarGracia]);

  useEffect(() => cancelarGracia, [cancelarGracia]);

  // La burbuja va `fixed` y no `absolute` a propósito: dentro de un cajón con
  // `overflow`, una burbuja absoluta se recorta justo donde empieza a servir.
  useLayoutEffect(() => {
    if (!abierta) return;
    const b = botonRef.current?.getBoundingClientRect();
    if (b === undefined) return;
    const cabeAbajo = b.bottom + SEPARACION + ALTO_ESTIMADO < window.innerHeight;
    setPos({
      arriba: !cabeAbajo,
      top: cabeAbajo ? b.bottom + SEPARACION : b.top - SEPARACION,
      left: Math.max(SEPARACION, Math.min(b.left, window.innerWidth - ANCHO_BURBUJA - SEPARACION)),
    });
  }, [abierta]);

  // Un clic en cualquier otro lado la cierra. Sólo se escucha mientras está
  // fijada: un oyente permanente en todo el documento por cada campo de una
  // forma de veinte sale caro y no sirve de nada.
  useEffect(() => {
    if (!fijada) return;
    const alTocarFuera = (e: MouseEvent): void => {
      const dentro =
        cajaRef.current?.contains(e.target as Node) === true ||
        // La burbuja cuelga del body, así que "dentro" tiene que incluirla: sin
        // esto, seleccionar su texto para copiarlo la apagaría.
        burbujaRef.current?.contains(e.target as Node) === true;
      if (!dentro) cerrar();
    };
    document.addEventListener("mousedown", alTocarFuera);
    return () => document.removeEventListener("mousedown", alTocarFuera);
  }, [fijada, cerrar]);

  const alTeclear = (e: ReactKeyboardEvent): void => {
    if (e.key !== "Escape" || !abierta) return;
    // Se detiene aquí: si siguiera hacia arriba, cerrar la ayuda cerraría el
    // cajón entero y se perdería lo capturado. Primero se cierra lo de encima.
    e.stopPropagation();
    cerrar();
  };

  return (
    <span
      ref={cajaRef}
      style={{ display: "inline-flex", position: "relative", width: 16, height: 16 }}
      onMouseEnter={señalar}
      onMouseLeave={dejarDeSeñalar}
      onKeyDown={alTeclear}
    >
      {/* Lo que oye el lector de pantalla, siempre, abierta o cerrada. */}
      <span id={id} style={soloParaLector}>
        {text}
      </span>
      <button
        ref={botonRef}
        type="button"
        aria-label={`Ayuda sobre ${label}`}
        style={botonStyle}
        onFocus={señalar}
        // El foco que se va apaga SÓLO lo pasajero, nunca lo fijado con clic:
        // si cerrara todo, ir a leer la burbuja —o a seleccionar su texto para
        // copiarlo— la apagaría justo al llegar.
        onBlur={() => {
          cancelarGracia();
          setPasajera(false);
        }}
        onClick={() => {
          cancelarGracia();
          setFijada((antes) => {
            if (antes) setPasajera(false);
            return !antes;
          });
        }}
      >
        <Icon name="help" size={16} stroke={2} />
      </button>
      {abierta && createPortal(
        <span
          ref={burbujaRef}
          role="tooltip"
          aria-hidden="true"
          // La burbuja se defiende sola en vez de confiar en que el contenedor
          // no reciba el `mouseleave` del botón: el orden nativo es salir y
          // luego entrar, así que entrar aquí la vuelve a sostener. Sin esto,
          // ir a leerla con lupa o a copiarla la apaga en el camino.
          onMouseEnter={señalar}
          onMouseLeave={dejarDeSeñalar}
          style={{
            position: "fixed",
            top: pos?.top ?? 0,
            left: pos?.left ?? 0,
            transform: pos?.arriba === true ? "translateY(-100%)" : undefined,
            zIndex: 1000,
            width: ANCHO_BURBUJA,
            padding: "9px 12px",
            borderRadius: "var(--r-md)",
            background: "var(--text)",
            color: "var(--surface)",
            fontSize: 12.5,
            lineHeight: 1.45,
            fontWeight: 500,
            boxShadow: "var(--shadow-tip)",
            pointerEvents: "auto",
          }}
        >
          {text}
        </span>,
        document.body,
      )}
    </span>
  );
}
