import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as familiesApi from "../../families/api";
import type { FamiliesAccess } from "../../families/model";
import * as studentsApi from "../../students/api";
import { FamilyDrawer, type FamilyDrawerProps } from "./FamilyDrawer";
import type { FamilyPupil } from "./familyPupil";

vi.mock("../../families/api", async (importOriginal) => ({
  ...(await importOriginal<typeof familiesApi>()),
  getFamily: vi.fn(),
  listFamilyContacts: vi.fn(),
  renameFamily: vi.fn(),
  removeStudentFromFamily: vi.fn(),
  addStudentsToFamily: vi.fn(),
  createFamily: vi.fn(),
  addFamilyContact: vi.fn(),
  revokeFamilyAccess: vi.fn(),
  dissolveFamily: vi.fn(),
}));
vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  inviteContactToApp: vi.fn(),
}));
const api = vi.mocked(familiesApi);
const students = vi.mocked(studentsApi);

const OWNER: FamiliesAccess = { canReadGuardians: true, canWriteGuardians: true, canUpdateStudents: true };
const member = (id: string, firstName: string, gradeGroup: string): familiesApi.FamilyMember => ({
  id, firstName, lastName: "GARCÍA", maternalLastName: "LÓPEZ", level: "PRIMARIA", gradeGroup, admissionStatus: "enrolled", draft: false,
});
const detail = (over: Partial<familiesApi.FamilyDetail> = {}): familiesApi.FamilyDetail => ({
  id: "f1", name: "GARCÍA LÓPEZ", studentIds: ["s1", "s2"], accountCount: 1, createdAt: "",
  members: [member("s1", "ANA", "A"), member("s2", "LUIS", "B")], ...over,
});
const MOTHER: familiesApi.FamilyPerson = { fullName: "MARÍA LÓPEZ", relationship: "mother", phone: "5512345678", email: "maria@example.com", guardianUserId: "u1", hasApp: true, contacts: [{ studentId: "s1", contactId: "c1" }] };
const FATHER: familiesApi.FamilyPerson = { fullName: "JUAN GARCÍA", relationship: "father", phone: "", email: "juan@example.com", hasApp: false, contacts: [{ studentId: "s2", contactId: "c2" }, { studentId: "s1", contactId: "c3" }] };
const pupil = (id: string, name: string, room: string): FamilyPupil => ({ id, firstName: "", lastName: "", name, room, admissionStatus: "enrolled" });
const PUPILS = [pupil("s1", "GARCÍA LÓPEZ ANA", "3° PRIMARIA · A"), pupil("s2", "GARCÍA LÓPEZ LUIS", "1° PRIMARIA · B"), pupil("s9", "GARCÍA LÓPEZ TEO", "1° PREESCOLAR · A")];

function renderDrawer(over: Partial<FamilyDrawerProps> = {}): FamilyDrawerProps {
  const props: FamilyDrawerProps = {
    familyId: "f1",
    access: OWNER,
    pupils: PUPILS,
    candidates: [PUPILS[2] as FamilyPupil],
    onClose: vi.fn(),
    onChanged: vi.fn(),
    onRemoved: vi.fn(),
    onOpenStudent: vi.fn(),
    say: vi.fn(),
    ...over,
  };
  render(<FamilyDrawer {...props} />);
  return props;
}
const drawer = (): Promise<HTMLElement> => screen.findByRole("dialog", { name: /GARCÍA LÓPEZ/ });

beforeEach(() => {
  api.getFamily.mockResolvedValue(detail());
  api.listFamilyContacts.mockResolvedValue([MOTHER, FATHER]);
  api.renameFamily.mockImplementation(async (_b, id, name) => ({ id, name, studentIds: [], createdAt: "" }));
  api.addStudentsToFamily.mockResolvedValue({ id: "f1", name: "GARCÍA LÓPEZ", studentIds: [], createdAt: "" });
  api.removeStudentFromFamily.mockResolvedValue({ id: "f1", name: "GARCÍA LÓPEZ", studentIds: ["s2"], createdAt: "" });
  api.revokeFamilyAccess.mockResolvedValue(2);
  api.dissolveFamily.mockResolvedValue();
  api.addFamilyContact.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

describe("FamilyDrawer — the family", () => {
  it("shows its name, its children with their group, and its accounts", async () => {
    renderDrawer();
    const d = await drawer();
    expect(await within(d).findByRole("button", { name: "GARCÍA LÓPEZ ANA" })).toBeInTheDocument();
    expect(within(d).getByText("3° PRIMARIA · A")).toBeInTheDocument();
    expect(within(d).getByText("1 de 2 cuentas de la app")).toBeInTheDocument();
  });

  it("each child opens their own file", async () => {
    const user = userEvent.setup();
    const props = renderDrawer();
    const link = await screen.findByRole("button", { name: "GARCÍA LÓPEZ LUIS" });
    // QA-1: --forest has no dark value; --link holds 4.5:1 in both themes.
    expect(link.style.color).toBe("var(--link)");
    await user.click(link);
    expect(props.onOpenStudent).toHaveBeenCalledWith("s2");
  });

  it("renames in place", async () => {
    const user = userEvent.setup();
    const props = renderDrawer();
    await user.click(await screen.findByRole("button", { name: "Renombrar familia" }));
    const field = screen.getByRole("textbox", { name: /Nombre de la familia/ });
    await user.clear(field);
    await user.type(field, "LOS GARCÍA");
    await user.click(screen.getByRole("button", { name: "Guardar nombre" }));
    await waitFor(() => expect(api.renameFamily).toHaveBeenCalledWith(expect.any(String), "f1", "LOS GARCÍA"));
    expect(props.onChanged).toHaveBeenCalled();
  });

  it("offers no rename without students:update", async () => {
    renderDrawer({ access: { ...OWNER, canUpdateStudents: false } });
    await screen.findByRole("button", { name: "GARCÍA LÓPEZ ANA" });
    expect(screen.queryByRole("button", { name: "Renombrar familia" })).toBeNull();
  });
});

describe("FamilyDrawer — children", () => {
  it("removes a child at once and hands the undo to the screen", async () => {
    const user = userEvent.setup();
    const props = renderDrawer();
    await user.click(await screen.findByRole("button", { name: "Quitar a GARCÍA LÓPEZ ANA de la familia" }));
    await waitFor(() => expect(api.removeStudentFromFamily).toHaveBeenCalledWith(expect.any(String), "f1", "s1"));
    expect(props.onRemoved).toHaveBeenCalledWith({ familyId: "f1", familyName: "GARCÍA LÓPEZ", id: "s1", name: "GARCÍA LÓPEZ ANA", dissolved: false });
  });

  it("removing the last child says the family is gone", async () => {
    const user = userEvent.setup();
    api.getFamily.mockResolvedValue(detail({ studentIds: ["s1"], members: [member("s1", "ANA", "A")] }));
    api.removeStudentFromFamily.mockResolvedValue(null);
    const props = renderDrawer();
    await user.click(await screen.findByRole("button", { name: "Quitar a GARCÍA LÓPEZ ANA de la familia" }));
    expect(await screen.findByText(/La familia ya no existe/)).toBeInTheDocument();
    expect(props.onRemoved).toHaveBeenCalledWith(expect.objectContaining({ id: "s1", dissolved: true }));
    expect(props.onChanged).toHaveBeenCalled();
  });

  it("a second press while the removal travels does not send it twice", async () => {
    const user = userEvent.setup();
    let finish: (f: familiesApi.Family | null) => void = () => {};
    api.removeStudentFromFamily.mockReturnValue(new Promise((resolve) => { finish = resolve; }));
    renderDrawer();
    const button = await screen.findByRole("button", { name: "Quitar a GARCÍA LÓPEZ ANA de la familia" });
    await user.click(button);
    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button).toBeEnabled();
    await user.click(button);
    expect(api.removeStudentFromFamily).toHaveBeenCalledTimes(1);
    finish({ id: "f1", name: "GARCÍA LÓPEZ", studentIds: ["s2"], createdAt: "" });
  });

  it("adds a child without a family", async () => {
    const user = userEvent.setup();
    const props = renderDrawer();
    await screen.findByRole("button", { name: "GARCÍA LÓPEZ ANA" });
    await user.type(screen.getByRole("searchbox", { name: "Agregar hijo" }), "teo");
    await user.click(screen.getByRole("button", { name: "Agregar a GARCÍA LÓPEZ TEO" }));
    await waitFor(() => expect(api.addStudentsToFamily).toHaveBeenCalledWith(expect.any(String), "f1", ["s9"]));
    expect(props.onChanged).toHaveBeenCalled();
    expect(props.say).toHaveBeenCalledWith("GARCÍA LÓPEZ TEO se agregó a la familia");
  });

  it("says why a child could not be added", async () => {
    const user = userEvent.setup();
    api.addStudentsToFamily.mockRejectedValue(new familiesApi.FamiliesError(409, "x", "FAMILY_BUSY"));
    renderDrawer();
    await screen.findByRole("button", { name: "GARCÍA LÓPEZ ANA" });
    await user.type(screen.getByRole("searchbox", { name: "Agregar hijo" }), "teo");
    await user.click(screen.getByRole("button", { name: "Agregar a GARCÍA LÓPEZ TEO" }));
    expect(await screen.findByText("Otra persona está cambiando esta familia; intenta de nuevo")).toBeInTheDocument();
  });
});

describe("FamilyDrawer — adults", () => {
  it("lists every adult once, with relationship, phone, email and the app", async () => {
    renderDrawer();
    const adults = await screen.findByRole("region", { name: "Adultos" });
    const mother = await within(adults).findByRole("listitem", { name: "MARÍA LÓPEZ" });
    expect(within(mother).getByText(/Madre/)).toBeInTheDocument();
    expect(within(mother).getByText(/5512345678/)).toBeInTheDocument();
    expect(within(mother).getByText(/maria@example.com/)).toBeInTheDocument();
    expect(within(mother).getByText("App")).toBeInTheDocument();
  });

  it("gives an adult access through the contact invite, with its first contact", async () => {
    const user = userEvent.setup();
    students.inviteContactToApp.mockResolvedValue({
      contact: { id: "c2", studentId: "s2", guardianUserId: "u2", relationship: "father", fullName: "JUAN GARCÍA", phone: "", email: "juan@example.com", hasAccount: true },
      invitation: { url: "https://x", expiresAt: "", sent: true, via: "smtp" },
    });
    renderDrawer();
    const father = await screen.findByRole("listitem", { name: "JUAN GARCÍA" });
    await user.click(within(father).getByRole("button", { name: "Dar acceso a la app" }));
    await waitFor(() => expect(students.inviteContactToApp).toHaveBeenCalledWith(expect.any(String), "s2", "c2", {}));
    expect(await screen.findByText(/Le enviamos la liga a/)).toBeInTheDocument();
    await waitFor(() => expect(api.getFamily).toHaveBeenCalledTimes(2));
  });

  it("a family with two accounts says so instead of offering a third", async () => {
    api.getFamily.mockResolvedValue(detail({ accountCount: 2 }));
    renderDrawer();
    const father = await screen.findByRole("listitem", { name: "JUAN GARCÍA" });
    expect(within(father).queryByRole("button", { name: "Dar acceso a la app" })).toBeNull();
    expect(within(father).getByText("Ya tiene 2 cuentas de la app")).toBeInTheDocument();
  });

  it("the server's 409 says the same and re-reads the family", async () => {
    const user = userEvent.setup();
    students.inviteContactToApp.mockRejectedValue(new studentsApi.StudentsError(409, "x", "FAMILY_ACCOUNT_LIMIT"));
    renderDrawer();
    const father = await screen.findByRole("listitem", { name: "JUAN GARCÍA" });
    await user.click(within(father).getByRole("button", { name: "Dar acceso a la app" }));
    expect(await screen.findByText("Esta familia ya tiene 2 cuentas de la app")).toBeInTheDocument();
    await waitFor(() => expect(api.getFamily).toHaveBeenCalledTimes(2));
  });

  it("asks before using a staff account, and confirms on request", async () => {
    const user = userEvent.setup();
    students.inviteContactToApp
      .mockRejectedValueOnce(new studentsApi.StudentsError(409, "that email belongs to a staff account; confirm it is the same person"))
      .mockResolvedValueOnce({ contact: { id: "c2", studentId: "s2", guardianUserId: "u2", relationship: "father", fullName: "JUAN GARCÍA", phone: "", email: "juan@example.com", hasAccount: true }, invitation: null });
    renderDrawer();
    const father = await screen.findByRole("listitem", { name: "JUAN GARCÍA" });
    await user.click(within(father).getByRole("button", { name: "Dar acceso a la app" }));
    await user.click(await screen.findByRole("button", { name: "Sí, es la misma persona" }));
    await waitFor(() => expect(students.inviteContactToApp).toHaveBeenLastCalledWith(expect.any(String), "s2", "c2", { confirmStaffAccount: true }));
    expect(await screen.findByText(/ya tenía cuenta con ese correo/)).toBeInTheDocument();
  });

  it("takes the app away from the whole family after asking", async () => {
    const user = userEvent.setup();
    const props = renderDrawer();
    const mother = await screen.findByRole("listitem", { name: "MARÍA LÓPEZ" });
    await user.click(within(mother).getByRole("button", { name: "Quitar acceso" }));
    const confirm = await screen.findByRole("dialog", { name: "Quitar el acceso a la app" });
    expect(within(confirm).getByText(/MARÍA LÓPEZ ya no podrá entrar a la app/)).toBeInTheDocument();
    expect(within(confirm).getByRole("button", { name: "Quitar acceso" }).style.background).toBe("var(--red)");
    await user.click(within(confirm).getByRole("button", { name: "Quitar acceso" }));
    await waitFor(() => expect(api.revokeFamilyAccess).toHaveBeenCalledWith(expect.any(String), "f1", "u1"));
    expect(props.say).toHaveBeenCalledWith("MARÍA LÓPEZ ya no tiene acceso a la app");
    expect(await drawer()).toBeInTheDocument();
  });

  it("records an adult on every child; errors only on save, the button never dead", async () => {
    const user = userEvent.setup();
    renderDrawer();
    await user.click(await screen.findByRole("button", { name: "Agregar adulto" }));
    const save = screen.getByRole("button", { name: "Guardar adulto" });
    expect(screen.queryByText("Falta este dato")).toBeNull();
    expect(save).toBeEnabled();
    await user.click(save);
    expect(await screen.findByText("Falta este dato")).toBeInTheDocument();
    expect(screen.getByText("Elige el parentesco")).toBeInTheDocument();
    expect(api.addFamilyContact).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Madre" }));
    await user.type(screen.getByRole("textbox", { name: /Nombre del adulto/ }), "ROSA PÉREZ");
    await user.type(screen.getByRole("textbox", { name: /Correo del adulto/ }), "rosa@example.com");
    await user.click(screen.getByRole("checkbox", { name: "Contacto de emergencia" }));
    await user.click(save);
    await waitFor(() => expect(api.addFamilyContact).toHaveBeenCalledWith(expect.any(String), "f1", {
      relationship: "mother", fullName: "ROSA PÉREZ", phone: "", email: "rosa@example.com", isEmergency: true,
    }));
  });

  it.each([
    ["sent", { url: "https://x", expiresAt: "", sent: true, via: "smtp" }, /Le enviamos la liga a/],
    ["not sent: the link to copy", { url: "https://liga.example/abc", expiresAt: "", sent: false, sendError: "smtp caído" }, /No se pudo enviar/],
  ])("keeps the invite's outcome after the re-read gives the person an account (%s)", async (_label, invitation, said) => {
    const user = userEvent.setup();
    students.inviteContactToApp.mockResolvedValue({
      contact: { id: "c2", studentId: "s2", guardianUserId: "u2", relationship: "father", fullName: "JUAN GARCÍA", phone: "", email: "juan@example.com", hasAccount: true },
      invitation,
    });
    api.listFamilyContacts.mockResolvedValueOnce([MOTHER, FATHER]).mockResolvedValue([MOTHER, { ...FATHER, guardianUserId: "u2", hasApp: true }]);
    renderDrawer();
    const father = await screen.findByRole("listitem", { name: "JUAN GARCÍA" });
    await user.click(within(father).getByRole("button", { name: "Dar acceso a la app" }));
    await waitFor(() => expect(api.listFamilyContacts).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(within(screen.getByRole("listitem", { name: "JUAN GARCÍA" })).getByText("App")).toBeInTheDocument());
    expect(within(screen.getByRole("listitem", { name: "JUAN GARCÍA" })).getByText(said)).toBeInTheDocument();
    if (!invitation.sent) expect(screen.getByDisplayValue("https://liga.example/abc")).toBeInTheDocument();
  });

  it("Esc on an untouched adult form closes the form, not the drawer", async () => {
    const user = userEvent.setup();
    const props = renderDrawer();
    await user.click(await screen.findByRole("button", { name: "Agregar adulto" }));
    await user.click(screen.getByRole("textbox", { name: /Nombre del adulto/ }));
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("button", { name: "Guardar adulto" })).toBeNull();
    expect(props.onClose).not.toHaveBeenCalled();
  });

  it("Esc on a half-written adult asks before discarding it", async () => {
    const user = userEvent.setup();
    const props = renderDrawer();
    await user.click(await screen.findByRole("button", { name: "Agregar adulto" }));
    const field = screen.getByRole("textbox", { name: /Nombre del adulto/ });
    await user.type(field, "ROSA");
    await user.keyboard("{Escape}");
    expect(props.onClose).not.toHaveBeenCalled();
    expect(screen.getByText("¿Descartar a este adulto?")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Seguir editando" }));
    expect(screen.getByRole("textbox", { name: /Nombre del adulto/ })).toHaveValue("ROSA");
    await user.click(screen.getByRole("textbox", { name: /Nombre del adulto/ }));
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "Descartar" }));
    expect(screen.queryByRole("button", { name: "Guardar adulto" })).toBeNull();
    expect(props.onClose).not.toHaveBeenCalled();
  });

  it("closing the drawer with a half-written adult asks the same", async () => {
    const user = userEvent.setup();
    const props = renderDrawer();
    await user.click(await screen.findByRole("button", { name: "Agregar adulto" }));
    await user.type(screen.getByRole("textbox", { name: /Nombre del adulto/ }), "ROSA");
    await user.click(screen.getByRole("button", { name: "Cerrar" }));
    expect(props.onClose).not.toHaveBeenCalled();
    expect(screen.getByText("¿Descartar a este adulto?")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Descartar" }));
    expect(props.onClose).toHaveBeenCalled();
  });

  it("a missing relationship is tied to its buttons and takes the focus", async () => {
    const user = userEvent.setup();
    renderDrawer();
    await user.click(await screen.findByRole("button", { name: "Agregar adulto" }));
    await user.type(screen.getByRole("textbox", { name: /Nombre del adulto/ }), "ROSA");
    await user.click(screen.getByRole("button", { name: "Guardar adulto" }));
    const mother = screen.getByRole("button", { name: "Madre" });
    expect(mother).toHaveAccessibleDescription("Elige el parentesco");
    await waitFor(() => expect(mother).toHaveFocus());
  });

  it("without guardians:read there are no adults and no read of them", async () => {
    renderDrawer({ access: { canReadGuardians: false, canWriteGuardians: false, canUpdateStudents: true } });
    await screen.findByRole("button", { name: "GARCÍA LÓPEZ ANA" });
    expect(screen.queryByRole("region", { name: "Adultos" })).toBeNull();
    expect(api.listFamilyContacts).not.toHaveBeenCalled();
  });
});

describe("FamilyDrawer — dissolving and permissions", () => {
  it("dissolves after saying nobody loses access", async () => {
    const user = userEvent.setup();
    const props = renderDrawer();
    await user.click(await screen.findByRole("button", { name: "Disolver familia" }));
    const confirm = await screen.findByRole("dialog", { name: "Disolver la familia" });
    expect(within(confirm).getByText(/los hijos se quedan sin familia; nadie pierde acceso/)).toBeInTheDocument();
    expect(within(confirm).getByRole("button", { name: "Disolver" }).style.background).toBe("var(--red)");
    await user.click(within(confirm).getByRole("button", { name: "Disolver" }));
    await waitFor(() => expect(api.dissolveFamily).toHaveBeenCalledWith(expect.any(String), "f1"));
    expect(props.onChanged).toHaveBeenCalled();
    expect(props.onClose).toHaveBeenCalled();
    expect(props.say).toHaveBeenCalledWith("Familia GARCÍA LÓPEZ disuelta");
  });

  it("a caller without guardians:write only reads", async () => {
    renderDrawer({ access: { canReadGuardians: true, canWriteGuardians: false, canUpdateStudents: false } });
    const father = await screen.findByRole("listitem", { name: "JUAN GARCÍA" });
    expect(within(father).queryByRole("button", { name: "Dar acceso a la app" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Quitar a .* de la familia/ })).toBeNull();
    expect(screen.queryByRole("searchbox", { name: "Agregar hijo" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Quitar acceso" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Agregar adulto" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Disolver familia" })).toBeNull();
  });
});

function Harness(): ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>Abrir familia</button>
      {open && (
        <FamilyDrawer familyId="f1" access={OWNER} pupils={PUPILS} candidates={[]} onClose={() => setOpen(false)}
          onChanged={vi.fn()} onRemoved={vi.fn()} onOpenStudent={vi.fn()} say={vi.fn()} />
      )}
    </>
  );
}

describe("FamilyDrawer — focus", () => {
  it("returns to what opened it, even after a confirmation replaced it", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const trigger = screen.getByRole("button", { name: "Abrir familia" });
    await user.click(trigger);
    await user.click(await screen.findByRole("button", { name: "Disolver familia" }));
    await user.click(within(await screen.findByRole("dialog", { name: "Disolver la familia" })).getByRole("button", { name: "Cancelar" }));
    await user.click(within(await drawer()).getByRole("button", { name: "Cerrar" }));
    await waitFor(() => expect(trigger).toHaveFocus());
  });
});
