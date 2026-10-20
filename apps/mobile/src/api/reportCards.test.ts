import AsyncStorage from "@react-native-async-storage/async-storage";
import { ApiError } from "./errors";
import { getReportCard, listReportCards, reportCardPdfPath } from "./reportCards";
import { clearTokens, saveTokens } from "./tokenStore";

function respond(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as unknown as Response;
}

describe("report cards API", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    await clearTokens();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });

  it("lists the child's issued report cards, newest term first as served", async () => {
    const fetchMock = jest.fn().mockResolvedValue(
      respond(200, {
        reportCards: [
          { id: "rc2", termId: "t2", termName: "2.º trimestre", termOrdinal: 2, version: 2, issuedAt: "2026-12-10T18:00:00Z", generalAverage: "8.75", hasGrades: true },
          { id: "rc1", termId: "t1", termName: "1.er trimestre", cycleName: "2026-2027", version: 1, issuedAt: null, generalAverage: null },
        ],
      }),
    );
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const cards = await listReportCards("s 1");

    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/family/students/s%201/report-cards");
    expect(cards).toEqual([
      { id: "rc2", termId: "t2", termName: "2.º trimestre", termOrdinal: 2, cycleName: "", version: 2, issuedAt: "2026-12-10T18:00:00Z", generalAverage: 8.75, hasGrades: true, gradingLetters: false },
      { id: "rc1", termId: "t1", termName: "1.er trimestre", termOrdinal: 0, cycleName: "2026-2027", version: 1, issuedAt: null, generalAverage: null, hasGrades: false, gradingLetters: false },
    ]);
  });

  it("answers an empty list when the server sends none", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(respond(200, { reportCards: null })) as unknown as typeof fetch;
    expect(await listReportCards("s1")).toEqual([]);
  });

  it("throws the HTTP status so a 404 (not your child) is distinguishable", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(respond(404, {})) as unknown as typeof fetch;
    await expect(listReportCards("s1")).rejects.toEqual(new ApiError(404, "failed to load report cards"));
  });

  it("reads one report card with its lines, months and scale", async () => {
    const fetchMock = jest.fn().mockResolvedValue(
      respond(200, {
        id: "rc1",
        version: 1,
        issuedAt: "2026-11-30T18:00:00Z",
        termName: "1.er trimestre",
        termOrdinal: 1,
        cycleName: "2026-2027",
        months: ["2026-09", "2026-10"],
        lines: [{ subjectId: "sub-math", subjectName: "Matemáticas", months: { "2026-09": 9.5, "2026-10": "8" }, average: 8.75 }],
        generalAverage: 8.75,
        scale: { min: 5, max: 10, decimals: 1, passing: 6 },
        hasGrades: true,
      }),
    );
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const card = await getReportCard("s1", "rc 1");

    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/family/students/s1/report-cards/rc%201");
    expect(card.termOrdinal).toBe(1);
    expect(card.periods).toEqual([]);
    expect(card.lines[0]).toEqual({ subjectId: "sub-math", subjectName: "Matemáticas", periods: {}, letters: {}, months: { "2026-09": 9.5, "2026-10": 8 }, average: 8.75 });
    expect(card.scale?.passing).toBe(6);
    expect(card.hasGrades).toBe(true);
  });

  it("reads a T15 card: its evaluation-period columns, by ordinal, and each line's grade per period", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(
      respond(200, {
        id: "rc1",
        periods: [
          { id: "p2", ordinal: 2, name: "2.º trimestre" },
          { id: "p1", ordinal: 1, name: "1.er trimestre" },
        ],
        months: [],
        lines: [{ subjectId: "sub-math", subjectName: "Matemáticas", periods: { p1: 9, p2: "8" }, months: {}, average: 8.5 }],
        generalAverage: 8.5,
        hasGrades: true,
      }),
    ) as unknown as typeof fetch;

    const card = await getReportCard("s1", "rc1");

    expect(card.periods).toEqual([
      { id: "p1", ordinal: 1, name: "1.er trimestre" },
      { id: "p2", ordinal: 2, name: "2.º trimestre" },
    ]);
    expect(card.lines[0]?.periods).toEqual({ p1: 9, p2: 8 });
    expect(card.months).toEqual([]);
  });

  // Preescolar's boleta carries letters (owner, 20-oct): per line, no average.
  it("reads a letters card: each line's letter per period, and says it carries letters", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(
      respond(200, {
        id: "rc1", periods: [{ id: "p1", ordinal: 1, name: "1.er trimestre" }], months: [], gradingLetters: true, hasGrades: true,
        generalAverage: null,
        lines: [{ subjectId: "sub-len", subjectName: "Lenguajes", periods: {}, letters: { p1: "A" }, months: {}, average: null }],
      }),
    ) as unknown as typeof fetch;

    const card = await getReportCard("s1", "rc1");

    expect(card.gradingLetters).toBe(true);
    expect(card.lines[0]?.letters).toEqual({ p1: "A" });
    expect(card.lines[0]?.periods).toEqual({});
  });

  it("lists a letters card as carrying letters", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(
      respond(200, { reportCards: [{ id: "rc1", termName: "1.er trimestre", version: 1, generalAverage: null, hasGrades: true, gradingLetters: true }] }),
    ) as unknown as typeof fetch;
    const [card] = await listReportCards("s1");
    expect(card?.gradingLetters).toBe(true);
  });

  it("fills a sparse report card with safe defaults", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(respond(200, { id: "rc1", lines: null, months: null })) as unknown as typeof fetch;

    const card = await getReportCard("s1", "rc1");

    expect(card).toEqual({
      id: "rc1",
      version: 1,
      issuedAt: null,
      termName: "",
      termOrdinal: 0,
      cycleName: "",
      periods: [],
      months: [],
      lines: [],
      generalAverage: null,
      scale: null,
      hasGrades: false,
      gradingLetters: false,
    });
  });

  it("answers an empty subject id when a line arrives without one", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(respond(200, { id: "rc1", lines: [{ subjectName: "Arte" }] })) as unknown as typeof fetch;

    const card = await getReportCard("s1", "rc1");

    expect(card.lines[0]?.subjectId).toBe("");
  });

  it("builds the PDF path for a child's report card", () => {
    expect(reportCardPdfPath("s 1", "rc/1")).toBe("/api/v1/family/students/s%201/report-cards/rc%2F1/pdf");
  });
});
