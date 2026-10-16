import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { WithdrawPanel } from "./WithdrawDialog";
import type { Enrollment } from "../../students/api";

vi.mock("../../students/api", async (load) => ({
  ...(await load<typeof import("../../students/api")>()),
  previewWithdrawal: vi.fn().mockResolvedValue({ toCancel: [], toCancelCents: 0, remainingDebtCents: 0 }),
  withdraw: vi.fn(),
}));

const enrollment: Enrollment = {
  id: "enrollment-1", studentId: "student-1", cycleId: "cycle-1", groupId: "group-1",
  termId: "", status: "active", enrolledAt: "2026-08-01",
};

describe("WithdrawPanel", () => {
  it("renders in the existing sheet, focuses its date, and formats the preview date", async () => {
    vi.mocked((await import("../../students/api")).previewWithdrawal).mockResolvedValue({ toCancel: [{ id: "fee-1", description: "Colegiatura", dueDate: "2026-11-11", amountCents: 12000 }], toCancelCents: 12000, remainingDebtCents: 0 });
    render(<WithdrawPanel enrollment={enrollment} onBack={vi.fn()} onSuccess={vi.fn()} />);

    expect(await screen.findByLabelText("Fecha de baja")).toHaveAttribute("type", "date");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByText(/vence 11 nov 2026/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Dar de baja" })).toHaveStyle({ color: "var(--on-color-ink)" });
    await waitFor(() => expect(screen.getByLabelText("Fecha de baja")).toHaveFocus());
  });

  it("requests that the parent close the panel from the back action", async () => {
    const user = userEvent.setup();
    const onBack = vi.fn();
    render(<WithdrawPanel enrollment={enrollment} onBack={onBack} onSuccess={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "← Volver a la ficha" }));
    expect(onBack).toHaveBeenCalledOnce();
  });

  it("focuses the error summary after a failed save", async () => {
    const user = userEvent.setup();
    vi.mocked((await import("../../students/api")).withdraw).mockRejectedValueOnce(new Error("No se pudo guardar"));
    render(<WithdrawPanel enrollment={enrollment} onBack={vi.fn()} onSuccess={vi.fn()} />);
    await user.click(await screen.findByRole("button", { name: "Dar de baja" }));

    const summary = await screen.findByRole("alert");
    await waitFor(() => expect(summary).toHaveFocus());
  });

  it("clears a preview error when the next date preview succeeds", async () => {
    const user = userEvent.setup();
    const previewWithdrawal = vi.mocked((await import("../../students/api")).previewWithdrawal);
    previewWithdrawal.mockReset();
    previewWithdrawal.mockResolvedValue({ toCancel: [], toCancelCents: 0, remainingDebtCents: 0 });
    previewWithdrawal.mockRejectedValueOnce(new Error("Vista previa no disponible"));
    render(<WithdrawPanel enrollment={enrollment} onBack={vi.fn()} onSuccess={vi.fn()} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Vista previa no disponible");

    const date = screen.getByLabelText("Fecha de baja");
    await user.clear(date);
    await user.type(date, "2026-10-14");
    await waitFor(() => expect(screen.queryByRole("alert")).not.toBeInTheDocument());
    expect(await screen.findByText(/Se cancelarán/)).toBeInTheDocument();
  });
});
