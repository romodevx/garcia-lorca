import { expect, test, type Page } from "@playwright/test";
import { openModule, pickStudent, signIn, uniqueSuffix } from "./helpers";

/**
 * THE DEMO, in order, against a database that starts empty.
 *
 * This is not a set of independent tests. It is one story told in sequence — the
 * school being set up from nothing — because that is what the walk-through
 * shows and because each step genuinely needs the last: there is no group
 * without a grade, no grade without a level, no level without a school year.
 *
 * Nothing here is seeded through the API. Every row is created by clicking, the
 * way it will be in front of an audience, so a screen that has drifted from its
 * service fails HERE rather than on stage.
 *
 * `test.describe.serial` is what makes the sequence honest: one failure stops the
 * rest, because a demo that cannot create a level has nothing to say about
 * groups.
 */
test.describe.serial("setting up a school from zero", () => {
  // One suffix for the whole run, so the story is internally consistent and a
  // re-run never collides with the last one.
  const tag = uniqueSuffix();
  const cycle = `Ciclo ${tag}`;
  const level = "Primaria";
  const grade = `1° ${tag}`;
  const group = `A ${tag}`;
  const subject = `Matemáticas ${tag}`;
  /*
   * En MAYÚSCULAS desde el principio, como se guarda.
   *
   * La capa única de captura sube a mayúsculas todo dato de catálogo, así que el
   * artículo queda como "SUÉTER 3N7K" en la base. El resto de la prueba lo
   * buscaba con la caja original y no lo encontraba: el <select> de "Prenda"
   * existía, vacío de la opción esperada. Es el mismo motivo por el que niveles y
   * grupos ya pasaban por `comoSeGuarda`.
   */
  const garment = `SUÉTER ${tag.toUpperCase()}`;

  test("the director signs in", async ({ page }) => {
    await signIn(page, true);
    await expect(page.getByRole("heading", { level: 1, name: /🌱/ })).toBeVisible();
  });

  test("the dashboard offers the modules to configure", async ({ page }) => {
    await signIn(page);
    // Inventario is checked by name on purpose: its tile was invisible for a
    // whole slice because it asked for a permission that had been retired, and
    // nothing failed anywhere.
    for (const name of ["Estudiantes", "Finanzas", "Calificaciones", "Asistencia", "Inventario", "Configuración"]) {
      await expect(page.getByText(name, { exact: false }).first()).toBeVisible();
    }
  });

  test("1 · the school year", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Configuración");

    // Todo el catálogo se captura en cajones, no en formas en línea. Esta suite
    // seguía escrita contra la versión en línea, y por eso pedía "Inicio del
    // ciclo" cuando el campo dice "Inicia".
    await page.getByRole("button", { name: "Nuevo ciclo" }).click();
    const cicloNuevo = page.getByRole("dialog", { name: "Nuevo ciclo escolar" });
    await cicloNuevo.getByLabel("Nombre del ciclo").fill(cycle);
    await cicloNuevo.getByLabel("Inicia").fill("2026-08-31");
    await cicloNuevo.getByLabel("Termina").fill("2027-07-09");
    await cicloNuevo.getByRole("button", { name: "Crear ciclo" }).click();

    // `.first()`: el chip del ciclo y el botón "Activar ciclo <nombre>" traen
    // los dos el nombre. El chip es el que va primero en el DOM.
    await expect(page.getByRole("button", { name: new RegExp(cycle, "i") }).first()).toBeVisible();

    // A cycle is born a draft. Everything downstream asks "which year is active"
    // — groups, tuition, enrolment — so putting it into service is part of
    // opening a school, not an afterthought.
    await page.getByRole("button", { name: new RegExp(cycle, "i") }).first().click();
    await page.getByRole("button", { name: "Activar ciclo" }).click();
    await expect(page.getByText(/es el ciclo en curso/i)).toBeVisible();
  });

  /**
   * The failure the user hit on their own first attempt. The form used to answer
   * "Intenta de nuevo" for a name the school already used — advice a person can
   * follow forever without getting anywhere.
   */
  test("1b · a repeated year says what is wrong", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Configuración");

    // Todo el catálogo se captura en cajones, no en formas en línea. Esta suite
    // seguía escrita contra la versión en línea, y por eso pedía "Inicio del
    // ciclo" cuando el campo dice "Inicia".
    await page.getByRole("button", { name: "Nuevo ciclo" }).click();
    const cicloNuevo = page.getByRole("dialog", { name: "Nuevo ciclo escolar" });
    await cicloNuevo.getByLabel("Nombre del ciclo").fill(cycle);
    await cicloNuevo.getByLabel("Inicia").fill("2026-08-31");
    await cicloNuevo.getByLabel("Termina").fill("2027-07-09");
    await cicloNuevo.getByRole("button", { name: "Crear ciclo" }).click();

    await expect(page.getByText(/Ya existe un ciclo con ese nombre/)).toBeVisible();
  });

  test("2 · a level, a grade, a group and a subject", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Configuración");
    await page.getByRole("button", { name: new RegExp(cycle, "i") }).first().click();

    await page.getByRole("button", { name: "Nuevo nivel" }).click();
    const nivelNuevo = page.getByRole("dialog", { name: "Nuevo nivel educativo" });
    await nivelNuevo.getByLabel("Nombre del nivel").fill(level);
    await nivelNuevo.getByLabel("Clave SEP").fill(`P${tag.slice(-2).toUpperCase()}`);
    // El nivel no se guarda hasta que alguien dice cómo se inscribe en él.
    await nivelNuevo.getByRole("radio", { name: "Por ciclo" }).check();
    await nivelNuevo.getByRole("button", { name: "Crear nivel" }).click();
    await expect(page.getByRole("button", { name: new RegExp(level, "i") }).first()).toBeVisible();

    // El grado ya no abre cajón: se captura en su propio renglón y el orden
    // llega propuesto, así que basta el nombre y Enter.
    await page.getByRole("button", { name: "Nuevo grado" }).click();
    await page.getByLabel("Nombre del grado").fill(grade);
    await page.getByLabel("Nombre del grado").press("Enter");
    await expect(page.getByRole("button", { name: new RegExp(grade, "i") }).first()).toBeVisible();

    // Creating the grade is not enough — the groups and subjects panels only
    // appear once a grade is SELECTED ("Selecciona un grado para gestionar
    // materias y grupos"). Worth knowing before doing this on stage: the screen
    // looks finished and half of it is not there yet.
    await page.getByRole("button", { name: new RegExp(grade, "i") }).first().click();

    // El grupo también: la letra y el cupo llegan propuestos.
    await page.getByRole("button", { name: "Nuevo grupo" }).click();
    await page.getByLabel("Nombre del grupo").fill(group);
    await page.getByLabel("Nombre del grupo").press("Enter");
    await expect(page.getByText(group, { exact: false }).first()).toBeVisible();

    // La materia (T13): nombre + grados; con un grado elegido arriba, va a ese.
    const subjectName = page.getByRole("textbox", { name: /Nombre de la materia/ });
    await subjectName.fill(subject);
    await subjectName.press("Enter");
    await expect(page.getByText(subject, { exact: false }).first()).toBeVisible();
  });

  /**
   * Docentes is its own module now, not the bottom half of Configuración. The
   * demo walks it as a separate stop for that reason — and because a school
   * hires its staff before it enrols anybody.
   */
  test("2b · the first teacher, and what they teach", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Docentes");

    const roster = page.getByRole("form", { name: "Agregar profesor" });
    await roster.getByLabel("Nombre").fill("Gabriela");
    await roster.getByLabel("Apellidos").fill(`Rojas ${tag}`);
    await roster.getByLabel("Correo").fill(`gabriela.${tag}@garcialorca.edu.mx`);
    await roster.getByRole("button", { name: "Crear profesor" }).click();

    await expect(page.getByRole("list").getByText(`Rojas ${tag}`)).toBeVisible();

    // The assignment is the knot: (grupo, materia, profesor). The cycle is
    // derived server-side from the group, which is why it is never sent.
    const assign = page.getByRole("form", { name: "Asignar profesor" });
    await assign.getByRole("combobox", { name: "Ciclo" }).selectOption({ label: comoSeGuarda(cycle) });
    await assign.getByRole("combobox", { name: "Grupo" }).selectOption({ label: comoSeGuarda(group) });
    await assign.getByRole("combobox", { name: "Materia" }).selectOption({ label: comoSeGuarda(subject) });
    await assign.getByRole("combobox", { name: "Profesor" }).selectOption({ label: comoSeGuarda(`Gabriela Rojas ${tag}`) });
    await assign.getByRole("button", { name: "Asignar profesor" }).click();

    // The assignment list, not the dropdown that also carries the subject name:
    // what is being asserted is that the knot was tied, not that an option exists.
    await expect(
      page.getByRole("listitem").filter({ hasText: subject }).filter({ hasText: `Rojas ${tag}` }),
    ).toBeVisible();
  });

  test("3 · the first pupil", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Estudiantes");

    // The form lives behind the primary action, not on the page.
    // Three buttons say this — header, empty state and FAB — and any of them
    // opens the same drawer.
    await page.getByRole("button", { name: "Nuevo alumno" }).first().click();
    // Scoped to the drawer: "CURP" also names the roster's search box, and an
    // unscoped match takes whichever came first in the DOM.
    const drawer = page.getByRole("dialog", { name: "Nuevo alumno" });
    // Sin el " *": el asterisco es hermano del <label>, no su hijo, para que el
    // nombre accesible del campo siga siendo "CURP" y no "CURP *".
    await drawer.getByRole("textbox", { name: "CURP" }).fill(curpFor(tag));
    // Surnames first (T3), the way the school reads its lists.
    await drawer.getByRole("textbox", { name: "Apellido paterno" }).fill(`Villalobos ${tag}`);
    await drawer.getByRole("textbox", { name: "Nombre(s)" }).fill("Zoe");
    // Level and grade are cascading selects, and the GROUP is never asked for:
    // it is chosen by whichever one still has a seat.
    await drawer.getByRole("combobox", { name: "Nivel" }).selectOption({ label: comoSeGuarda(level) });
    await drawer.getByRole("combobox", { name: "Grado" }).selectOption({ label: comoSeGuarda(grade) });
    await drawer.getByRole("textbox", { name: "Tutor" }).fill("Marisol Villalobos");
    await drawer.getByRole("button", { name: "Crear alumno" }).click();

    /*
     * Se BUSCA, no se espera que caiga en la primera pantalla.
     *
     * La lista ordena por nombre y pagina; en una escuela con cuatrocientos
     * alumnos el recién creado casi nunca está arriba. La versión anterior de
     * esta línea sólo funcionaba con la base vacía, y entonces no probaba que el
     * alumno existiera: probaba que la lista fuera corta.
     */
    await page.getByRole("textbox", { name: "Buscar" }).fill(`Villalobos ${tag}`);
    await expect(page.getByText(`Villalobos ${tag}`, { exact: false }).first()).toBeVisible();
  });

  /**
   * The money the school actually lives on. Two things happen here that are worth
   * watching on stage: a level with no table says so instead of showing zeros,
   * and the run can be pressed twice without charging anybody twice.
   */
  test("3c · what the level costs, and the month's charges", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Configuración");
    await page.getByRole("button", { name: new RegExp(cycle, "i") }).first().click();

    // Price the level the pupil is enrolled in.
    const row = page.getByRole("row").filter({ hasText: level });
    await row.getByRole("button", { name: /Definir|Editar/ }).click();

    // El tabulador es una PÁGINA, no un cajón: catorce campos en cinco
    // secciones no caben en uno sin partir la forma en columnas.
    await expect(page.getByRole("heading", { name: /^Precios de / })).toBeVisible();

    // La inscripción se captura por conceptos con su vigencia (ADR 0011): un
    // monto único dejó de existir.
    await page.getByLabel("Nuevo concepto").fill("INSCRIPCIÓN");
    await page.getByRole("button", { name: /Agregar concepto/ }).click();
    await page.getByLabel("Monto").fill("5000");
    await page.getByLabel("Vigente desde").fill("2026-08-01");
    await page.getByRole("button", { name: /Agregar monto/ }).click();

    await page.getByLabel("Colegiatura mensual").fill("3200");
    await page.getByRole("button", { name: "Guardar tabulador" }).click();

    await expect(page.getByRole("row").filter({ hasText: level })).toContainText("$3200.00");

    // Run the month, then run it again. The second press is the demonstration:
    // everything comes back as already existing.
    await page.getByRole("button", { name: /Generar cargos del mes/ }).click();
    const summary = page.getByRole("status");
    await expect(summary).toBeVisible();

    await page.getByRole("button", { name: /Generar cargos del mes/ }).click();
    await expect(summary).toContainText(/Se generaron 0 colegiaturas/);
  });

  test("4 · the uniform shop's catalogue", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Inventario");
    await page.getByRole("tab", { name: "Catálogo" }).click();
    // "Nuevo artículo", no "Nueva prenda": el inventario dejó de ser sólo de ropa
    // en 491a2b9f y el botón se renombró con él. Esta prueba se quedó pidiendo el
    // nombre viejo, así que llevaba desde entonces en rojo -- y con ella los seis
    // pasos que vienen detrás, porque la suite es serial.
    await page.getByRole("button", { name: "Nuevo artículo" }).first().click();

    await page.getByLabel("Nombre").fill(garment);
    await page.getByLabel("Precio").fill("650");
    await page.getByRole("button", { name: "Crear y poner tallas" }).click();

    // It goes straight into sizes: a garment with no sizes cannot be sold, and
    // stopping there would leave the job looking finished.
    const sizes = page.getByRole("textbox", { name: "Tallas" });
    await expect(sizes).toBeVisible();
    await sizes.fill("CH, M, G");
    await page.getByRole("button", { name: /Guardar 3 tallas/ }).click();

    // It says what it did rather than saving in silence.
    await expect(page.getByText(/3 tallas creadas/)).toBeVisible();
  });

  test("5 · the first box arrives", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Inventario");
    await page.getByRole("tab", { name: "Existencias" }).click();
    await page.getByRole("button", { name: "Entrada de almacén" }).click();

    await page.getByLabel("Prenda").selectOption({ label: garment });
    await page.getByLabel("Piezas talla CH").fill("4");
    await page.getByLabel("Piezas talla M").fill("6");
    await page.getByLabel("Piezas talla G").fill("2");
    await page.getByLabel("Nota").fill("Primera entrega del proveedor");
    await page.getByRole("button", { name: /Guardar 12 piezas/ }).click();

    await expect(page.getByRole("dialog")).toBeHidden();
    await expect(page.getByText("6 disponibles").first()).toBeVisible();
  });

  test("6 · the first sale", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Inventario");

    await addToBasket(page, garment, "M");
    await pickStudent(page, `Villalobos ${tag} Zoe`);
    await page.getByRole("button", { name: /Cobrar y entregar/ }).click();

    // The folio is the only handle on the sale afterwards, so it is what the
    // screen shows and what this asserts.
    await expect(page.getByText(/^U-\d{4}-\d{6}$/)).toBeVisible();
  });

  test("7 · the sale is in the order list and the stock went down", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Inventario");

    await page.getByRole("tab", { name: "Pedidos" }).click();
    await expect(page.getByText(/U-\d{4}-\d{6}/).first()).toBeVisible();

    await page.getByRole("tab", { name: "Existencias" }).click();
    await expect(page.getByText("5 disponibles").first()).toBeVisible();
  });

  test("8 · the till refuses to sell what is not there", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Inventario");

    // Three of the two G there are.
    for (let i = 0; i < 3; i++) {
      await addToBasket(page, garment, "G");
    }
    await pickStudent(page, `Villalobos ${tag} Zoe`);
    await page.getByRole("button", { name: /Cobrar y entregar/ }).click();

    // Next paints an empty route-announcer with role=alert, so the till's own
    // banner is the one that carries text.
    const alert = page.getByRole("alert").filter({ hasText: /./ });
    await expect(alert).toBeVisible();
    // Its own sentence, not the one shown for a conflict: an empty shelf
    // is not the cashier's fault and must not read like it.
    await expect(alert).toContainText(/ya no está disponible/i);
    // And the detail line names the garment and the size, in the language the
    // person at the counter speaks. It used to arrive in English, straight from
    // the service's internal wording.
    await expect(alert).toContainText(new RegExp(`no quedan suficientes ${garment} en talla G`));
  });

  test("9 · a sale with no pupil still has to say who bought it", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Inventario");

    await addToBasket(page, garment, "CH");
    await page.getByRole("button", { name: "Venta sin alumno" }).click();

    await expect(page.getByText(/no quedará en ningún estado de cuenta/)).toBeVisible();
    await expect(page.getByRole("button", { name: /Cobrar y entregar/ })).toBeDisabled();

    await page.getByLabel("Nota").fill("Tío de Zoe Villalobos");
    await expect(page.getByRole("button", { name: /Cobrar y entregar/ })).toBeEnabled();
  });

  test("10 · a stock correction will not go through without a reason", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Inventario");
    await page.getByRole("tab", { name: "Existencias" }).click();
    await page.getByRole("button", { name: new RegExp(`Ajustar ${garment} talla M`) }).click();

    await page.getByLabel("Diferencia").fill("-1");
    // This is the one write that can conceal a shortfall.
    await expect(page.getByRole("button", { name: "Guardar ajuste" })).toBeDisabled();

    await page.getByLabel("Motivo (obligatorio)").fill("una pieza dañada en bodega");
    await expect(page.getByRole("button", { name: "Guardar ajuste" })).toBeEnabled();
  });
});

/** A syntactically valid CURP, unique per run. Never a real person's. */
/**
 * A CURP the backend will accept: the shape check wants a vowel (or X) in the
 * second slot and consonants in the 12th-14th, so the tag can only vary the
 * slots that are free. Two letters plus two digits is 26*26*100 combinations —
 * enough that repeated demo runs against the same database do not collide.
 */
/**
 * Puts one piece in the till's basket.
 *
 * Both clicks are needed every time: choosing a size collapses the garment and
 * size steps into the basket line, so a second piece is a second trip through
 * the picker. The garment regex is anchored because the basket line offers a
 * "Quitar <garment> talla G" button that would otherwise match too.
 */
async function addToBasket(page: Page, garmentName: string, size: string): Promise<void> {
  await page.getByRole("button", { name: new RegExp(`^${garmentName}`) }).click();
  await page.getByRole("button", { name: new RegExp(`^${size}\\s`) }).click();
}

/**
 * Como el sistema guarda el nombre, no como se tecleó.
 *
 * Los nombres de catálogo y de personas se capturan en mayúsculas, así que una
 * <option> ya no dice "Ciclo 5p7u" sino "CICLO 5P7U", y selectOption({label})
 * compara exacto.
 */
function comoSeGuarda(nombre: string): string {
  return nombre.toLocaleUpperCase("es-MX");
}

function curpFor(tag: string): string {
  const A = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";
  let hash = 7;
  for (const ch of tag) hash = (hash * 31 + ch.charCodeAt(0)) % 67600;
  const first = A[hash % 26];
  const third = A[Math.floor(hash / 26) % 26];
  const digits = String(Math.floor(hash / 676) % 100).padStart(2, "0");
  return `${first}A${third}X150312MDFRNN${digits}`;
}
