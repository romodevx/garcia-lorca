import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement, ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../../config/api";
import type { GradeLevel, Level } from "../../../config/api";
import * as financeApi from "../../../finance/api";
import { CalendarPanel } from "../CalendarPanel";
import { CopyMonthsInline } from "../CopyMonthsInline";
import { GradeLevelsPanel } from "../GradeLevelsPanel";
import { LevelsPanel } from "../LevelsPanel";
import { Section } from "../parts";
import { LevelPeriodsEntry } from "../periods/LevelPeriodsEntry";
import { SubjectsPanel } from "../SubjectsPanel";
import { TuitionPanel } from "../TuitionPanel";
import { SetupPresentationProvider, useSetupPresentation } from "./SetupPresentation";

vi.mock("../../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../../config/api")>()),
  listSubjects: vi.fn(),
  listNonWorkingPeriods: vi.fn(),
  listSchoolEvents: vi.fn(),
}));
vi.mock("../../../finance/api", () => ({
  listTuitionPlans: vi.fn(),
  upsertTuitionPlan: vi.fn(),
  runBilling: vi.fn(),
}));
const config = vi.mocked(configApi);
const finance = vi.mocked(financeApi);

beforeEach(() => {
  config.listSubjects.mockResolvedValue([]);
  config.listNonWorkingPeriods.mockResolvedValue([]);
  config.listSchoolEvents.mockResolvedValue([]);
  finance.listTuitionPlans.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

const CYCLE = { id: "c1", name: "2026-2027", status: "active" as const, startsOn: "2026-08-01", endsOn: "2027-06-30" };
const PRIMARIA: Level = { id: "l1", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle" };
const BACHILLERATO: Level = { id: "l2", name: "BACHILLERATO", sepCode: "", enrollmentGranularity: "term" };
const GRADES: GradeLevel[] = [
  { id: "g1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 },
  { id: "g2", levelId: "l1", name: "2° PRIMARIA", ordinal: 2 },
];

const inSetup = (ui: ReactNode): ReturnType<typeof render> => render(<SetupPresentationProvider>{ui}</SetupPresentationProvider>);

/** A Btn's look is its inline background: lime is the only `var(--primary)` one. */
const isLime = (el: HTMLElement): boolean => el.style.background === "var(--primary)";

const noop = async (): Promise<void> => {};

function Probe(): ReactElement {
  return <span>{useSetupPresentation().inSetup ? "setup" : "classic"}</span>;
}

describe("useSetupPresentation", () => {
  it("is the old view without a provider, and the setup view inside one", () => {
    const { unmount } = render(<Probe />);
    expect(screen.getByText("classic")).toBeInTheDocument();
    unmount();

    inSetup(<Probe />);
    expect(screen.getByText("setup")).toBeInTheDocument();
  });
});

describe("Section", () => {
  it("keeps its card in the old view", () => {
    render(<Section title="Grupos">x</Section>);
    const section = screen.getByRole("heading", { level: 2, name: "Grupos" }).closest("section");
    expect(section?.style.border).not.toBe("");
    expect(section?.style.boxShadow).not.toBe("");
  });

  it("drops the card inside a setup step, and keeps its h2 title", () => {
    inSetup(<Section title="Grupos" subtitle="De 1°">x</Section>);
    const section = screen.getByRole("heading", { level: 2, name: "Grupos" }).closest("section");
    expect(section?.style.border).toBe("");
    expect(section?.style.boxShadow).toBe("");
    expect(section?.style.background).toBe("");
    expect(screen.getByText("De 1°")).toBeInTheDocument();
  });
});

describe("LevelsPanel in a setup step", () => {
  const props = {
    levels: [PRIMARIA, BACHILLERATO],
    onSelect: () => {},
    onCreate: noop,
    onSetAttendanceMode: noop,
    onSetGrading: noop,
  };

  it("names a level chip without '· por ciclo', and keeps '· por periodo' where it applies (#12)", () => {
    inSetup(<LevelsPanel {...props} selectedLevelId={undefined} />);
    expect(screen.getByRole("button", { name: "PRIMARIA" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "BACHILLERATO · por periodo" })).toBeInTheDocument();
  });

  it("keeps Nuevo nivel lime and steps Guardar calificación down to outline (§1.3)", () => {
    inSetup(<LevelsPanel {...props} selectedLevelId="l1" />);
    expect(isLime(screen.getByRole("button", { name: "Nuevo nivel" }))).toBe(true);
    expect(isLime(screen.getByRole("button", { name: "Guardar calificación" }))).toBe(false);
  });

  it("shows Eliminar nivel as red text at the foot of the level's settings (#10)", async () => {
    const onDelete = vi.fn();
    const user = userEvent.setup();
    inSetup(<LevelsPanel {...props} selectedLevelId="l1" onDelete={onDelete} />);

    const remove = screen.getByRole("button", { name: "Eliminar nivel PRIMARIA" });
    expect(remove.style.border).not.toContain("solid");
    expect(remove.style.color).toBe("var(--bad-fg)");
    await user.click(remove);
    expect(onDelete).toHaveBeenCalledWith("l1");
  });

  it("keeps today's chips and outlined Eliminar nivel without the provider", () => {
    render(<LevelsPanel {...props} selectedLevelId="l1" onDelete={() => {}} />);
    expect(screen.getByRole("button", { name: "PRIMARIA · por ciclo" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Eliminar nivel PRIMARIA" }).style.border).toContain("solid");
    expect(isLime(screen.getByRole("button", { name: "Guardar calificación" }))).toBe(true);
  });
});

describe("GradeLevelsPanel in a setup step", () => {
  it("names a grade chip '1° PRIMARIA', not '1. 1° PRIMARIA' (#12)", () => {
    inSetup(<GradeLevelsPanel level={PRIMARIA} gradeLevels={GRADES} selectedGradeLevelId={undefined} onSelect={() => {}} onCreate={noop} />);
    expect(screen.getByRole("button", { name: "1° PRIMARIA" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "1. 1° PRIMARIA" })).toBeNull();
  });
});

describe("Periodos (step 4) in a setup step", () => {
  it("steps Periodos por grado down to outline (§1.3)", () => {
    inSetup(<LevelPeriodsEntry level={PRIMARIA} onOpen={() => {}} />);
    expect(isLime(screen.getByRole("button", { name: "Periodos por grado de PRIMARIA" }))).toBe(false);
  });

  it("steps Copiar meses down to outline (§1.3)", async () => {
    const user = userEvent.setup();
    inSetup(<CopyMonthsInline targets={[BACHILLERATO]} onCopy={noop} />);
    await user.click(screen.getByRole("button", { name: "Copiar a otros niveles" }));
    expect(isLime(screen.getByRole("button", { name: "Copiar meses" }))).toBe(false);
  });

  it("keeps both lime without the provider", async () => {
    const user = userEvent.setup();
    render(
      <>
        <LevelPeriodsEntry level={PRIMARIA} onOpen={() => {}} />
        <CopyMonthsInline targets={[BACHILLERATO]} onCopy={noop} />
      </>,
    );
    await user.click(screen.getByRole("button", { name: "Copiar a otros niveles" }));
    expect(isLime(screen.getByRole("button", { name: "Periodos por grado de PRIMARIA" }))).toBe(true);
    expect(isLime(screen.getByRole("button", { name: "Copiar meses" }))).toBe(true);
  });
});

describe("SubjectsPanel in a setup step", () => {
  it("labels the grade picker 'En qué grados' and keeps it (A49)", async () => {
    inSetup(<SubjectsPanel level={PRIMARIA} gradeLevels={GRADES} />);
    const picker = await screen.findByRole("group", { name: "En qué grados" });
    expect(picker).toHaveTextContent("Todos los grados");
  });

  it("keeps 'Grados' without the provider", async () => {
    render(<SubjectsPanel level={PRIMARIA} gradeLevels={GRADES} />);
    expect(await screen.findByRole("group", { name: "Grados" })).toBeInTheDocument();
  });
});

describe("TuitionPanel in a setup step", () => {
  it("steps Generar cargos del mes down to outline (§1.3)", async () => {
    inSetup(<TuitionPanel cycle={CYCLE} levels={[PRIMARIA]} />);
    await waitFor(() => expect(finance.listTuitionPlans).toHaveBeenCalled());
    expect(isLime(screen.getByRole("button", { name: "Generar cargos del mes" }))).toBe(false);
  });

  it("keeps it lime without the provider", async () => {
    render(<TuitionPanel cycle={CYCLE} levels={[PRIMARIA]} />);
    await waitFor(() => expect(finance.listTuitionPlans).toHaveBeenCalled());
    expect(isLime(screen.getByRole("button", { name: "Generar cargos del mes" }))).toBe(true);
  });
});

describe("CalendarPanel in a setup step", () => {
  it("makes Actividades an h3 under the panel's h2 (#14)", async () => {
    inSetup(<CalendarPanel cycle={CYCLE} />);
    expect(screen.getByRole("heading", { level: 2, name: "Calendario escolar" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 3, name: "Actividades" })).toBeInTheDocument();
  });

  it("keeps Actividades as plain text without the provider", async () => {
    render(<CalendarPanel cycle={CYCLE} />);
    expect(await screen.findByText("Actividades")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Actividades" })).toBeNull();
  });
});
