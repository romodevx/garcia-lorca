import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { GradeSheetClient, PeriodSheet, SavedPeriodGrade } from "../../grades/periodSheet";
import { PeriodGradeSheet } from "./PeriodGradeSheet";
import { useGradeSaves } from "./useGradeSaves";

/** Preescolar: grades with letters (owner, 20-oct). */
const SHEET: PeriodSheet = {
  assignmentId: "a1",
  periodId: "p1",
  periods: [{ id: "p1", ordinal: 1, name: "1.er trimestre", startsOn: "2026-08-31", endsOn: "2026-11-30" }],
  currentPeriodId: "p1",
  today: "2026-10-20",
  closedPeriodIds: [],
  scale: { min: 5, max: 10, decimals: 0, passing: 6 },
  gradingEnabled: false,
  gradingLetters: true,
  editable: true,
  rows: [
    { enrollmentId: "en1", studentId: "st1", name: "Ana Ávila", value: null, letter: "A", updatedAt: "2026-10-10T10:00:00Z", rowEditable: true },
    { enrollmentId: "en2", studentId: "st2", name: "Beto Díaz", value: null, updatedAt: null, rowEditable: true },
    { enrollmentId: "en3", studentId: "st3", name: "Carla Ruiz", value: null, letter: "C", updatedAt: "2026-10-10T10:00:00Z", rowEditable: true },
  ],
};

type MockClient = { [K in keyof GradeSheetClient]: ReturnType<typeof vi.fn> & GradeSheetClient[K] };

function fakeClient(sheet: PeriodSheet = SHEET): MockClient {
  return {
    load: vi.fn().mockResolvedValue(sheet),
    save: vi.fn().mockImplementation(
      async (i: { enrollmentId: string; periodId: string; value: string }): Promise<SavedPeriodGrade> => ({
        enrollmentId: i.enrollmentId, periodId: i.periodId, value: null, letter: i.value, updatedAt: "2026-10-20T10:00:00Z",
      }),
    ),
    remove: vi.fn().mockResolvedValue(undefined),
  } as MockClient;
}

function Screen({ client }: { client: GradeSheetClient }) {
  const saves = useGradeSaves(client);
  return <PeriodGradeSheet assignmentId="a1" client={client} saves={saves} />;
}

async function renderSheet(client: MockClient = fakeClient()): Promise<MockClient> {
  render(<Screen client={client} />);
  await screen.findByRole("table", { name: "Calificaciones del periodo" });
  return client;
}

const letters = (name: string): HTMLElement => screen.getByRole("radiogroup", { name: `Calificación de ${name}` });

afterEach(() => vi.clearAllMocks());

describe("PeriodGradeSheet — letters (preescolar)", () => {
  it("offers A–D per pupil with what each means, and shows what is captured", async () => {
    await renderSheet();
    expect(within(letters("Ana Ávila")).getByRole("radio", { name: "A · Excelente" })).toBeChecked();
    expect(within(letters("Beto Díaz")).getByRole("radio", { name: "B · Bien" })).not.toBeChecked();
    expect(within(letters("Carla Ruiz")).getByRole("radio", { name: "C · Suficiente" })).toBeChecked();
    expect(within(letters("Beto Díaz")).getByRole("radio", { name: "D · Requiere apoyo" })).toBeInTheDocument();
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("saves a letter the moment it is chosen", async () => {
    const user = userEvent.setup();
    const client = await renderSheet();
    await user.click(within(letters("Beto Díaz")).getByRole("radio", { name: "B · Bien" }));
    await waitFor(() => expect(client.save).toHaveBeenCalledWith({ assignmentId: "a1", periodId: "p1", enrollmentId: "en2", value: "B" }));
    expect(await within(screen.getByRole("row", { name: /Beto Díaz/ })).findByText("Guardado")).toBeInTheDocument();
  });

  it("clears a letter", async () => {
    const user = userEvent.setup();
    const client = await renderSheet();
    await user.click(screen.getByRole("button", { name: "Quitar la calificación de Ana Ávila" }));
    await waitFor(() => expect(client.remove).toHaveBeenCalledWith({ assignmentId: "a1", periodId: "p1", enrollmentId: "en1" }));
  });

  it("counts each letter instead of an average — letters are never averaged", async () => {
    await renderSheet();
    const summary = screen.getByRole("group", { name: "Resumen del periodo" });
    expect(within(summary).getByText("2 de 3")).toBeInTheDocument();
    expect(within(summary).queryByText("Promedio")).toBeNull();
    expect(within(summary).queryByText("Reprobados")).toBeNull();
    expect(within(summary).getByText("A 1 · B 0 · C 1 · D 0")).toBeInTheDocument();
  });

  it("a closed period shows the letters read-only", async () => {
    await renderSheet(fakeClient({ ...SHEET, editable: false, readOnlyReason: "El periodo está cerrado" }));
    expect(screen.queryByRole("radiogroup")).toBeNull();
    expect(within(screen.getByRole("row", { name: /Ana Ávila/ })).getByText("A · Excelente")).toBeInTheDocument();
  });
});
