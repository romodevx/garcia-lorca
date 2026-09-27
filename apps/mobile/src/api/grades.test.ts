import AsyncStorage from "@react-native-async-storage/async-storage";
import { ApiError } from "./errors";
import { getMonthlyGrades } from "./grades";
import { clearTokens, saveTokens } from "./tokenStore";

function respond(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as unknown as Response;
}

describe("getMonthlyGrades", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    await clearTokens();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });

  it("reads the child's monthly grades", async () => {
    const fetchMock = jest.fn().mockResolvedValue(
      respond(200, {
        cycleName: "2026-2027",
        months: ["2026-08", "2026-09"],
        scale: { min: 5, max: 10, decimals: 1, passing: 6 },
        gradingEnabled: true,
        subjects: [{ subjectId: "m", subjectName: "Matemáticas", grades: { "2026-09": 9.5 } }],
      }),
    );
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const grades = await getMonthlyGrades("s 1");

    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/family/students/s%201/grades/monthly");
    expect(grades.months).toEqual(["2026-08", "2026-09"]);
    expect(grades.scale?.passing).toBe(6);
    expect(grades.subjects[0]?.grades["2026-09"]).toBe(9.5);
  });

  it("fills absent optional fields with safe defaults", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(
      respond(200, { cycleName: "2026-2027", months: null, subjects: [{ subjectId: "m", subjectName: "Arte", grades: null }] }),
    ) as unknown as typeof fetch;

    const grades = await getMonthlyGrades("s1");

    expect(grades.months).toEqual([]);
    expect(grades.gradingEnabled).toBe(true);
    expect(grades.scale).toBeNull();
    expect(grades.subjects[0]?.grades).toEqual({});
  });

  it("reads an empty 0–0 scale as no scale at all (nothing to judge by)", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(
      respond(200, { cycleName: "2026-2027", months: [], gradingEnabled: false, scale: { min: 0, max: 0, decimals: 0, passing: 0 }, subjects: [] }),
    ) as unknown as typeof fetch;

    expect((await getMonthlyGrades("s1")).scale).toBeNull();
  });

  it("throws an ApiError carrying the status (404 = not your child)", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(respond(404, {})) as unknown as typeof fetch;

    const failure = await getMonthlyGrades("s1").catch((e: unknown) => e);

    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).status).toBe(404);
  });
});
