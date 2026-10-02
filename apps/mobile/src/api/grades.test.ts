import AsyncStorage from "@react-native-async-storage/async-storage";
import { ApiError } from "./errors";
import { getPeriodGrades } from "./grades";
import { clearTokens, saveTokens } from "./tokenStore";

function respond(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as unknown as Response;
}

const P1 = { id: "p1", ordinal: 1, name: "1.er trimestre", startsOn: "2026-08-24", endsOn: "2026-11-30" };
const P2 = { id: "p2", ordinal: 2, name: "2.º trimestre", startsOn: "2026-12-01", endsOn: "2027-03-15" };

describe("getPeriodGrades", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    await clearTokens();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });

  it("reads the child's grades per evaluation period", async () => {
    const fetchMock = jest.fn().mockResolvedValue(
      respond(200, {
        cycleName: "2026-2027",
        periods: [P1, P2],
        scale: { min: 5, max: 10, decimals: 0, passing: 6 },
        gradingEnabled: true,
        subjects: [{ subjectId: "m", subjectName: "Matemáticas", grades: { p1: 9 } }],
      }),
    );
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const grades = await getPeriodGrades("s 1");

    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/family/students/s%201/grades/periods");
    expect(grades.periods).toEqual([P1, P2]);
    expect(grades.scale?.passing).toBe(6);
    expect(grades.subjects[0]?.grades.p1).toBe(9);
  });

  it("orders the periods by ordinal whatever order they arrive in", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(
      respond(200, { cycleName: "2026-2027", periods: [P2, P1], gradingEnabled: true, subjects: [] }),
    ) as unknown as typeof fetch;

    expect((await getPeriodGrades("s1")).periods.map((p) => p.id)).toEqual(["p1", "p2"]);
  });

  it("fills absent optional fields with safe defaults", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(
      respond(200, { cycleName: "2026-2027", periods: null, subjects: [{ subjectId: "m", subjectName: "Arte", grades: null }] }),
    ) as unknown as typeof fetch;

    const grades = await getPeriodGrades("s1");

    expect(grades.periods).toEqual([]);
    expect(grades.gradingEnabled).toBe(true);
    expect(grades.scale).toBeNull();
    expect(grades.subjects[0]?.grades).toEqual({});
  });

  it("reads an empty 0–0 scale as no scale at all (nothing to judge by)", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(
      respond(200, { cycleName: "2026-2027", periods: [], gradingEnabled: false, scale: { min: 0, max: 0, decimals: 0, passing: 0 }, subjects: [] }),
    ) as unknown as typeof fetch;

    expect((await getPeriodGrades("s1")).scale).toBeNull();
  });

  it("throws an ApiError carrying the status (404 = not your child)", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(respond(404, {})) as unknown as typeof fetch;

    const failure = await getPeriodGrades("s1").catch((e: unknown) => e);

    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).status).toBe(404);
  });
});
