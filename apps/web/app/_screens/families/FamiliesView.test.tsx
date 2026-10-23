import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as familiesApi from "../../families/api";
import type { FamiliesAccess } from "../../families/model";
import { FamiliesView, type FamiliesViewProps } from "./FamiliesView";
import type { FamilyPupil } from "./familyPupil";

vi.mock("../../families/api", async (importOriginal) => ({
  ...(await importOriginal<typeof familiesApi>()),
  listFamilySuggestions: vi.fn(),
  createFamily: vi.fn(),
  dismissFamilySuggestion: vi.fn(),
}));
const api = vi.mocked(familiesApi);

const pupil = (id: string, lastName: string, maternalLastName: string, firstName: string, room: string): FamilyPupil => ({
  id, firstName, lastName, maternalLastName, name: `${lastName} ${maternalLastName} ${firstName}`, room, admissionStatus: "enrolled",
});
const PUPILS: FamilyPupil[] = [
  pupil("s1", "GARCÍA", "LÓPEZ", "ANA", "3° PRIMARIA · A"),
  pupil("s2", "GARCÍA", "LÓPEZ", "LUIS", "1° PRIMARIA · B"),
  pupil("s3", "PÉREZ", "RUIZ", "EVA", "2° PREESCOLAR · A"),
  pupil("s4", "PÉREZ", "RUIZ", "TEO", "1° PREESCOLAR · A"),
  pupil("s5", "SOTO", "MORA", "IAN", "4° PRIMARIA · A"),
];
const OWNER: FamiliesAccess = { canReadGuardians: true, canWriteGuardians: true, canUpdateStudents: true };
const CLERK: FamiliesAccess = { canReadGuardians: false, canWriteGuardians: false, canUpdateStudents: true };

const HIGH: familiesApi.FamilySuggestion = { signature: "sig-1", proposedName: "PÉREZ RUIZ", studentIds: ["s3", "s4"], reasons: ["same_surnames", "shared_contact_email"], confidence: "high" };
const MEDIUM: familiesApi.FamilySuggestion = { signature: "sig-2", proposedName: "GARCÍA LÓPEZ", studentIds: ["s1", "s2"], reasons: ["same_surnames"], confidence: "medium" };
const FAMILY: familiesApi.Family = { id: "f1", name: "SOTO MORA", studentIds: ["s5"], accountCount: 1, createdAt: "" };

function renderView(overrides: Partial<FamiliesViewProps> = {}): FamiliesViewProps {
  const props: FamiliesViewProps = {
    access: OWNER,
    families: [FAMILY],
    familiesFailed: false,
    pupils: PUPILS,
    onOpenFamily: vi.fn(),
    onFamiliesChanged: vi.fn(),
    onShowWithoutFamily: vi.fn(),
    say: vi.fn(),
    ...overrides,
  };
  render(<FamiliesView {...props} />);
  return props;
}

const suggestionCard = (name: string): HTMLElement => screen.getByRole("article", { name: `Sugerencia ${name}` });

beforeEach(() => {
  api.listFamilySuggestions.mockResolvedValue([HIGH, MEDIUM]);
  api.createFamily.mockImplementation(async (_b, input) => ({ id: "new", name: input.name, studentIds: [...input.studentIds], createdAt: "" }));
  api.dismissFamilySuggestion.mockResolvedValue();
});
afterEach(() => vi.clearAllMocks());

describe("FamiliesView — numbers", () => {
  it("counts families, suggestions, families with the app and pupils without a family", async () => {
    renderView();
    await screen.findByRole("article", { name: "Sugerencia PÉREZ RUIZ" });
    expect(screen.getByTestId("families-stat-total")).toHaveTextContent("1");
    expect(screen.getByTestId("families-stat-suggestions")).toHaveTextContent("2");
    expect(screen.getByTestId("families-stat-app")).toHaveTextContent("1");
    expect(screen.getByTestId("families-stat-orphans")).toHaveTextContent("4");
  });

  it("hides the app count from a caller without guardians:read", async () => {
    renderView({ access: CLERK, families: [{ ...FAMILY, accountCount: undefined }] });
    await screen.findByRole("article", { name: "Sugerencia PÉREZ RUIZ" });
    expect(screen.queryByTestId("families-stat-app")).toBeNull();
    expect(screen.queryByText("1 de 2")).toBeNull();
  });

  it("'Alumnos sin familia' takes the office to those pupils", async () => {
    const user = userEvent.setup();
    const props = renderView();
    await user.click(screen.getByTestId("families-stat-orphans"));
    expect(props.onShowWithoutFamily).toHaveBeenCalled();
  });
});

describe("FamiliesView — suggestions", () => {
  it("shows each suggestion's pupils, reasons and confidence", async () => {
    renderView();
    const card = await screen.findByRole("article", { name: "Sugerencia PÉREZ RUIZ" });
    expect(within(card).getByText("PÉREZ RUIZ EVA")).toBeInTheDocument();
    expect(within(card).getByText("2° PREESCOLAR · A")).toBeInTheDocument();
    expect(within(card).getByText("Mismos apellidos")).toBeInTheDocument();
    expect(within(card).getByText("Mismo correo")).toBeInTheDocument();
    expect(within(card).getByText("Muy probable")).toBeInTheDocument();
    expect(within(suggestionCard("GARCÍA LÓPEZ")).getByText("Posible")).toBeInTheDocument();
  });

  it("'Es familia' builds it in one click", async () => {
    const user = userEvent.setup();
    const props = renderView();
    const card = await screen.findByRole("article", { name: "Sugerencia PÉREZ RUIZ" });
    await user.click(within(card).getByRole("button", { name: "Es familia" }));
    await waitFor(() => expect(api.createFamily).toHaveBeenCalledWith(expect.any(String), { name: "PÉREZ RUIZ", studentIds: ["s3", "s4"] }));
    expect(props.say).toHaveBeenCalledWith("Familia PÉREZ RUIZ creada");
    expect(props.onFamiliesChanged).toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByRole("article", { name: "Sugerencia PÉREZ RUIZ" })).toBeNull());
  });

  it("'No es familia' dismisses it", async () => {
    const user = userEvent.setup();
    const props = renderView();
    const card = await screen.findByRole("article", { name: "Sugerencia GARCÍA LÓPEZ" });
    await user.click(within(card).getByRole("button", { name: "No es familia" }));
    await waitFor(() => expect(api.dismissFamilySuggestion).toHaveBeenCalledWith(expect.any(String), ["s1", "s2"]));
    expect(props.say).toHaveBeenCalledWith("Listo: no se volverá a sugerir");
    await waitFor(() => expect(screen.queryByRole("article", { name: "Sugerencia GARCÍA LÓPEZ" })).toBeNull());
  });

  it("accepts every 'muy probable' at once", async () => {
    const user = userEvent.setup();
    const props = renderView();
    await user.click(await screen.findByRole("button", { name: "Aceptar las muy probables (1)" }));
    await waitFor(() => expect(api.createFamily).toHaveBeenCalledTimes(1));
    expect(api.createFamily).toHaveBeenCalledWith(expect.any(String), { name: "PÉREZ RUIZ", studentIds: ["s3", "s4"] });
    expect(props.say).toHaveBeenCalledWith("1 familia creada");
    expect(screen.getByRole("article", { name: "Sugerencia GARCÍA LÓPEZ" })).toBeInTheDocument();
  });

  it("says why a suggestion could not become a family", async () => {
    const user = userEvent.setup();
    api.createFamily.mockRejectedValue(new familiesApi.FamiliesError(409, "x", "FAMILY_ACCOUNT_LIMIT"));
    renderView();
    const card = await screen.findByRole("article", { name: "Sugerencia PÉREZ RUIZ" });
    await user.click(within(card).getByRole("button", { name: "Es familia" }));
    expect(await within(card).findByText("Esta familia ya tiene 2 cuentas de la app")).toBeInTheDocument();
  });

  it("'Ajustar' opens the new family prefilled; a pupil can be left out", async () => {
    const user = userEvent.setup();
    renderView();
    const card = await screen.findByRole("article", { name: "Sugerencia PÉREZ RUIZ" });
    await user.click(within(card).getByRole("button", { name: "Ajustar" }));
    const drawer = await screen.findByRole("dialog", { name: "Nueva familia" });
    expect(within(drawer).getByRole("textbox", { name: /Nombre de la familia/ })).toHaveValue("PÉREZ RUIZ");
    await user.click(within(drawer).getByRole("button", { name: "Quitar a PÉREZ RUIZ TEO" }));
    await user.clear(within(drawer).getByRole("textbox", { name: /Nombre de la familia/ }));
    await user.type(within(drawer).getByRole("textbox", { name: /Nombre de la familia/ }), "LOS PÉREZ");
    await user.click(within(drawer).getByRole("button", { name: "Crear familia" }));
    await waitFor(() => expect(api.createFamily).toHaveBeenCalledWith(expect.any(String), { name: "LOS PÉREZ", studentIds: ["s3"] }));
  });

  it("without guardians:write the suggestions are read-only", async () => {
    renderView({ access: CLERK });
    const card = await screen.findByRole("article", { name: "Sugerencia PÉREZ RUIZ" });
    expect(screen.getByText("Sólo la dirección puede armar familias")).toBeInTheDocument();
    expect(within(card).queryByRole("button", { name: "Es familia" })).toBeNull();
    expect(within(card).queryByRole("button", { name: "Ajustar" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Aceptar las muy probables/ })).toBeNull();
    expect(screen.queryByRole("button", { name: "Nueva familia" })).toBeNull();
    // Dismissing is students:update, which the clerk holds.
    expect(within(card).getByRole("button", { name: "No es familia" })).toBeInTheDocument();
  });

  it("without students:update nobody offers to dismiss", async () => {
    renderView({ access: { ...CLERK, canUpdateStudents: false } });
    const card = await screen.findByRole("article", { name: "Sugerencia PÉREZ RUIZ" });
    expect(within(card).queryByRole("button", { name: "No es familia" })).toBeNull();
  });
});

describe("FamiliesView — the families table", () => {
  it("lists each family with its children and its app accounts", async () => {
    renderView();
    const row = await screen.findByRole("row", { name: /SOTO MORA/ });
    expect(within(row).getByText("SOTO MORA IAN")).toBeInTheDocument();
    expect(within(row).getByText("4° PRIMARIA · A")).toBeInTheDocument();
    expect(within(row).getByText("1 de 2")).toBeInTheDocument();
  });

  it("opens the family from its row", async () => {
    const user = userEvent.setup();
    const props = renderView();
    await user.click(await screen.findByRole("button", { name: "Abrir la familia SOTO MORA" }));
    expect(props.onOpenFamily).toHaveBeenCalledWith("f1");
  });

  it("searches by family or child name", async () => {
    const user = userEvent.setup();
    renderView({ families: [FAMILY, { id: "f2", name: "LUNA", studentIds: ["s1"], createdAt: "" }] });
    await screen.findByRole("row", { name: /SOTO MORA/ });
    await user.type(screen.getByRole("searchbox", { name: "Buscar familia o hijo" }), "ana");
    expect(screen.queryByRole("row", { name: /SOTO MORA/ })).toBeNull();
    expect(screen.getByRole("row", { name: /LUNA/ })).toBeInTheDocument();
  });

  it("says so when the families could not be read", async () => {
    renderView({ families: [], familiesFailed: true });
    expect(await screen.findByText("No pudimos leer las familias.")).toBeInTheDocument();
  });
});

describe("FamiliesView — Nueva familia", () => {
  it("picks pupils without a family; the name follows the first one's surnames", async () => {
    const user = userEvent.setup();
    renderView();
    await user.click(await screen.findByRole("button", { name: "Nueva familia" }));
    const drawer = await screen.findByRole("dialog", { name: "Nueva familia" });
    await user.type(within(drawer).getByRole("searchbox", { name: "Buscar alumno sin familia" }), "garc");
    // SOTO MORA IAN already has a family: never offered.
    expect(within(drawer).queryByRole("button", { name: "Agregar a SOTO MORA IAN" })).toBeNull();
    await user.click(within(drawer).getByRole("button", { name: "Agregar a GARCÍA LÓPEZ LUIS" }));
    expect(within(drawer).getByRole("textbox", { name: /Nombre de la familia/ })).toHaveValue("GARCÍA LÓPEZ");
    await user.click(within(drawer).getByRole("button", { name: "Crear familia" }));
    await waitFor(() => expect(api.createFamily).toHaveBeenCalledWith(expect.any(String), { name: "GARCÍA LÓPEZ", studentIds: ["s2"] }));
  });

  it("asks for a pupil on save instead of a dead button", async () => {
    const user = userEvent.setup();
    renderView();
    await user.click(await screen.findByRole("button", { name: "Nueva familia" }));
    const drawer = await screen.findByRole("dialog", { name: "Nueva familia" });
    const save = within(drawer).getByRole("button", { name: "Crear familia" });
    expect(save).toBeEnabled();
    await user.click(save);
    expect(await within(drawer).findByText("Elige al menos un alumno")).toBeInTheDocument();
    expect(api.createFamily).not.toHaveBeenCalled();
  });
});
