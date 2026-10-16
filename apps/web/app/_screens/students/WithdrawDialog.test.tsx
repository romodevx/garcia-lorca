import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { WithdrawDialog } from "./WithdrawDialog";
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

describe("WithdrawDialog", () => {
  it("uses the shared date field and drawer width token", async () => {
    render(<WithdrawDialog enrollment={enrollment} onClose={vi.fn()} onSuccess={vi.fn()} />);

    expect(await screen.findByLabelText("Fecha de baja")).toHaveAttribute("type", "date");
    expect(screen.getByRole("dialog", { name: "Dar de baja" })).toHaveClass("drawer-sm");
  });
});
