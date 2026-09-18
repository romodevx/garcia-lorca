import { expect, test } from "@playwright/test";

import { API_URL, apiAsOwner, signIn, uniqueSuffix } from "./helpers";

/**
 * El tabulador, de punta a punta: capturarlo en la página y que el precio quede.
 *
 * Existe porque el recorrido desde cero (`setup-from-zero.spec.ts`) sólo llega
 * aquí después de diez pasos encadenados, y además exige base vacía — su nivel
 * se llama "Primaria", sin marca de corrida. Verificar el tabulador no puede
 * costar borrar el colegio sembrado que el colegio usa para demostrar.
 *
 * Por eso las precondiciones van por API y las aserciones por la interfaz, que
 * es la regla que `helpers.ts` ya declara. Lo que se prueba aquí es lo que el
 * rediseño cambió: que la forma sea una PÁGINA, que guarde, y que el atajo de
 * copiar del ciclo anterior exista de verdad — no sólo en el componente.
 *
 * CADA CORRIDA USA SUS PROPIOS AÑOS. El "ciclo anterior" se elige por fecha de
 * inicio, así que con fechas fijas el ANTERIOR de la corrida pasada empata con
 * el de ésta y a veces gana el que no tiene tabulador: la prueba fallaba una de
 * cada dos veces por sus propios datos. Los años se separan de tres en tres para
 * que ningún ciclo de otra corrida pueda colarse entre los dos de ésta.
 */
function añosDeLaCorrida(): { anterior: number; nuevo: number } {
  const base = 2100 + (Date.now() % 300) * 3;
  return { anterior: base, nuevo: base + 1 };
}
test.describe("el tabulador de un nivel", () => {
  test("se captura en la página y el precio queda en la tabla", async ({ page, request }) => {
    const sello = uniqueSuffix();
    const api = await apiAsOwner(request);
    const nivel = `KINDER ${sello}`;

    const { anterior: año } = añosDeLaCorrida();
    await api.post(`${API_URL}/api/v1/config/cycles`, {
      data: { name: `CICLO ${sello}`, startsOn: `${año}-08-01`, endsOn: `${año + 1}-06-30` },
    });
    await api.post(`${API_URL}/api/v1/config/levels`, {
      data: { name: nivel, sepCode: "KIN", enrollmentGranularity: "cycle" },
    });

    await signIn(page);
    await page.goto("/configuracion");
    await page.getByRole("button", { name: new RegExp(`CICLO ${sello}`, "i") }).first().click();

    await page.getByRole("row", { name: new RegExp(nivel) })
      .getByRole("button", { name: /Definir|Editar/ }).click();

    // Es una página, no un cajón. Ese es el arreglo, no un detalle de estilo.
    await expect(page.getByRole("heading", { name: new RegExp(`^Precios de ${nivel}`) })).toBeVisible();
    await expect(page.getByRole("dialog")).toHaveCount(0);

    // La inscripción se captura por conceptos con su vigencia (ADR 0011).
    await page.getByLabel("Nuevo concepto").fill("INSCRIPCIÓN");
    await page.getByRole("button", { name: /Agregar concepto/ }).click();
    await page.getByLabel("Monto").fill("4500");
    await page.getByLabel("Vigente desde").fill(`${año}-08-01`);
    await page.getByRole("button", { name: /Agregar monto/ }).click();

    await page.getByLabel("Colegiatura mensual").fill("2800");

    // El año completo se calcula mientras se captura, que es la razón de que
    // esta página no tenga pasos: separar el total de sus insumos obliga a ir y
    // venir para entender de dónde salió el precio que se va a cobrar.
    await expect(page.getByText("$32,500.00")).toBeVisible();

    await page.getByRole("button", { name: "Guardar tabulador" }).click();

    // De vuelta en la tabla, con los dos precios puestos. La inscripción se
    // SUMA de los conceptos: `enrollmentCents` viaja en cero desde ADR 0011, y
    // leerlo anunciaba como gratis una inscripción de miles de pesos.
    const fila = page.getByRole("row", { name: new RegExp(nivel) });
    await expect(fila).toContainText("2800.00");
    await expect(fila, "la inscripción se volvió a anunciar como gratis").toContainText("4500.00");
  });

  test("ofrece copiar del ciclo anterior, y trae los montos", async ({ page, request }) => {
    const sello = uniqueSuffix();
    const api = await apiAsOwner(request);
    const nivel = `PRIMARIA ${sello}`;

    const { anterior: añoPrevio, nuevo: añoNuevo } = añosDeLaCorrida();
    const anterior = await api.post(`${API_URL}/api/v1/config/cycles`, {
      data: { name: `ANTERIOR ${sello}`, startsOn: `${añoPrevio}-08-01`, endsOn: `${añoPrevio + 1}-06-30` },
    });
    const nuevo = await api.post(`${API_URL}/api/v1/config/cycles`, {
      data: { name: `NUEVO ${sello}`, startsOn: `${añoNuevo}-08-01`, endsOn: `${añoNuevo + 1}-06-30` },
    });
    const level = await api.post(`${API_URL}/api/v1/config/levels`, {
      data: { name: nivel, sepCode: "PRI", enrollmentGranularity: "cycle" },
    });

    const cicloAnteriorId = (await anterior.json()).id as string;
    const cicloNuevoId = (await nuevo.json()).id as string;
    const levelId = (await level.json()).id as string;
    expect(cicloAnteriorId, "el ciclo anterior no se creó").toBeTruthy();
    expect(cicloNuevoId, "el ciclo nuevo no se creó").toBeTruthy();

    // El tabulador del año pasado: lo que se va a copiar.
    await api.put(`${API_URL}/api/v1/finance/tuition-plans`, {
      data: {
        cycleId: cicloAnteriorId,
        levelId,
        enrollmentCents: 0,
        monthlyCents: 320000,
        months: 10,
        firstBilledMonth: 9,
        dueDay: 10,
        lateFeeCents: 0,
        monthlyDiscountCents: 0,
        schedule: [],
        active: true,
        fees: [],
        enrollmentDueOn: "",
        enrollmentConcepts: [
          { name: "INSCRIPCIÓN", tiers: [{ effectiveFrom: `${añoPrevio}-08-31`, effectiveTo: "", amountCents: 500000 }] },
        ],
        policy: {
          dailyMode: false,
          lateFeeDailyCents: 0,
          surchargeCountsAllDays: true,
          earlyPaymentCents: 0,
          earlyPaymentDays: 0,
        },
      },
    });

    await signIn(page);
    await page.goto("/configuracion");
    await page.getByRole("button", { name: new RegExp(`NUEVO ${sello}`, "i") }).first().click();

    await page.getByRole("row", { name: new RegExp(nivel) })
      .getByRole("button", { name: /Definir|Editar/ }).click();
    await expect(page.getByRole("heading", { name: new RegExp(`^Precios de ${nivel}`) })).toBeVisible();

    /*
     * El atajo tiene que EXISTIR en la app, no sólo en el componente.
     *
     * Durante un rato no existió: `TuitionPanel` sabía ofrecerlo pero nadie le
     * pasaba el ciclo anterior, así que "Copiar de…" no aparecía nunca. Es el
     * mismo defecto que dejó inalcanzable al buscador de alumno, y por eso esta
     * prueba mira la pantalla y no las props.
     */
    await expect(page.getByText(new RegExp(`Copiar de ANTERIOR ${sello}`, "i"))).toBeVisible();
    await page.getByRole("button", { name: "Copiar" }).click();

    // Los montos llegan TAL CUAL: subirlos es la decisión que quien captura vino
    // a tomar, y adivinarla sería cobrarle a las familias un número que nadie
    // escribió.
    await expect(page.getByLabel("Colegiatura mensual")).toHaveValue("3200.00");
    await expect(page.getByLabel("Mensualidades")).toHaveValue("10");
  });
});
