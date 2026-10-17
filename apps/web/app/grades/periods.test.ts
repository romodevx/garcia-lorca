import { afterEach, describe, expect, it, vi } from "vitest";

import { GradeRequestError } from "./requestError";
import { boletaFileName, fileNameFromDisposition, generalAverageLabel, periodsClient, type MissingCapture, type ReportCard } from "./periods";

const BASE = "http://gw";

afterEach(() => vi.unstubAllGlobals());

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function call(mock: ReturnType<typeof vi.fn>, index = 0): [string, RequestInit] {
  const c = mock.mock.calls[index] as [string, RequestInit];
  return [c[0], c[1]];
}

const period = {
  termId: "t1", termName: "1.er trimestre", ordinal: 1, levelId: "l1", levelName: "PRIMARIA",
  startsOn: "2026-08-24", endsOn: "2026-11-30", status: "open", version: 0, progress: { captured: 412, expected: 450 },
};

describe("periodsClient", () => {
  const api = periodsClient(BASE);

  it("lists a cycle's periods with credentials", async () => {
    const fetchMock = vi.fn().mockResolvedValue(json({ periods: [period] }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.list("c1")).resolves.toEqual([period]);
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/grades/periods?cycleId=c1");
    expect(call(fetchMock)[1].credentials).toBe("include");
  });

  it("reads a period's capture progress", async () => {
    const progress = { termId: "t1", unfinished: false, endsOn: "2026-11-30", groupsWithoutPeriod: [], expected: 10, captured: 8, missing: [], reportCards: 4, skippedWithoutGrades: 1, withdrawnReportCards: 0 };
    const fetchMock = vi.fn().mockResolvedValue(json(progress));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.progress("t1")).resolves.toEqual(progress);
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/grades/periods/t1/progress");
  });

  it("closes with {force} and returns how many boletas were issued", async () => {
    const fetchMock = vi.fn().mockResolvedValue(json({ status: "closed", version: 1, reportCards: 124, skippedWithoutGrades: 1, withdrawnReportCards: 0 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.close("t1", true)).resolves.toEqual({ status: "closed", version: 1, reportCards: 124, skippedWithoutGrades: 1, withdrawnReportCards: 0 });
    const [url, init] = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/grades/periods/t1/close");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ force: true });
  });

  it("a refused close carries the server's words and its code (MISSING_GRADES)", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("Faltan 38 calificaciones por capturar", { status: 409, headers: { "X-Error-Code": "MISSING_GRADES" } })));

    const err = (await api.close("t1", false).catch((e: unknown) => e)) as GradeRequestError;
    expect(err).toBeInstanceOf(GradeRequestError);
    expect(err.code).toBe("MISSING_GRADES");
    expect(err.detail).toBe("Faltan 38 calificaciones por capturar");
  });

  it("reopens a period", async () => {
    const fetchMock = vi.fn().mockResolvedValue(json({ status: "open" }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.reopen("t1")).resolves.toEqual({ status: "open" });
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/grades/periods/t1/reopen");
    expect(call(fetchMock)[1].method).toBe("POST");
  });

  it("lists a group's boletas for a term", async () => {
    const card = { id: "rc1", studentId: "s1", studentName: "Ana Pérez", groupLabel: "1° A", version: 2, issuedAt: "2026-12-01T10:00:00Z", generalAverage: 8.7 };
    const fetchMock = vi.fn().mockResolvedValue(json({ reportCards: [card] }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.reportCards("t1", "g1")).resolves.toEqual([card]);
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/grades/report-cards?termId=t1&groupId=g1");
  });

  it("fetches a boleta's PDF as a blob, with credentials, and the server's file name", async () => {
    const headers = {
      "Content-Type": "application/pdf",
      "Content-Disposition": 'attachment; filename="boleta-2026-00042-1-er-trimestre-v1.pdf"',
    };
    // A string body, not a Blob: jsdom's Blob has no stream() on every Node, and undici's Response needs it.
    const fetchMock = vi.fn().mockResolvedValue(new Response("%PDF-1.4", { status: 200, headers }));
    vi.stubGlobal("fetch", fetchMock);

    const { blob, fileName } = await api.reportCardPdf("rc1");
    expect(blob.size).toBeGreaterThan(0);
    expect(fileName).toBe("boleta-2026-00042-1-er-trimestre-v1.pdf");
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/grades/report-cards/rc1/pdf");
    expect(call(fetchMock)[1].credentials).toBe("include");
  });

  it("a PDF without a readable Content-Disposition has no file name (the caller falls back)", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("%PDF-1.4", { status: 200 })));

    expect((await api.reportCardPdf("rc1")).fileName).toBeUndefined();
  });
});

describe("boletaFileName", () => {
  it("is boleta-<alumno>-<trimestre>-v<N>.pdf, without accents or spaces", () => {
    expect(boletaFileName("José Núñez Peña", "1.er trimestre", 2)).toBe("boleta-jose-nunez-pena-1-er-trimestre-v2.pdf");
  });
});

describe("fileNameFromDisposition", () => {
  it.each([
    ['attachment; filename="boleta-2026-00042-1-er-trimestre-v1.pdf"', "boleta-2026-00042-1-er-trimestre-v1.pdf"],
    ["attachment; filename=boleta-a-v1.pdf", "boleta-a-v1.pdf"],
    ["attachment; filename*=UTF-8''boleta-jos%C3%A9-v1.pdf", "boleta-josé-v1.pdf"],
  ])("reads %s", (header, expected) => {
    expect(fileNameFromDisposition(header)).toBe(expected);
  });

  it.each([null, "", "attachment", 'attachment; filename="../../etc/passwd"'])("gives nothing usable for %s", (header) => {
    expect(fileNameFromDisposition(header)).toBeUndefined();
  });
});

describe("contract types (openapi GradePeriodProgress / ReportCardSummary)", () => {
  it("a missing capture names its subject by id and a boleta may not have an issue date yet", () => {
    const missing: MissingCapture = { groupId: "g", groupLabel: "1° A", subjectId: "s", subjectName: "Mate", teacherName: "Ada", missingCount: 1 };
    const card: ReportCard = { id: "rc", studentId: "st", studentName: "Ana", groupLabel: "1° A", version: 1, issuedAt: null, generalAverage: null };
    expect(missing.subjectId).toBe("s");
    expect(card.issuedAt).toBeNull();
  });
});

describe("generalAverageLabel", () => {
  it("prints the truncated average without trailing zeros, and a dash for none", () => {
    expect(generalAverageLabel(8.66)).toBe("8.66");
    expect(generalAverageLabel(8.5)).toBe("8.5");
    expect(generalAverageLabel(9)).toBe("9");
    expect(generalAverageLabel(null)).toBe("—");
  });
});
