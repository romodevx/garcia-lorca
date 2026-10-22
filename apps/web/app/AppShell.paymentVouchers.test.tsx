import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";
import { Providers, fakeRouter, resetFakeRouter, useFakePathname } from "./test-utils";
import { me as apiMe } from "./auth/api";
import { listPendingVouchers } from "./finance/vouchers";

/*
 * module_payment_vouchers (ADR 0040) is read once here and handed to Registro
 * de pagos; no screen reads the flag itself.
 */

vi.mock("next/navigation", () => ({
  useRouter: () => fakeRouter,
  usePathname: () => useFakePathname(),
}));

vi.mock("./auth/api", () => ({
  login: vi.fn(),
  changePassword: vi.fn(),
  logout: vi.fn(),
  refresh: vi.fn(() => Promise.reject(new Error("no session"))),
  me: vi.fn(),
}));

vi.mock("./students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./students/api")>()),
  listStudentsWithRooms: vi.fn().mockResolvedValue([
    { id: "s1", curp: "CURP1", firstName: "Ana", lastName: "Arce", level: "Primaria", levelId: "", gradeGroup: "A", guardianName: "", admissionStatus: "enrolled" },
  ]),
}));

vi.mock("./finance/vouchers", () => ({
  listPendingVouchers: vi.fn(),
  getVoucherFile: vi.fn(),
  rejectVoucher: vi.fn(),
  VOUCHER_CODES: {},
}));

beforeEach(() => {
  resetFakeRouter("/pagos");
  vi.mocked(apiMe).mockResolvedValue({ userId: "u1", email: "caja@example.com", permissions: ["collections:read", "payments:write"] });
  vi.mocked(listPendingVouchers).mockResolvedValue([
    { id: "v1", studentId: "s1", status: "pending", fileContentType: "image/png", fileSizeBytes: 1, uploadedByName: "Rosa", createdAt: "2026-10-20T18:00:00Z" },
  ]);
});
afterEach(() => vi.clearAllMocks());

describe("Registro de pagos and the module_payment_vouchers flag", () => {
  it("OFF: no queue, and the queue is never asked for", async () => {
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>,
    );
    expect(await screen.findByLabelText("Buscar alumno")).toBeInTheDocument();
    expect(listPendingVouchers).not.toHaveBeenCalled();
    expect(screen.queryByRole("region", { name: /Comprobantes por revisar/ })).toBeNull();
  });

  it("ON: the queue sits on the counter", async () => {
    render(
      <Providers flags={{ module_finance: true, module_payment_vouchers: true }}>
        <AppShell />
      </Providers>,
    );
    expect(await screen.findByRole("region", { name: /Comprobantes por revisar \(1\)/ })).toBeInTheDocument();
  });

  it("ON without module_finance: nothing — the vouchers live in finance", async () => {
    render(
      <Providers flags={{ module_payment_vouchers: true }}>
        <AppShell />
      </Providers>,
    );
    await waitFor(() => expect(apiMe).toHaveBeenCalled());
    expect(listPendingVouchers).not.toHaveBeenCalled();
  });
});
