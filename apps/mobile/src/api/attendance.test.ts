import AsyncStorage from "@react-native-async-storage/async-storage";
import { getAttendanceMonth } from "./attendance";
import { ApiError } from "./errors";
import { clearTokens, saveTokens } from "./tokenStore";

function respond(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as unknown as Response;
}

describe("getAttendanceMonth", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    await clearTokens();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });

  it("asks for the month and returns the days, today and summary", async () => {
    const fetchMock = jest.fn().mockResolvedValue(
      respond(200, {
        month: "2026-09",
        today: "2026-09-28",
        days: [
          {
            date: "2026-09-28",
            status: "partial",
            mode: "per_class",
            classes: [{ subjectName: "Matemáticas", status: "absent", note: "Salió temprano" }],
          },
        ],
        summary: { present: 10, absent: 1, late: 2, excused: 0, partial: 1 },
      }),
    );
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const month = await getAttendanceMonth("s 1", "2026-09");

    const url = String(fetchMock.mock.calls[0][0]);
    expect(url).toContain("/api/v1/family/students/s%201/attendance?month=2026-09");
    expect(month.today).toBe("2026-09-28");
    expect(month.days[0]?.classes[0]).toEqual({ subjectName: "Matemáticas", status: "absent", note: "Salió temprano" });
    expect(month.summary).toEqual({ present: 10, absent: 1, late: 2, excused: 0, partial: 1 });
  });

  it("normalizes missing lists and counts instead of fabricating them", async () => {
    globalThis.fetch = jest
      .fn()
      .mockResolvedValue(
        respond(200, { month: "2026-09", today: "2026-09-28", days: [{ date: "2026-09-28", status: "present", mode: "daily" }] }),
      ) as unknown as typeof fetch;

    const month = await getAttendanceMonth("s1", "2026-09");

    expect(month.days[0]?.classes).toEqual([]);
    expect(month.summary).toEqual({ present: 0, absent: 0, late: 0, excused: 0, partial: 0 });
  });

  it("throws an ApiError carrying 404 for a child that is not linked", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(respond(404, {})) as unknown as typeof fetch;

    await expect(getAttendanceMonth("x", "2026-09")).rejects.toEqual(expect.any(ApiError));
    await expect(getAttendanceMonth("x", "2026-09")).rejects.toMatchObject({ status: 404 });
  });
});
