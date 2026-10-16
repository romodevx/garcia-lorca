import { afterEach, describe, expect, it, vi } from "vitest";
import { previewWithdrawal, withdraw } from "./api";

describe("withdrawal API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("posts the enrollment id, date, and optional reason", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: "en-1" }), { status: 200 }));
    vi.stubGlobal("fetch", fetch);

    await withdraw("https://school.test/", "en-1", { withdrawnOn: "2026-10-15", reason: "Cambio de escuela" });

    expect(fetch).toHaveBeenCalledWith("https://school.test/api/v1/enrollments/withdraw", expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ enrollmentId: "en-1", withdrawnOn: "2026-10-15", reason: "Cambio de escuela" }),
    }));
  });

  it("gets the preview with the chosen date", async () => {
    const preview = { toCancel: [], toCancelCents: 0, remainingDebtCents: 1200 };
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify(preview), { status: 200 }));
    vi.stubGlobal("fetch", fetch);

    await expect(previewWithdrawal("https://school.test", "en-1", "2026-10-15")).resolves.toEqual(preview);
    expect(fetch).toHaveBeenCalledWith("https://school.test/api/v1/enrollments/en-1/withdrawal-preview?withdrawnOn=2026-10-15", expect.objectContaining({ credentials: "include" }));
  });
});
