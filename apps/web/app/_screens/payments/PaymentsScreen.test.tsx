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
  getCashUp: vi.fn(),
}));
vi.mock("../../students/api", () => ({ listStudents: vi.fn() }));

const finance = vi.mocked(financeApi);
const students = vi.mocked(studentsApi);

const student = (id: string, first: string, last: string): studentsApi.Student => ({
  id, curp: `C${id}`, firstName: first, lastName: last, level: "primaria", levelId: "",
  gradeGroup: "1° A", guardianName: "Tutor", admissionStatus: "enrolled",
});

beforeEach(() => {
  students.listStudents.mockResolvedValue([student("s1", "Valentina", "Cruz"), student("s2", "Mateo", "Sánchez")]);
  finance.getExpected.mockResolvedValue({ amountCents: 670000, surchargeCents: 0, discountCents: 0, annualBalanceCents: 1300000, lines: [] });
  finance.getAccount.mockResolvedValue({ charges: [], payments: [], totalBalanceCents: 1300000, unallocatedCents: 0, dueTodayCents: 0, surchargeCents: 0, earlyDiscountCents: 0, asOf: "" });
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

    expect(await screen.findByRole("dialog", { name: /^Cobrar/ })).toBeInTheDocument();
  });

  // THE BUG THIS EXISTS TO CLOSE.
  //
  // The pupil list was loaded with .catch(() => {}) and the screen then said
  // "Ningún alumno con ese nombre" for every search. When the students service
  // was down, the counter told the office that a pupil who exists does not —
  // which is the worst answer available: it is confident and wrong, and sends
  // somebody looking for a registration problem that is not there.
  it("says the list could not be loaded, instead of claiming the pupil does not exist", async () => {
    const user = userEvent.setup();
    students.listStudents.mockRejectedValue(new Error("students is down"));
    render(<PaymentsScreen onBack={() => {}} />);

    await user.type(await screen.findByLabelText("Buscar alumno"), "Valen");

    expect(await screen.findByRole("alert")).toHaveTextContent(/no pudimos cargar/i);
    expect(screen.queryByText(/Ningún alumno con ese nombre/)).not.toBeInTheDocument();
  });

  // And it can be tried again without reloading the page: the service being
  // back is the common case, and a dead end at the counter is not acceptable.
  it("can retry the list", async () => {
    const user = userEvent.setup();
    students.listStudents.mockRejectedValueOnce(new Error("students is down"));
    render(<PaymentsScreen onBack={() => {}} />);
    await screen.findByRole("alert");

    students.listStudents.mockResolvedValue([student("s1", "Valentina", "Cruz")]);
    await user.click(screen.getByRole("button", { name: /Reintentar/ }));

    await user.type(screen.getByLabelText("Buscar alumno"), "Valen");
    expect(await screen.findByText("Valentina Cruz")).toBeInTheDocument();
  });

  // What each pupil owes is asked ONCE. The first version listed `owed` among
  // its own effect dependencies and then wrote to it, so every answer threw the
  // other requests away and started them again — a dozen names became dozens of
  // round trips.
  it("asks what each pupil owes once", async () => {
    const user = userEvent.setup();
    students.listStudents.mockResolvedValue([
      student("s1", "Valentina", "Cruz"),
      student("s2", "Valeria", "Cruz"),
      student("s3", "Valentín", "Cruz"),
    ]);
    render(<PaymentsScreen onBack={() => {}} />);
    await screen.findByLabelText("Buscar alumno");

    await user.type(screen.getByLabelText("Buscar alumno"), "Cruz");
    await waitFor(() => expect(finance.getExpected).toHaveBeenCalledTimes(3));

    expect(finance.getExpected).toHaveBeenCalledTimes(3);
  });
});

// Task 9: the cashier closes the day from the counter. The cut lived only in
// Finanzas, which the Cobranza role does not open.
describe("PaymentsScreen · corte de caja", () => {
  it("opens the day's cut from the counter", async () => {
    const user = userEvent.setup();
    finance.getCashUp.mockResolvedValue({
      deposits: [], cancelled: [], cashCents: 320000, cardCents: 0, transferCents: 0, totalCents: 320000,
    });
    render(<PaymentsScreen onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: /Corte de caja/ }));
    expect(await screen.findByTestId("cash-up-total")).toHaveTextContent("3,200");
    expect(screen.getByLabelText("Día del corte")).toBeInTheDocument();
  });
});
