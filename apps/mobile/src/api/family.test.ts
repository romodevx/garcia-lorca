import AsyncStorage from "@react-native-async-storage/async-storage";
import {
  getAccount,
  getAttendance,
  getGrades,
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

  describe("getAccount", () => {
    it("returns the statement, defaulting null charge/payment lists to empty", async () => {
      const fetchMock = jest.fn().mockResolvedValue(
        jsonOk({
          charges: [
            {
              id: "c1",
              description: "Colegiatura junio",
              amountCents: 240000,
              discountCents: 0,
              paidCents: 0,
              remainingCents: 240000,
              dueDate: "2026-06-15",
              status: "pending",
            },
          ],
          payments: null,
          totalBalanceCents: 240000,
        }),
      );
      globalThis.fetch = fetchMock as unknown as typeof fetch;

      const account = await getAccount("s1");

      expect(account.totalBalanceCents).toBe(240000);
      expect(account.charges).toHaveLength(1);
      expect(account.payments).toEqual([]);
      expect(lastUrl(fetchMock)).toContain("/api/v1/family/students/s1/account");
    });

    // What the school is asking for TODAY, which is not the same as the year.
    // The parent is the one person who cannot walk to the office and ask.
    it("carries the day's figure and the day it was worked out for", async () => {
      globalThis.fetch = jest.fn().mockResolvedValue(
        jsonOk({
          charges: [],
          payments: [],
          totalBalanceCents: 3200000,
          dueTodayCents: 775000,
          surchargeCents: 15000,
          earlyDiscountCents: 10000,
          asOf: "2026-11-20",
        }),
      ) as unknown as typeof fetch;

      const account = await getAccount("s1");

      expect(account.dueTodayCents).toBe(775000);
      expect(account.surchargeCents).toBe(15000);
      expect(account.earlyDiscountCents).toBe(10000);
      expect(account.asOf).toBe("2026-11-20");
    });

    // A gateway older than M18 sends none of it. Zero would tell a family that
    // owes money that they owe nothing, so the EMPTY asOf is what the screen
    // reads to withhold the figure instead of inventing one.
    it("leaves the day unknown when the gateway did not send it", async () => {
      globalThis.fetch = jest.fn().mockResolvedValue(
        jsonOk({ charges: [], payments: [], totalBalanceCents: 240000 }),
      ) as unknown as typeof fetch;

      const account = await getAccount("s1");

      expect(account.asOf).toBe("");
      expect(account.dueTodayCents).toBe(0);
    });

    it("throws an ApiError on a non-2xx response", async () => {
      globalThis.fetch = jest.fn().mockResolvedValue(notOk(404)) as unknown as typeof fetch;
      await expect(getAccount("s1")).rejects.toMatchObject({ status: 404 });
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

  describe("getAttendance", () => {
    it("returns the records and forwards the from/to range", async () => {
      const fetchMock = jest.fn().mockResolvedValue(
        jsonOk({
          records: [{ id: "a1", enrollmentId: "e1", date: "2026-06-04", status: "absent", note: "" }],
        }),
      );
      globalThis.fetch = fetchMock as unknown as typeof fetch;

      const records = await getAttendance("s1", "2026-06-01", "2026-06-30");

      expect(records).toHaveLength(1);
      expect(records[0]).toMatchObject({ date: "2026-06-04", status: "absent" });
      const url = lastUrl(fetchMock);
      expect(url).toContain("/api/v1/family/students/s1/attendance");
      expect(url).toContain("from=2026-06-01");
      expect(url).toContain("to=2026-06-30");
    });

    it("returns an empty array when the envelope carries a null list", async () => {
      globalThis.fetch = jest.fn().mockResolvedValue(jsonOk({ records: null })) as unknown as typeof fetch;
      expect(await getAttendance("s1")).toEqual([]);
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
});
