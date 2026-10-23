import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as familiesApi from "../../families/api";
import type { FamiliesAccess } from "../../families/model";
import { FamilyCard, type FamilyCardProps } from "./FamilyCard";

vi.mock("../../families/api", async (importOriginal) => ({
  ...(await importOriginal<typeof familiesApi>()),
  getFamily: vi.fn(),
  listFamilies: vi.fn(),
  addStudentsToFamily: vi.fn(),
  createFamily: vi.fn(),
}));
const api = vi.mocked(familiesApi);

const OWNER: FamiliesAccess = { canReadGuardians: true, canWriteGuardians: true, canUpdateStudents: true };
const READER: FamiliesAccess = { canReadGuardians: false, canWriteGuardians: false, canUpdateStudents: true };
const member = (id: string, firstName: string): familiesApi.FamilyMember => ({
  id, firstName, lastName: "GARCÍA", maternalLastName: "LÓPEZ", level: "PRIMARIA", gradeGroup: "A", admissionStatus: "enrolled", draft: false,
});
const DETAIL: familiesApi.FamilyDetail = { id: "f1", name: "GARCÍA LÓPEZ", studentIds: ["s1", "s2"], createdAt: "", members: [member("s1", "ANA"), member("s2", "LUIS")] };

function renderCard(over: Partial<FamilyCardProps> = {}): FamilyCardProps {
  const props: FamilyCardProps = { studentId: "s1", familyId: "f1", access: OWNER, onOpenFamily: vi.fn(), onOpenStudent: vi.fn(), ...over };
  render(<FamilyCard {...props} />);
  return props;
}

beforeEach(() => {
  api.getFamily.mockResolvedValue(DETAIL);
  api.listFamilies.mockResolvedValue([{ id: "f7", name: "SOTO MORA", studentIds: ["s7"], createdAt: "" }]);
  api.addStudentsToFamily.mockResolvedValue({ id: "f7", name: "SOTO MORA", studentIds: ["s7", "s1"], createdAt: "" });
  api.createFamily.mockResolvedValue({ id: "f9", name: "GARCÍA LÓPEZ", studentIds: ["s1"], createdAt: "" });
});
afterEach(() => vi.clearAllMocks());

describe("FamilyCard", () => {
  it("names the family and the siblings, each a way to their file", async () => {
    const user = userEvent.setup();
    const props = renderCard();
    expect(await screen.findByText("GARCÍA LÓPEZ")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "GARCÍA LÓPEZ ANA" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "GARCÍA LÓPEZ LUIS" }));
    expect(props.onOpenStudent).toHaveBeenCalledWith("s2");
    await user.click(screen.getByRole("button", { name: "Ver familia" }));
    expect(props.onOpenFamily).toHaveBeenCalledWith("f1");
  });

  it("says when the pupil is the only child of the family", async () => {
    api.getFamily.mockResolvedValue({ ...DETAIL, studentIds: ["s1"], members: [member("s1", "ANA")] });
    renderCard();
    expect(await screen.findByText("Sin hermanos en la escuela.")).toBeInTheDocument();
  });

  it("adds a pupil without a family to an existing one", async () => {
    const user = userEvent.setup();
    api.getFamily.mockResolvedValue({ id: "f7", name: "SOTO MORA", studentIds: ["s7", "s1"], createdAt: "", members: [member("s7", "IAN"), member("s1", "ANA")] });
    renderCard({ familyId: undefined });
    await user.click(screen.getByRole("button", { name: "Agregar a una familia" }));
    await user.type(await screen.findByRole("searchbox", { name: "Buscar familia" }), "soto");
    await user.click(screen.getByRole("button", { name: "Agregar a la familia SOTO MORA" }));
    await waitFor(() => expect(api.addStudentsToFamily).toHaveBeenCalledWith(expect.any(String), "f7", ["s1"]));
    expect(await screen.findByText("SOTO MORA")).toBeInTheDocument();
    expect(api.getFamily).toHaveBeenCalledWith(expect.any(String), "f7");
  });

  it("or starts a new family with this pupil", async () => {
    const user = userEvent.setup();
    renderCard({ familyId: undefined });
    await user.click(screen.getByRole("button", { name: "Agregar a una familia" }));
    await user.click(await screen.findByRole("button", { name: "Crear una familia nueva con este alumno" }));
    await waitFor(() => expect(api.createFamily).toHaveBeenCalledWith(expect.any(String), { name: "", studentIds: ["s1"] }));
    await waitFor(() => expect(api.getFamily).toHaveBeenCalledWith(expect.any(String), "f9"));
  });

  it("says why the pupil could not join", async () => {
    const user = userEvent.setup();
    api.addStudentsToFamily.mockRejectedValue(new familiesApi.FamiliesError(409, "x", "FAMILY_ACCOUNT_LIMIT"));
    renderCard({ familyId: undefined });
    await user.click(screen.getByRole("button", { name: "Agregar a una familia" }));
    await user.type(await screen.findByRole("searchbox", { name: "Buscar familia" }), "soto");
    await user.click(screen.getByRole("button", { name: "Agregar a la familia SOTO MORA" }));
    expect(await screen.findByText("Esta familia ya tiene 2 cuentas de la app")).toBeInTheDocument();
  });

  it("without guardians:write a pupil without a family is only said so", () => {
    renderCard({ familyId: undefined, access: READER });
    expect(screen.getByText("Sin familia.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Agregar a una familia" })).toBeNull();
    expect(api.listFamilies).not.toHaveBeenCalled();
  });
});
