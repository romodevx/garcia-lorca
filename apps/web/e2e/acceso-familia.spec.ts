import { expect, test, type Page } from "@playwright/test";

import { API_URL, apiAsOwner, openModule, signIn, uniqueSuffix, anyLevelId } from "./helpers";

/**
 * El colegio le da la app a una familia, de punta a punta.
 *
 * Es el camino que hasta ahora no existía: el guard anti-escalada del borde
 * rechazaba asignar el rol `guardian` a cualquiera, porque `family:read` no lo
 * tenía nadie, y la única forma de crear un tutor era escribir SQL. Lo que se
 * prueba aquí no es un endpoint: es que la secretaria puede hacerlo sola, desde
 * la pantalla donde ya trabaja, sin que nadie toque la base.
 *
 * Todo debajo del navegador es real — Next, el gateway, iam, students y un
 * Postgres vivo. Las capas de abajo tienen sus propias pruebas y ninguna puede
 * demostrar las costuras: aquí el permiso vive en iam, el contacto en students,
 * la composición en el gateway y el botón en el panel, y los cuatro tienen que
 * estar de acuerdo.
 *
 * Se monta su propio alumno por API (la precondición no es lo que se prueba) y
 * asserta por la UI.
 */

/**
 * Una CURP con la FORMA que valida la frontera de students (no una real: el
 * padrón nacional no es asunto de esta prueba, y una CURP inventada con forma
 * correcta es exactamente lo que el sistema debe aceptar).
 */
function shapedCurp(): string {
  const consonants = "BCDFGHJKLMNPQRSTVWXYZ";
  const pick = (from: string): string => from[Math.floor(Math.random() * from.length)]!;
  const inner = pick(consonants) + pick(consonants) + pick(consonants);
  return `${pick(consonants)}A${pick(consonants)}${pick(consonants)}200315M` +
    `DF${inner}${pick("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")}${pick("0123456789")}`;
}


/**
 * Abre la ficha del alumno desde el directorio: buscar por nombre y entrar.
 *
 * No se usa `pickStudent` del helper compartido porque ese es el selector de
 * OTRA pantalla (la de cobros). Aquí el camino real de la secretaria es el
 * directorio de Estudiantes, que es donde vive la lista de contactos.
 */
async function openStudentFile(page: Page, fullName: string): Promise<void> {
  await openModule(page, "Estudiantes");
  await page.getByRole("textbox", { name: "Buscar" }).fill(fullName);
  const open = page.getByRole("button", { name: new RegExp(`Ver ficha de ${fullName}`) }).first();
  await expect(open, `no apareció ${fullName} en el directorio`).toBeVisible();
  await open.click();
}

test.describe("dar acceso a la app", () => {
  test("una contacto del padrón se convierte en familia que entra a la app", async ({ page, request }) => {
    const api = await apiAsOwner(request);
    const stamp = uniqueSuffix();
    const pupilName = `Ana Paula ${stamp}`;
    const motherName = `Lucía Herrera ${stamp}`;
    const motherEmail = `lucia.${stamp}@garcialorca.edu.mx`;

    // --- precondición: un alumno y su madre, registrada sin cuenta ----------
    const pupil = await api.post(`${API_URL}/api/v1/students`, {
      data: {
        curp: shapedCurp(),
        firstName: `Ana Paula ${stamp}`,
        lastName: "Contreras Rojas",
        levelId: await anyLevelId(api),
        gradeGroup: `5A${stamp}`,
        guardianName: motherName,
      },
    });
    expect(pupil.ok(), `no se pudo crear el alumno: ${await pupil.text()}`).toBeTruthy();
    const pupilId = (await pupil.json()).id as string;

    const contact = await api.post(`${API_URL}/api/v1/contacts`, {
      data: { studentId: pupilId, relationship: "mother", fullName: motherName, email: motherEmail },
    });
    expect(contact.ok(), `no se pudo registrar el contacto: ${await contact.text()}`).toBeTruthy();
    expect((await contact.json()).hasAccount, "un contacto nace sin cuenta").toBe(false);

    // --- lo que se prueba: la secretaria lo hace desde el panel -------------
    await signIn(page);
    await openStudentFile(page, `${pupilName} Contreras Rojas`);

    await expect(page.getByText(motherName).first()).toBeVisible();
    const give = page.getByRole("button", { name: /Dar acceso a la app/ });
    await expect(give, "la madre tiene correo, así que el botón debe ofrecerse").toBeVisible();
    await give.click();

    // El panel no promete lo que no sabe: dice por dónde salió la liga. Lo que
    // no puede pasar nunca es que no diga nada.
    await expect(page.getByRole("status")).toBeVisible();

    // Y la marca cambia: de "hay que llamarle" a "puede entrar".
    await expect(page.getByText("App").first()).toBeVisible();
    await expect(page.getByRole("button", { name: /Dar acceso a la app/ }))
      .toHaveCount(0);

    // --- la costura que ninguna capa puede probar sola ----------------------
    // La madre existe en iam CON el rol de tutor, y el vínculo la deja ver a su
    // hija por la superficie auto-acotada de la familia. Sin el permiso nuevo en
    // el rol `owner` (ADR 0012) el botón de arriba habría respondido 403.
    const guardians = await api.get(`${API_URL}/api/v1/students/${pupilId}/guardians`);
    expect(guardians.ok()).toBeTruthy();
    const links = (await guardians.json()).guardians as { id: string; hasAccount: boolean; fullName: string }[];
    expect(links, "el alumno conserva UNA lista de contactos, no dos").toHaveLength(1);
    expect(links[0]?.hasAccount, "la cuenta quedó en la fila que ya estaba").toBe(true);
    // Tal cual se registró: las mayúsculas son cosa del formulario del panel, no
    // del API por el que esta prueba sembró el contacto.
    expect(links[0]?.fullName).toBe(motherName);
  });

  test("a un contacto sin correo no se le ofrece el botón, y se dice por qué", async ({ page, request }) => {
    const api = await apiAsOwner(request);
    const stamp = uniqueSuffix();
    const pupilName = `Diego ${stamp}`;
    const grandmother = `Rosa Méndez ${stamp}`;

    const pupil = await api.post(`${API_URL}/api/v1/students`, {
      data: {
        curp: shapedCurp(),
        firstName: `Diego ${stamp}`,
        lastName: "Salazar Martínez",
        levelId: await anyLevelId(api),
        gradeGroup: `1A${stamp}`,
        guardianName: grandmother,
      },
    });
    expect(pupil.ok(), `no se pudo crear el alumno: ${await pupil.text()}`).toBeTruthy();
    const pupilId = (await pupil.json()).id as string;

    // La abuela que puede recoger a la niña: un teléfono al que se llama, no una
    // cuenta. Es la mayoría de los contactos de un colegio.
    await api.post(`${API_URL}/api/v1/contacts`, {
      data: { studentId: pupilId, relationship: "other", fullName: grandmother, phone: "5551234567" },
    });

    await signIn(page);
    await openStudentFile(page, `${pupilName} Salazar Martínez`);

    await expect(page.getByText(grandmother).first()).toBeVisible();
    // Una acción que no puede tener éxito no debe poder apretarse — y decir por
    // qué es lo que convierte un botón ausente en algo que la oficina arregla.
    await expect(page.getByRole("button", { name: /Dar acceso a la app/ })).toHaveCount(0);
    await expect(page.getByText(/Falta el correo/)).toBeVisible();
  });
});
