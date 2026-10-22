import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as issuesApi from "../../students/importIssues";
import { useImportIssues } from "./useImportIssues";

vi.mock("../../students/importIssues", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../students/importIssues")>()),
  listImportIssues: vi.fn(),
  acceptImportIssue: vi.fn(),
}));

const api = vi.mocked(issuesApi);
const ISSUE: issuesApi.StudentImportIssue = {
  id: "i1", studentId: "s1", field: "email", code: "email_invalid", originalValue: "ana@", storedValue: "", createdAt: "2026-10-21T10:00:00Z", resolution: "",
};

afterEach(() => vi.clearAllMocks());

describe("useImportIssues", () => {
  it("rolls a refused «Está bien así» back without duplicating an issue a reload already brought back", async () => {
    api.listImportIssues.mockResolvedValue([ISSUE]);
    let refuse: (e: Error) => void = () => undefined;
    api.acceptImportIssue.mockReturnValue(new Promise((_, reject) => { refuse = reject; }));
    const { result } = renderHook(() => useImportIssues("s1", true));
    await waitFor(() => expect(result.current.of("email")).toHaveLength(1));

    let accepting: Promise<void> = Promise.resolve();
    act(() => { accepting = result.current.accept("email"); });
    expect(result.current.of("email")).toHaveLength(0);
    act(() => result.current.reload());
    await waitFor(() => expect(result.current.of("email")).toHaveLength(1));
    await act(async () => { refuse(new Error("boom")); await accepting; });

    expect(result.current.of("email")).toHaveLength(1);
    expect(result.current.acceptError).toBe("No se pudo cerrar el aviso. Intenta de nuevo.");
  });
});
