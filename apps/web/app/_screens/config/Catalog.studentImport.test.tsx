import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../config/api";
import * as financeApi from "../../finance/api";
import { Catalog } from "./Catalog";

vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
  listSubjects: vi.fn(),
  listGroups: vi.fn(),
  listTerms: vi.fn(),
  listNonWorkingPeriods: vi.fn(),
  listSchoolEvents: vi.fn(),
  getSchoolProfile: vi.fn(),
  ConfigError: class ConfigError extends Error {},
}));
vi.mock("../../finance/api", () => ({ listTuitionPlans: vi.fn() }));

const mocked = vi.mocked(api);

beforeEach(() => {
  mocked.listCycles.mockResolvedValue([]);
  mocked.listLevels.mockResolvedValue([]);
  mocked.listGradeLevels.mockResolvedValue([]);
  mocked.listSubjects.mockResolvedValue([]);
  mocked.listGroups.mockResolvedValue([]);
  mocked.listTerms.mockResolvedValue([]);
  mocked.listNonWorkingPeriods.mockResolvedValue([]);
  mocked.listSchoolEvents.mockResolvedValue([]);
  mocked.getSchoolProfile.mockResolvedValue({ profile: { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "" }, found: false });
  vi.mocked(financeApi).listTuitionPlans.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

describe("the way into Migrar alumnos (ADR 0033)", () => {
  it("offers a header button and a card under the cycle when the caller may import", async () => {
    const onOpenStudentImport = vi.fn();
    const user = userEvent.setup();
    render(<Catalog onBack={() => {}} onOpenStudentImport={onOpenStudentImport} />);

    const card = await screen.findByRole("region", { name: "Migrar alumnos del sistema anterior" });
    await user.click(within(card).getByRole("button", { name: "Migrar alumnos" }));
    const header = screen.getAllByRole("button", { name: "Migrar alumnos" }).find((b) => !card.contains(b));
    expect(header).toBeDefined();
    if (header !== undefined) await user.click(header);

    expect(onOpenStudentImport).toHaveBeenCalledTimes(2);
  });

  it("shows neither when the screen was not given the way in", async () => {
    render(<Catalog onBack={() => {}} />);
    await screen.findByText("Ciclo escolar");
    expect(screen.queryByRole("button", { name: "Migrar alumnos" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Migrar alumnos del sistema anterior" })).not.toBeInTheDocument();
  });
});
