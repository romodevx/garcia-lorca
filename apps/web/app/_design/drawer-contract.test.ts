import { globSync, readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

/**
 * Un solo cajón en todo el sistema.
 *
 * Llegó a haber NUEVE, ocho de ellos copiados a mano de <SlideOver>, con cinco
 * anchos distintos: 520, 520, 520, 460, 520, 480, 520, 460 y 560. Por eso el
 * ancho del cajón no se podía arreglar en un lugar — no había un lugar. Y por
 * eso el defecto de Configuración (el velo que no cubría la barra superior)
 * pudo existir en una copia mientras las demás se salvaban por casualidad.
 *
 * Esta prueba mira el código fuente porque lo que defiende no es una conducta
 * de una pantalla, sino que NADIE vuelva a escribir la novena copia.
 */
const SCREENS = globSync("app/_screens/**/*.tsx", { cwd: process.cwd() })
  .filter((f) => !f.includes(".test."))
  .map((f) => [f, readFileSync(f, "utf8")] as const);

/**
 * Los diálogos dibujados a mano que ya existían, con nombre y razón.
 *
 * La lista está VACÍA, y eso es el punto: existe para que una excepción tenga
 * que escribirse y explicarse, no para que se acumulen.
 *
 * Tuvo un renglón exactamente una vez. `StudentPicker` apareció cuando la
 * cáscara se centró —hasta entonces la firma que se buscaba era el anclaje a la
 * derecha, y un buscador centrado se colaba— y se convirtió en cuanto el dueño
 * lo autorizó: su diálogo propio no tenía trampa de foco ni devolvía el foco al
 * cerrar, y eso no se arregla catorce veces, se arregla en la cáscara.
 */
const KNOWN_HANDROLLED: readonly string[] = [];

describe("el cajón", () => {
  it("nadie dibuja el suyo", () => {
    const offenders = SCREENS
      /*
       * La firma del velo: fijo, pegado a los cuatro lados, y un `zIndex` alto
       * acomodando su contenido. Antes bastaba con buscar `flex-end`, porque el
       * panel iba anclado a la derecha; al centrarse, esa firma dejó de existir
       * y la prueba habría dejado pasar la copia número diez sin decir nada.
       *
       * Un toast fijo abajo al centro no es un cajón: por eso se exige también
       * el velo oscuro, que es lo que de verdad distingue a un modal.
       */
      .filter(([, src]) =>
        /position:\s*"fixed",\s*inset:\s*0[\s\S]{0,320}background:\s*"rgba\(/.test(src),
      )
      .map(([f]) => f)
      .filter((f) => !KNOWN_HANDROLLED.includes(f));
    expect(offenders).toEqual([]);
  });

  it("nadie pega el ancho de un cajón en el JSX", () => {
    // min(520px, 100%) y sus cuatro primos. El ancho vive en globals.css, donde
    // una @media sí puede alcanzarlo.
    const offenders = SCREENS
      .filter(([, src]) => /width:\s*"min\((?:4[0-9]{2}|5[0-9]{2})px/.test(src))
      .map(([f]) => f);
    expect(offenders).toEqual([]);
  });
});

/**
 * El flujo tipo periódico dentro de una forma.
 *
 * Baymard lo midió: el layout multicolumna hace que se omitan campos
 * obligatorios y se llenen campos que no venían al caso, porque las columnas
 * "jalan la atención en múltiples direcciones". En el cajón grande además había
 * un agravante propio: el corte caía donde alcanzara la altura, así que agregar
 * un concepto de inscripción hacía brincar de columna a "Cobranza" entera —
 * la forma se reacomodaba bajo las manos de quien capturaba.
 *
 * Esta prueba mira el CSS porque lo que defiende no es la conducta de una
 * pantalla: es que nadie lo vuelva a escribir.
 */
describe("las formas", () => {
  it("no fluyen en columnas", () => {
    expect(readFileSync("app/globals.css", "utf8")).not.toMatch(/column-count/);
  });
});
