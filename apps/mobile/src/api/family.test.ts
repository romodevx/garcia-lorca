import AsyncStorage from "@react-native-async-storage/async-storage";
import {
  getGrades,
  getPaymentInfo,
  getStatement,
  statementPdfPath,
  getProfile,
  listChildren,
  markNotificationRead,
} from "./family";
import { clearTokens, saveTokens } from "./tokenStore";

function jsonOk(body: unknown): Response {
  return { ok: true, status: 200, json: async () => body } as unknown as Response;
}

function notOk(status: number): Response {
  return { ok: false, status, json: async () => ({}) } as unknown as Response;
}

/** Last URL the mocked fetch was called with. */
function lastUrl(mock: jest.Mock): string {
  return String(mock.mock.calls[mock.mock.calls.length - 1][0]);
}

/** RequestInit the mocked fetch was last called with. */
function lastInit(mock: jest.Mock): RequestInit {
  return (mock.mock.calls[mock.mock.calls.length - 1][1] ?? {}) as RequestInit;
}

describe("family API client", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    await clearTokens();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });

  describe("listChildren", () => {
    it("returns the students carried in the envelope", async () => {
      const fetchMock = jest.fn().mockResolvedValue(
        jsonOk({
          students: [
            { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "primaria", gradeGroup: "3° A" },
          ],
        }),
      );
      globalThis.fetch = fetchMock as unknown as typeof fetch;

      const children = await listChildren();

      expect(children).toHaveLength(1);
      expect(children[0]).toMatchObject({ id: "s1", firstName: "Mateo", gradeGroup: "3° A" });
      expect(lastUrl(fetchMock)).toContain("/api/v1/family/students");
    });

    it("returns an empty array when the envelope carries a null list", async () => {
      globalThis.fetch = jest.fn().mockResolvedValue(jsonOk({ students: null })) as unknown as typeof fetch;
      expect(await listChildren()).toEqual([]);
    });

    it("throws an ApiError on a non-2xx response", async () => {
      globalThis.fetch = jest.fn().mockResolvedValue(notOk(403)) as unknown as typeof fetch;
      await expect(listChildren()).rejects.toMatchObject({ status: 403 });
    });
  });

  describe("getProfile", () => {
    it("returns the child profile with its current enrollment", async () => {
      const fetchMock = jest.fn().mockResolvedValue(
        jsonOk({
          id: "s1",
          firstName: "Mateo",
          lastName: "Herrera",
          level: "primaria",
          gradeGroup: "3° A",
          currentEnrollment: { id: "e1", studentId: "s1", status: "active" },
        }),
      );
      globalThis.fetch = fetchMock as unknown as typeof fetch;

      const profile = await getProfile("s1");

      expect(profile.currentEnrollment?.id).toBe("e1");
      expect(lastUrl(fetchMock)).toContain("/api/v1/family/students/s1/profile");
    });
  });

  // T7: the ledger is the backend's (finance.GetStatement). The app renders it
  // as it comes; it never rebuilds one from charges and payments again.
  describe("getStatement", () => {
    it("reads the family's ledger, defaulting what is missing", async () => {
      const fetchMock = jest.fn().mockResolvedValue(
        jsonOk({
          movements: [
            { kind: "charge", date: "2026-09-10", description: "Colegiatura septiembre", amountCents: 320000, balanceCents: 320000, accrued: false, notYetDue: false, voided: false },
          ],
          totals: { chargedCents: 320000, discountCents: 0, paidCents: 0, balanceCents: 320000, notYetDueCents: 0 },
          dueTodayCents: 320000,
          asOf: "2026-10-01",
        }),
      );
      globalThis.fetch = fetchMock as unknown as typeof fetch;

      const statement = await getStatement("s1");

      expect(lastUrl(fetchMock)).toContain("/api/v1/family/students/s1/statement");
      expect(statement.movements).toHaveLength(1);
      expect(statement.movements[0]?.balanceCents).toBe(320000);
      expect(statement.totals.balanceCents).toBe(320000);
      expect(statement.dueTodayCents).toBe(320000);
      expect(statement.surchargeCents).toBe(0);
      expect(statement.asOf).toBe("2026-10-01");
    });

    it("turns null lists and missing totals into empty ones", async () => {
      globalThis.fetch = jest.fn().mockResolvedValue(jsonOk({ movements: null })) as unknown as typeof fetch;

      const statement = await getStatement("s1");

      expect(statement.movements).toEqual([]);
      expect(statement.totals).toEqual({ chargedCents: 0, discountCents: 0, paidCents: 0, balanceCents: 0, notYetDueCents: 0 });
      expect(statement.asOf).toBe("");
    });

    it("throws an ApiError on a non-2xx response", async () => {
      globalThis.fetch = jest.fn().mockResolvedValue(notOk(404)) as unknown as typeof fetch;
      await expect(getStatement("s1")).rejects.toMatchObject({ status: 404 });
    });

    it("names the PDF's path", () => {
      expect(statementPdfPath("s1")).toBe("/api/v1/family/students/s1/statement/pdf");
    });
  });

  describe("getGrades", () => {
    it("returns the grade entries and forwards the termId filter", async () => {
      const fetchMock = jest.fn().mockResolvedValue(
        jsonOk({
          gradeEntries: [
            { id: "g1", subjectId: "sub1", termId: "t1", teacherId: "te1", value: 9.2, subjectName: "Español", termName: "1er periodo" },
          ],
        }),
      );
      globalThis.fetch = fetchMock as unknown as typeof fetch;

      const entries = await getGrades("s1", "t1");

      expect(entries).toHaveLength(1);
      expect(entries[0]).toMatchObject({ subjectName: "Español", value: 9.2 });
      const url = lastUrl(fetchMock);
      expect(url).toContain("/api/v1/family/students/s1/grades");
      expect(url).toContain("termId=t1");
    });

    it("omits the termId query when no term is given", async () => {
      const fetchMock = jest.fn().mockResolvedValue(jsonOk({ gradeEntries: null }));
      globalThis.fetch = fetchMock as unknown as typeof fetch;

      expect(await getGrades("s1")).toEqual([]);
      expect(lastUrl(fetchMock)).not.toContain("termId");
    });
  });

  describe("markNotificationRead", () => {
    it("POSTs to the guardian read route and resolves on success", async () => {
      const fetchMock = jest.fn().mockResolvedValue(jsonOk({ id: "n1", readAt: "2026-06-12T10:00:00Z" }));
      globalThis.fetch = fetchMock as unknown as typeof fetch;

      await expect(markNotificationRead("n1")).resolves.toBeUndefined();

      expect(lastUrl(fetchMock)).toContain("/api/v1/family/notifications/n1/read");
      expect(lastInit(fetchMock).method).toBe("POST");
    });

    it("throws an ApiError on a non-2xx response", async () => {
      globalThis.fetch = jest.fn().mockResolvedValue(notOk(404)) as unknown as typeof fetch;
      await expect(markNotificationRead("n1")).rejects.toMatchObject({ status: 404 });
    });
  });

  describe("getPaymentInfo", () => {
    it("reads how to pay for the child from its own route", async () => {
      const fetchMock = jest.fn().mockResolvedValue(
        jsonOk({ clabe: "012180001234567899", bank: "BBVA México", beneficiary: "Educación García Lorca, S.C.", reference: "1234567" }),
      );
      globalThis.fetch = fetchMock as unknown as typeof fetch;

      const info = await getPaymentInfo("s1");

      expect(lastUrl(fetchMock)).toContain("/api/v1/family/students/s1/payment-info");
      expect(info).toEqual({ clabe: "012180001234567899", bank: "BBVA México", beneficiary: "Educación García Lorca, S.C.", reference: "1234567" });
    });

    it("fills what an older gateway leaves out with empty text", async () => {
      globalThis.fetch = jest.fn().mockResolvedValue(jsonOk({})) as unknown as typeof fetch;

      expect(await getPaymentInfo("s1")).toEqual({ clabe: "", bank: "", beneficiary: "", reference: "" });
    });
  });
});
