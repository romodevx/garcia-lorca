import { afterEach, describe, expect, it, vi } from "vitest";

import { AdminError, deleteUser } from "./api";

// T2 — deleting a user. DELETE /api/v1/users/{id} answers 204; a refusal
// carries iam's own sentence, which is what the director must read.

afterEach(() => vi.restoreAllMocks());

describe("deleteUser", () => {
  it("DELETEs the user's path with the session cookie", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await deleteUser("http://gw/", "u2");

    const [calledUrl, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(calledUrl).toBe("http://gw/api/v1/users/u2");
    expect(init.method).toBe("DELETE");
    expect(init.credentials).toBe("include");
  });

  it("throws an AdminError carrying the status and the server's sentence", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response("no se puede eliminar al último director activo\n", { status: 409 })),
    );

    const err = await deleteUser("http://gw", "u2").catch((e: unknown) => e);

    expect(err).toBeInstanceOf(AdminError);
    expect((err as AdminError).status).toBe(409);
    expect((err as AdminError).detail).toBe("no se puede eliminar al último director activo");
  });

  it("carries the refusal's machine code (X-Error-Code)", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response("internal error", { status: 500, headers: { "X-Error-Code": "PROVIDER_PENDING" } })),
    );

    const err = await deleteUser("http://gw", "u2").catch((e: unknown) => e);

    expect((err as AdminError).code).toBe("PROVIDER_PENDING");
  });
});
