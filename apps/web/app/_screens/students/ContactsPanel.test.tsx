import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import { ContactsPanel } from "./ContactsPanel";

vi.mock("../../students/api", () => ({ listContacts: vi.fn(), addContact: vi.fn() }));
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
