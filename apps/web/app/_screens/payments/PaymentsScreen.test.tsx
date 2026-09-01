import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import * as studentsApi from "../../students/api";
import { PaymentsScreen } from "./PaymentsScreen";

vi.mock("../../finance/api", () => ({
  getExpected: vi.fn(),
  getAccount: vi.fn(),
  previewDeposit: vi.fn(),
  registerDeposit: vi.fn(),
}));
vi.mock("../../students/api", () => ({ listStudents: vi.fn() }));

const finance = vi.mocked(financeApi);
const students = vi.mocked(studentsApi);

const student = (id: string, first: string, last: string): studentsApi.Student => ({
  id, curp: `C${id}`, firstName: first, lastName: last, level: "primaria",
  gradeGroup: "1° A", guardianName: "Tutor", admissionStatus: "enrolled",
});

beforeEach(() => {
  students.listStudents.mockResolvedValue([student("s1", "Valentina", "Cruz"), student("s2", "Mateo", "Sánchez")]);
  finance.getExpected.mockResolvedValue({ amountCents: 670000, surchargeCents: 0, discountCents: 0, annualBalanceCents: 1300000 });
  finance.getAccount.mockResolvedValue({ charges: [], payments: [], totalBalanceCents: 1300000, unallocatedCents: 0 });
  finance.previewDeposit.mockResolvedValue({ allocations: [], surcharges: [], unallocatedCents: 0 });
});
afterEach(() => vi.clearAllMocks());

describe("PaymentsScreen", () => {
  // The counter's whole flow is one question: which pupil. Everything else
  // happens in the drawer.
  it("starts on the search, with nothing else in the way", async () => {
    render(<PaymentsScreen onBack={() => {}} />);

    expect(await screen.findByLabelText("Buscar alumno")).toBeInTheDocument();
    expect(screen.getByText(/Escribe el nombre del alumno/)).toBeInTheDocument();
    // No list until somebody types: four hundred names is not a starting point.
    expect(screen.queryByText("Valentina Cruz")).not.toBeInTheDocument();
  });

  it("shows what each match owes today, before opening anything", async () => {
    const user = userEvent.setup();
    render(<PaymentsScreen onBack={() => {}} />);
    await screen.findByLabelText("Buscar alumno");

    await user.type(screen.getByLabelText("Buscar alumno"), "Valen");

    expect(await screen.findByText("Valentina Cruz")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText("$6,700.00")).toBeInTheDocument());
    expect(screen.queryByText("Mateo Sánchez")).not.toBeInTheDocument();
  });

  it("opens the counter on the pupil picked", async () => {
    const user = userEvent.setup();
    render(<PaymentsScreen onBack={() => {}} />);
    await screen.findByLabelText("Buscar alumno");

    await user.type(screen.getByLabelText("Buscar alumno"), "Valen");
    await user.click(await screen.findByRole("button", { name: /Valentina Cruz/ }));

    expect(await screen.findByRole("dialog", { name: "Cobrar" })).toBeInTheDocument();
  });
});
