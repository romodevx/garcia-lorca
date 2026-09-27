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
          { id: "rc2", termId: "t2", termName: "2.º trimestre", version: 2, issuedAt: "2026-12-10T18:00:00Z", generalAverage: 8.75, hasGrades: true },
          { id: "rc1", termId: "t1", termName: "1.er trimestre", cycleName: "2026-2027", version: 1, issuedAt: null, generalAverage: null },
        ],
      }),
    );
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const cards = await listReportCards("s 1");

    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/family/students/s%201/report-cards");
    expect(cards).toEqual([
      { id: "rc2", termId: "t2", termName: "2.º trimestre", cycleName: "", version: 2, issuedAt: "2026-12-10T18:00:00Z", generalAverage: 8.75 },
      { id: "rc1", termId: "t1", termName: "1.er trimestre", cycleName: "2026-2027", version: 1, issuedAt: null, generalAverage: null },
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
        cycleName: "2026-2027",
        months: ["2026-09", "2026-10"],
        lines: [{ subjectName: "Matemáticas", months: { "2026-09": 9.5, "2026-10": "8" }, average: 8.75 }],
        generalAverage: 8.75,
        scale: { min: 5, max: 10, decimals: 1, passing: 6 },
        hasGrades: true,
      }),
    );
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const card = await getReportCard("s1", "rc 1");

    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/family/students/s1/report-cards/rc%201");
    expect(card.lines[0]).toEqual({ subjectName: "Matemáticas", months: { "2026-09": 9.5, "2026-10": 8 }, average: 8.75 });
    expect(card.scale?.passing).toBe(6);
    expect(card.hasGrades).toBe(true);
  });

  it("fills a sparse report card with safe defaults", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(respond(200, { id: "rc1", lines: null, months: null })) as unknown as typeof fetch;

    const card = await getReportCard("s1", "rc1");

    expect(card).toEqual({
      id: "rc1",
      version: 1,
      issuedAt: null,
      termName: "",
      cycleName: "",
      months: [],
      lines: [],
      generalAverage: null,
      scale: null,
      hasGrades: false,
    });
  });

  it("builds the PDF path for a child's report card", () => {
    expect(reportCardPdfPath("s 1", "rc/1")).toBe("/api/v1/family/students/s%201/report-cards/rc%2F1/pdf");
  });
});
