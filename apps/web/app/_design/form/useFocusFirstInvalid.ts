"use client";

import { useCallback, useEffect, useState } from "react";

/**
 * Llevar la vista al primer campo que falla, al guardar.
 *
 * El defecto que esto cierra, medido en el alta de alumno: al guardar vacío
 * salían CINCO errores y sólo se veían CUATRO. El de "Tutor" quedaba 53 px por
 * debajo del pie pegajoso del cajón, la forma no se desplazaba (`scrollTop`
 * seguía en 0) y el foco se quedaba en el botón — así que el teclado tampoco
 * llevaba allí. El director corregía los cuatro que veía, volvía a guardar, y
 * seguía fallando por una razón que no aparecía en ningún lado.
 *
 * Busca en el DOM y no en el estado, a propósito: en este sistema conviven dos
 * validadores (`useRequired` y los `missingFields` propios de cada pantalla), y
 * los dos acaban marcando `aria-invalid` a través de la capa de captura. El DOM
 * es donde se encuentran, y su orden es justamente el que el ojo sigue bajando
 * por la forma — que no tiene por qué ser el orden en que se declararon.
 *
 * Se llama DESPUÉS de marcar los errores, en el mismo manejador. React agrupa
 * las dos actualizaciones en un render, y el efecto corre cuando el
 * `aria-invalid` ya está puesto.
 */
export function useFocusFirstInvalid(): () => void {
  // Un contador y no un booleano: dos intentos seguidos con los mismos errores
  // tienen que volver a saltar. Con un booleano, el segundo no cambiaría nada.
  const [intento, setIntento] = useState(0);

  useEffect(() => {
    if (intento === 0) return;

    // Detrás del cajón puede haber otra forma con lo suyo en rojo; saltar allá
    // sacaría al capturista del cajón que tiene abierto. Sólo hay un diálogo a
    // la vez, que es la regla de formas del proyecto.
    const raiz: ParentNode = document.querySelector('[role="dialog"]') ?? document;
    const primero = raiz.querySelector<HTMLElement>('[aria-invalid="true"]');
    if (primero === null) return;

    // `preventScroll` y luego el desplazamiento a mano: el que hace `focus` por
    // su cuenta es el mínimo, y el mínimo deja el campo justo pegado al borde
    // — debajo del pie. `center` lo despeja (WCAG 2.2 · 2.4.11).
    primero.focus({ preventScroll: true });
    primero.scrollIntoView?.({ block: "center", inline: "nearest" });
  }, [intento]);

  return useCallback(() => setIntento((n) => n + 1), []);
}
