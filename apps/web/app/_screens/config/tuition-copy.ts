import type { Cycle } from "../../config/api";
import type { TuitionPlan } from "../../finance/api";
import { seedConcepts } from "./tuition-math";

/**
 * El tabulador del año pasado, listo para el que empieza.
 *
 * El caso real del colegio no es capturar catorce campos en blanco: es "los
 * precios del año pasado, con el monto más alto". Ese camino no existía, así que
 * cada agosto la captura empezaba de cero aunque casi nada hubiera cambiado.
 *
 * Los MONTOS se copian tal cual, a propósito: subirlos es la decisión que la
 * directora vino a tomar, y adivinarla sería cobrarle a las familias un número
 * que nadie escribió. Las FECHAS sí se recorren, porque una vigencia del ciclo
 * pasado aplicada al nuevo no es un dato conservador — es un dato equivocado que
 * cobra mal durante meses.
 *
 * No necesita backend nuevo: `listTuitionPlans` ya recibe el ciclo como
 * parámetro, así que el plan anterior se pide igual que el actual.
 */
export function copyPlanToCycle(previous: TuitionPlan, target: Cycle): TuitionPlan {
  return {
    ...previous,
    // Vacío y no el del plan viejo: mandarlo sería pedirle al servidor que
    // sobrescriba el tabulador del año pasado.
    id: "",
    cycleId: target.id,
    enrollmentDueOn: shiftOneYear(previous.enrollmentDueOn),
    enrollmentStartsOn: shiftOneYear(previous.enrollmentStartsOn),
    policy: { ...previous.policy },
    schedule: previous.schedule.map((i) => ({ ...i })),
    fees: previous.fees.map((f) => ({ ...f, months: [...f.months] })),
    /*
     * `seedConcepts` y no los conceptos tal cual.
     *
     * Un plan escrito antes de ADR 0011 lleva UN `enrollmentCents` y ningún
     * concepto. Copiar la lista vacía dejaba la inscripción en cero: la tarjeta
     * prometía "trae los montos tal cual" y el de inscripción desaparecía sin
     * decir nada, hasta que el guardado reclamaba "captura la inscripción" sobre
     * un número que sí existía.
     */
    enrollmentCents: 0,
    enrollmentConcepts: seedConcepts(previous, target).map((c) => ({
      ...c,
      tiers: c.tiers.map((t) => ({
        ...t,
        effectiveFrom: shiftOneYear(t.effectiveFrom),
        effectiveTo: shiftOneYear(t.effectiveTo),
      })),
    })),
  };
}

/**
 * La misma fecha, un año después.
 *
 * Aquí vivía una comprobación: si la fecha corrida caía fuera del ciclo destino,
 * se devolvía vacía. Parecía prudente y estaba mal, y una prueba lo demostró: la
 * FECHA LÍMITE DE INSCRIPCIÓN cae fuera del ciclo POR DISEÑO — la escuela la
 * cobra en junio para un año que abre en agosto. La regla habría borrado ese
 * dato en silencio en cada copia, que es justo el tipo de falla que pretendía
 * evitar.
 *
 * El corrimiento de un año es correcto por construcción y las fechas quedan
 * visibles y editables en la forma, que es donde se revisan. No hace falta que
 * el sistema opine además sobre ellas.
 *
 * Aritmética sobre el texto ISO y no sobre `Date`: `finance` no tiene zona
 * horaria, y construir un `Date` aquí movería el día según dónde esté la
 * computadora.
 */
export function shiftOneYear(iso: string): string {
  if (iso === "") return "";
  const year = Number(iso.slice(0, 4)) + 1;
  const rest = iso.slice(4);
  /*
   * El 29 de febrero se recorre al 28.
   *
   * La aritmética sobre el texto daba "2029-02-29", un día que no existe: un
   * <input type="date"> no puede mostrarlo, así que la vigencia copiada
   * desaparecía de la forma sin aviso — y si llegaba al servidor, tumbaba el
   * guardado entero con un mensaje sobre una fecha que nadie tecleó.
   */
  if (rest === "-02-29" && !isLeapYear(year)) return `${String(year)}-02-28`;
  return `${String(year)}${rest}`;
}

function isLeapYear(year: number): boolean {
  return (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0;
}
