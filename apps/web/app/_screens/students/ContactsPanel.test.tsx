import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import { ContactsPanel } from "./ContactsPanel";

vi.mock("../../students/api", () => ({
  listContacts: vi.fn(),
  addContact: vi.fn(),
  inviteContactToApp: vi.fn(),
  updateContact: vi.fn(),
  removeContact: vi.fn(),
}));
const students = vi.mocked(studentsApi);

beforeEach(() => {
  students.listContacts.mockResolvedValue([
    { id: "gl-1", studentId: "st-1", guardianUserId: "usr-1", relationship: "mother", fullName: "Ana Cruz", phone: "5559876543", email: "", hasAccount: true },
    { id: "gl-2", studentId: "st-1", guardianUserId: "", relationship: "other", fullName: "Rosa Méndez", phone: "5551234567", email: "", hasAccount: false },
  ]);
});
afterEach(() => vi.clearAllMocks());

describe("ContactsPanel", () => {
  it("lists everyone the school can call", async () => {
    render(<ContactsPanel studentId="st-1" />);

    expect(await screen.findByText("Ana Cruz")).toBeInTheDocument();
    expect(screen.getByText("Rosa Méndez")).toBeInTheDocument();
  });

  // The office needs to tell apart who can see the app from who is only a
  // number to call — otherwise they stop calling the ones who never got a login.
  it("marks the contact who can open the app", async () => {
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Ana Cruz");

    expect(screen.getAllByText("App")).toHaveLength(1);
  });

  it("adds somebody with a phone and no account", async () => {
    const user = userEvent.setup();
    students.addContact.mockResolvedValue({
      id: "gl-3", studentId: "st-1", guardianUserId: "", relationship: "other",
      fullName: "Luis Cruz", phone: "5550000000", email: "", hasAccount: false,
    });
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Ana Cruz");

    await user.type(screen.getByLabelText("Nombre del contacto"), "Luis Cruz");
    await user.type(screen.getByLabelText("Teléfono del contacto"), "5550000000");
    await user.click(screen.getByRole("button", { name: /Agregar/ }));

    await waitFor(() => expect(students.addContact).toHaveBeenCalled());
    const sent = students.addContact.mock.calls[0]?.[1];
    expect(sent?.fullName).toBe("LUIS CRUZ");
    expect(sent?.phone).toBe("5550000000");
  });

  it("refuses a phone number with nobody attached to it", async () => {
    const user = userEvent.setup();
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Ana Cruz");

    await user.type(screen.getByLabelText("Teléfono del contacto"), "5550000000");
    await user.click(screen.getByRole("button", { name: /Agregar/ }));

    expect(students.addContact).not.toHaveBeenCalled();
    expect(await screen.findByRole("alert")).toBeInTheDocument();
  });

  // The review asked for numbers AND emails. The column existed from the start
  // and the form never asked for one.
  it("records a correo too", async () => {
    const user = userEvent.setup();
    students.addContact.mockResolvedValue({
      id: "gl-3", studentId: "st-1", guardianUserId: "", relationship: "other",
      fullName: "Luis Cruz", phone: "", email: "luis@example.com", hasAccount: false,
    });
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Ana Cruz");

    await user.type(screen.getByLabelText("Nombre del contacto"), "Luis Cruz");
    await user.type(screen.getByLabelText("Correo del contacto"), "luis@example.com");
    await user.click(screen.getByRole("button", { name: /Agregar/ }));

    await waitFor(() => expect(students.addContact).toHaveBeenCalled());
    expect(students.addContact.mock.calls[0]?.[1]?.email).toBe("luis@example.com");
  });
});

// --- dar acceso a la app ---------------------------------------------------

describe("ContactsPanel — dar acceso a la app", () => {
  // The whole point: a contact the office already typed becomes a family that
  // can open the app, with one press, where the secretary already works.
  it("gives a contact with an email access to the app", async () => {
    const user = userEvent.setup();
    students.listContacts.mockResolvedValue([
      { id: "gl-2", studentId: "st-1", guardianUserId: "", relationship: "father", fullName: "Mateo Contreras", phone: "", email: "mateo@example.com", hasAccount: false },
    ]);
    students.inviteContactToApp.mockResolvedValue({
      contact: { id: "gl-2", studentId: "st-1", guardianUserId: "usr-9", relationship: "father", fullName: "Mateo Contreras", phone: "", email: "mateo@example.com", hasAccount: true },
      invitation: { url: "", expiresAt: "", sent: true, via: "smtp" },
    });
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Mateo Contreras");

    await user.click(screen.getByRole("button", { name: /Dar acceso a la app/ }));

    await waitFor(() => expect(students.inviteContactToApp).toHaveBeenCalled());
    expect(students.inviteContactToApp.mock.calls[0]?.[1]).toBe("st-1");
    expect(students.inviteContactToApp.mock.calls[0]?.[2]).toBe("gl-2");
  });

  // A contact with no address has nowhere to receive the invitation, so the
  // button is not offered: an action that cannot succeed should not be pressable.
  it("does not offer the button to a contact with no email", async () => {
    students.listContacts.mockResolvedValue([
      { id: "gl-3", studentId: "st-1", guardianUserId: "", relationship: "other", fullName: "Rosa Méndez", phone: "5551234567", email: "", hasAccount: false },
    ]);
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Rosa Méndez");

    expect(screen.queryByRole("button", { name: /Dar acceso a la app/ })).not.toBeInTheDocument();
    expect(screen.getByText(/Falta el correo/)).toBeInTheDocument();
  });

  it("does not offer the button to somebody who already has the app", async () => {
    students.listContacts.mockResolvedValue([
      { id: "gl-1", studentId: "st-1", guardianUserId: "usr-1", relationship: "mother", fullName: "Ana Cruz", phone: "", email: "ana@example.com", hasAccount: true },
    ]);
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Ana Cruz");

    expect(screen.queryByRole("button", { name: /Dar acceso a la app/ })).not.toBeInTheDocument();
  });

  // THE ONE THE SCHOOL WILL LIVE WITH. The provider's message reliably lands in
  // spam, so telling the office "correo enviado" and stopping there is how a
  // family waits a week for a link that was always in their junk folder.
  it("warns that the provider's message tends to land in spam", async () => {
    const user = userEvent.setup();
    students.listContacts.mockResolvedValue([
      { id: "gl-2", studentId: "st-1", guardianUserId: "", relationship: "father", fullName: "Mateo Contreras", phone: "", email: "mateo@example.com", hasAccount: false },
    ]);
    students.inviteContactToApp.mockResolvedValue({
      contact: { id: "gl-2", studentId: "st-1", guardianUserId: "usr-9", relationship: "father", fullName: "Mateo Contreras", phone: "", email: "mateo@example.com", hasAccount: true },
      invitation: { url: "", expiresAt: "", sent: true, via: "provider" },
    });
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Mateo Contreras");

    await user.click(screen.getByRole("button", { name: /Dar acceso a la app/ }));

    expect(await screen.findByText(/spam/i)).toBeInTheDocument();
  });

  // When the message did not leave, the office must get the link so it can hand
  // access over by other means — and must NOT be told the mail went out.
  it("shows the link to copy when the message never left", async () => {
    const user = userEvent.setup();
    students.listContacts.mockResolvedValue([
      { id: "gl-2", studentId: "st-1", guardianUserId: "", relationship: "father", fullName: "Mateo Contreras", phone: "", email: "mateo@example.com", hasAccount: false },
    ]);
    students.inviteContactToApp.mockResolvedValue({
      contact: { id: "gl-2", studentId: "st-1", guardianUserId: "usr-9", relationship: "father", fullName: "Mateo Contreras", phone: "", email: "mateo@example.com", hasAccount: true },
      invitation: { url: "https://escuela/invite/abc", expiresAt: "", sent: false, sendError: "smtp: sin credencial", via: "" },
    });
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Mateo Contreras");

    await user.click(screen.getByRole("button", { name: /Dar acceso a la app/ }));

    const link = await screen.findByDisplayValue("https://escuela/invite/abc");
    expect(link).toBeInTheDocument();
    expect(screen.getByText(/no se pudo enviar/i)).toBeInTheDocument();
  });

  // A parent with two children at the school signs in once. Saying "invitación
  // enviada" about a message nobody sent would have the office waiting for it.
  it("says the account already existed when nothing was sent", async () => {
    const user = userEvent.setup();
    students.listContacts.mockResolvedValue([
      { id: "gl-2", studentId: "st-1", guardianUserId: "", relationship: "mother", fullName: "Lucía Herrera", phone: "", email: "lucia@example.com", hasAccount: false },
    ]);
    students.inviteContactToApp.mockResolvedValue({
      contact: { id: "gl-2", studentId: "st-1", guardianUserId: "usr-old", relationship: "mother", fullName: "Lucía Herrera", phone: "", email: "lucia@example.com", hasAccount: true },
      invitation: null,
    });
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Lucía Herrera");

    await user.click(screen.getByRole("button", { name: /Dar acceso a la app/ }));

    expect(await screen.findByText(/ya tenía cuenta/i)).toBeInTheDocument();
  });

  // Regression: ISSUE-001 — an EmailField is type="text" (to keep the caret), so
  // the browser no longer blocks a malformed address; the server's refusal came
  // back in English. The panel checks the shape itself, in Spanish, on the field.
  // Found by /qa on 2026-09-24
  it("marca un correo mal escrito en el campo y no lo manda", async () => {
    const user = userEvent.setup();
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Ana Cruz");

    await user.type(screen.getByLabelText("Nombre del contacto"), "Luis Cruz");
    await user.type(screen.getByLabelText("Correo del contacto"), "luis@gmail");
    await user.click(screen.getByRole("button", { name: /Agregar/ }));

    const correo = screen.getByLabelText("Correo del contacto");
    expect(correo).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText(/nombre@dominio/)).toBeInTheDocument();
    expect(students.addContact).not.toHaveBeenCalled();
  });
});

describe("ContactsPanel · editing (T5)", () => {
  const rosa = { id: "gl-2", studentId: "st-1", guardianUserId: "", relationship: "other" as const, fullName: "Rosa Méndez", phone: "5551234567", email: "", hasAccount: false };

  it("marks whom to call first", async () => {
    students.listContacts.mockResolvedValue([{ ...rosa, isEmergency: true }]);
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Rosa Méndez");

    expect(screen.getByText("Emergencias")).toBeInTheDocument();
  });

  it("edits a contact in its own row and marks it for emergencies", async () => {
    const user = userEvent.setup();
    students.updateContact.mockResolvedValue({ ...rosa, phone: "5550001111", isEmergency: true });
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Rosa Méndez");

    await user.click(screen.getByRole("button", { name: "Editar a Rosa Méndez" }));
    const phone = screen.getByLabelText("Teléfono de Rosa Méndez");
    await user.clear(phone);
    await user.type(phone, "5550001111");
    await user.click(screen.getByLabelText("Contacto de emergencia de Rosa Méndez"));
    await user.click(screen.getByRole("button", { name: "Guardar contacto" }));

    await waitFor(() => expect(students.updateContact).toHaveBeenCalled());
    expect(students.updateContact.mock.calls[0]?.slice(1)).toEqual([
      "st-1",
      "gl-2",
      { relationship: "other", fullName: "Rosa Méndez", phone: "5550001111", email: "", isEmergency: true },
    ]);
  });

  it("refuses a phone that is not ten digits, on the field", async () => {
    const user = userEvent.setup();
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Ana Cruz");

    await user.type(screen.getByLabelText("Nombre del contacto"), "Luis Cruz");
    await user.type(screen.getByLabelText("Teléfono del contacto"), "555123");
    await user.click(screen.getByRole("button", { name: /Agregar/ }));

    expect(students.addContact).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Teléfono del contacto")).toHaveAttribute("aria-invalid", "true");
  });

  it("sends the emergency mark when adding", async () => {
    const user = userEvent.setup();
    students.addContact.mockResolvedValue({ ...rosa, id: "gl-9", isEmergency: true });
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Ana Cruz");

    await user.type(screen.getByLabelText("Nombre del contacto"), "Luis Cruz");
    await user.click(screen.getByLabelText("Contacto de emergencia"));
    await user.click(screen.getByRole("button", { name: /Agregar/ }));

    await waitFor(() => expect(students.addContact).toHaveBeenCalled());
    expect(students.addContact.mock.calls[0]?.[1]?.isEmergency).toBe(true);
  });

  // Reversible: the row goes at once, the server hears about it only when the
  // eight seconds run out (or the page is left).
  it("removes with an undo, and only then tells the server", async () => {
    const user = userEvent.setup();
    students.removeContact.mockResolvedValue(undefined);
    const { unmount } = render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Rosa Méndez");

    await user.click(screen.getByRole("button", { name: "Quitar a Rosa Méndez" }));
    expect(screen.queryByText("Rosa Méndez")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Deshacer/ }));
    expect(screen.getByText("Rosa Méndez")).toBeInTheDocument();
    expect(students.removeContact).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Quitar a Rosa Méndez" }));
    unmount();
    expect(students.removeContact).toHaveBeenCalledWith(expect.any(String), "st-1", "gl-2");
  });

  // The server refuses it (409): revoke the access first. A button that
  // cannot succeed is not offered.
  it("does not offer to remove a contact who can open the app", async () => {
    render(<ContactsPanel studentId="st-1" />);
    await screen.findByText("Ana Cruz");

    expect(screen.queryByRole("button", { name: "Quitar a Ana Cruz" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Editar a Ana Cruz" })).toBeInTheDocument();
  });

  // LinkGuardian creates contacts that carry only an account; the server lets
  // them go without a name. Editing one must not demand a name first.
  it("edits a linked account that has no name", async () => {
    const user = userEvent.setup();
    const linked = { id: "gl-7", studentId: "st-1", guardianUserId: "usr-7", relationship: "father" as const, fullName: "", phone: "", email: "", hasAccount: true };
    students.listContacts.mockResolvedValue([linked]);
    students.updateContact.mockResolvedValue({ ...linked, isEmergency: true });
    render(<ContactsPanel studentId="st-1" />);

    await user.click(await screen.findByRole("button", { name: "Editar a Cuenta vinculada" }));
    await user.click(screen.getByLabelText("Contacto de emergencia de Cuenta vinculada"));
    await user.click(screen.getByRole("button", { name: "Guardar contacto" }));

    await waitFor(() => expect(students.updateContact).toHaveBeenCalled());
    expect(students.updateContact.mock.calls[0]?.[3]).toMatchObject({ fullName: "", isEmergency: true });
  });
});

// ISSUE-T14-009: a withdrawn pupil's file is read-only; the office still
// needs to read who to call.
describe("ContactsPanel · sólo lectura", () => {
  it("lista los contactos sin ofrecer agregar, editar, quitar ni dar acceso", async () => {
    students.listContacts.mockResolvedValue([
      { id: "gl-2", studentId: "st-1", guardianUserId: "", relationship: "other", fullName: "Rosa Méndez", phone: "5551234567", email: "rosa@example.com", hasAccount: false },
    ]);
    render(<ContactsPanel studentId="st-1" readOnly />);

    expect(await screen.findByText("Rosa Méndez")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Agregar/ })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Nombre del contacto")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Editar a/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Quitar a/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Dar acceso/ })).not.toBeInTheDocument();
  });
});
