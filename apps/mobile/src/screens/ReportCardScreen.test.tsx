import { act, fireEvent, screen, waitFor } from "@testing-library/react-native";
import { clearTokens, saveTokens } from "../api";
import { downloadPdf, PdfError, sharePdf } from "../files/reportCardPdf";
import { renderWithProviders } from "../test/renderWithProviders";
import { ReportCardScreen } from "./ReportCardScreen";

jest.mock("../files/reportCardPdf", () => {
  const actual = jest.requireActual("../files/reportCardPdf");
  return { ...actual, downloadPdf: jest.fn(), sharePdf: jest.fn(async () => undefined) };
});

const mateo = { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "3° A" };
const lucia = { id: "s2", firstName: "Lucía", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "1° A" };

const card = {
  id: "rc1",
  version: 2,
  issuedAt: "2026-12-10T18:00:00Z",
  termName: "1.er trimestre",
  cycleName: "2026-2027",
  months: ["2026-09", "2026-10", "2026-11"],
  lines: [
    { subjectName: "Matemáticas", months: { "2026-09": 9.5, "2026-10": 8 }, average: 8.75 },
    { subjectName: "Historia", months: { "2026-09": 5, "2026-10": 5.5, "2026-11": 5 }, average: 5.16 },
  ],
  generalAverage: 6.95,
  scale: { min: 5, max: 10, decimals: 1, passing: 6 },
  hasGrades: true,
};

function mockGateway(reply: { status: number; body?: unknown }): jest.Mock {
  const fetchMock = jest.fn(async (url: string) => {
    const u = String(url);
    if (/\/family\/students\/s[12]\/report-cards\/rc1$/.test(u)) {
      return { ok: reply.status === 200, status: reply.status, json: async () => reply.body ?? card };
    }
    if (u.endsWith("/family/students")) return { ok: true, status: 200, json: async () => ({ students: [mateo, lucia] }) };
    return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
  });
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  return fetchMock;
}

describe("ReportCardScreen", () => {
  beforeEach(async () => {
    jest.clearAllMocks();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("renders the boleta as a subject × month table with each average", async () => {
    mockGateway({ status: 200 });
    await renderWithProviders(<ReportCardScreen id="rc1" studentId="s1" />);

    await waitFor(() => expect(screen.getByText("1.er trimestre")).toBeTruthy());
    expect(screen.getByText("2026-2027 · Versión 2 · Actualizada")).toBeTruthy();
    // The header row is visual only: each row's label already names its months.
    expect(screen.getByText("Sep", { includeHiddenElements: true })).toBeTruthy();
    expect(screen.getByText("Prom.", { includeHiddenElements: true })).toBeTruthy();
    expect(screen.getByLabelText("Matemáticas: septiembre 9.5, octubre 8, noviembre sin calificación, promedio 8.75")).toBeTruthy();
    expect(screen.getByLabelText("Historia: septiembre 5, octubre 5.5, noviembre 5, promedio 5.16, reprobada")).toBeTruthy();
    expect(screen.getByText("Reprobada")).toBeTruthy();
    expect(screen.getByLabelText("Promedio general: 6.95")).toBeTruthy();
  });

  it("says so when the term closed without grades", async () => {
    mockGateway({ status: 200, body: { ...card, lines: [], generalAverage: null, hasGrades: false } });
    await renderWithProviders(<ReportCardScreen id="rc1" studentId="s1" />);

    await waitFor(() => expect(screen.getByText("Sin calificaciones capturadas en este periodo")).toBeTruthy());
    expect(screen.queryByText("Prom.", { includeHiddenElements: true })).toBeNull();
  });

  it("says a superseded or foreign boleta is not available on a 404", async () => {
    mockGateway({ status: 404 });
    await renderWithProviders(<ReportCardScreen id="rc1" studentId="s1" />);

    await waitFor(() => expect(screen.getByText("Esta boleta ya no está disponible")).toBeTruthy());
  });

  it("downloads the PDF and opens the share sheet", async () => {
    mockGateway({ status: 200 });
    let reportProgress: (f: number | null) => void = () => undefined;
    let finish: (uri: string) => void = () => undefined;
    (downloadPdf as jest.Mock).mockImplementation(
      (req: { onProgress: (f: number | null) => void }) =>
        new Promise<string>((resolve) => {
          reportProgress = req.onProgress;
          finish = resolve;
        }),
    );
    await renderWithProviders(<ReportCardScreen id="rc1" studentId="s1" />);
    await waitFor(() => expect(screen.getByRole("button", { name: "Descargar PDF" })).toBeTruthy());

    await fireEvent.press(screen.getByRole("button", { name: "Descargar PDF" }));
    await waitFor(() => expect(screen.getByText("Descargando…")).toBeTruthy());
    await act(async () => reportProgress(0.4));
    await waitFor(() => expect(screen.getByText("Descargando… 40 %")).toBeTruthy());
    await act(async () => finish("file:///cache/boletas/boleta.pdf"));

    await waitFor(() => expect(sharePdf).toHaveBeenCalledWith("file:///cache/boletas/boleta.pdf", "Boleta 1.er trimestre"));
    expect(downloadPdf).toHaveBeenCalledWith(
      expect.objectContaining({ path: "/api/v1/family/students/s1/report-cards/rc1/pdf", fileName: "boleta-s1-1er-trimestre-v2.pdf" }),
    );
    await waitFor(() => expect(screen.getByRole("button", { name: "Descargar PDF" })).toBeTruthy());
  });

  it("explains a failed download in plain words and lets the parent retry", async () => {
    mockGateway({ status: 200 });
    (downloadPdf as jest.Mock).mockRejectedValueOnce(new PdfError("network")).mockResolvedValueOnce("file:///b.pdf");
    await renderWithProviders(<ReportCardScreen id="rc1" studentId="s1" />);
    await waitFor(() => expect(screen.getByRole("button", { name: "Descargar PDF" })).toBeTruthy());

    await fireEvent.press(screen.getByRole("button", { name: "Descargar PDF" }));

    await waitFor(() => expect(screen.getByText(/No pudimos descargar la boleta/)).toBeTruthy());
    expect(sharePdf).not.toHaveBeenCalled();

    await fireEvent.press(screen.getByRole("button", { name: "Descargar PDF" }));
    await waitFor(() => expect(sharePdf).toHaveBeenCalledWith("file:///b.pdf", "Boleta 1.er trimestre"));
    expect(screen.queryByText(/No pudimos descargar la boleta/)).toBeNull();
  });

  it("shows the boleta of the child named in the route, not the selected child", async () => {
    const fetchMock = mockGateway({ status: 200 });
    (downloadPdf as jest.Mock).mockResolvedValueOnce("file:///b.pdf");
    await renderWithProviders(<ReportCardScreen id="rc1" studentId="s2" />);
    await waitFor(() => expect(screen.getByRole("button", { name: "Descargar PDF" })).toBeTruthy());

    const cardReads = fetchMock.mock.calls.map(([url]) => String(url)).filter((u) => u.includes("/report-cards/"));
    expect(cardReads.every((u) => u.includes("/family/students/s2/report-cards/rc1"))).toBe(true);
    expect(cardReads.length).toBeGreaterThan(0);

    await fireEvent.press(screen.getByRole("button", { name: "Descargar PDF" }));
    await waitFor(() =>
      expect(downloadPdf).toHaveBeenCalledWith(
        expect.objectContaining({ path: "/api/v1/family/students/s2/report-cards/rc1/pdf", fileName: "boleta-s2-1er-trimestre-v2.pdf" }),
      ),
    );
  });

  it("tells the parent when the phone cannot open the PDF", async () => {
    mockGateway({ status: 200 });
    (downloadPdf as jest.Mock).mockResolvedValueOnce("file:///b.pdf");
    (sharePdf as jest.Mock).mockRejectedValueOnce(new PdfError("unavailable"));
    await renderWithProviders(<ReportCardScreen id="rc1" studentId="s1" />);
    await waitFor(() => expect(screen.getByRole("button", { name: "Descargar PDF" })).toBeTruthy());

    await fireEvent.press(screen.getByRole("button", { name: "Descargar PDF" }));

    await waitFor(() => expect(screen.getByText(/no puede abrir el PDF/)).toBeTruthy());
  });
});
