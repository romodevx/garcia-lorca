import { fireEvent, screen, waitFor } from "@testing-library/react-native";
import { router } from "expo-router";
import { type ReactElement } from "react";
import { Pressable, Text } from "react-native";
import { clearTokens, saveTokens } from "../api";
import { useAppState } from "../state/AppState";
import { renderWithProviders } from "../test/renderWithProviders";
import { GradesScreen } from "./GradesScreen";

type Card = { id: string; termId: string; termName: string; cycleName?: string; version: number; issuedAt: string | null; generalAverage: number | null; hasGrades?: boolean };
type Reply = { status: number; reportCards?: Card[] } | "hang";

const mateo = { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "3° A" };
const lucia = { id: "s2", firstName: "Lucía", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "1° A" };

const second: Card = { id: "rc2", termId: "t2", termName: "2.º trimestre", cycleName: "2026-2027", version: 2, issuedAt: "2027-03-20T18:00:00Z", generalAverage: 8.75, hasGrades: true };
const first: Card = { id: "rc1", termId: "t1", termName: "1.er trimestre", cycleName: "2026-2027", version: 1, issuedAt: "2026-12-10T18:00:00Z", generalAverage: 9, hasGrades: true };

function mockGateway(replies: Record<string, Reply | (() => Reply)>, children = [mateo]): jest.Mock {
  const fetchMock = jest.fn(async (url: string) => {
    const u = String(url);
    const cards = /\/family\/students\/([^/]+)\/report-cards$/.exec(u);
    if (cards) {
      const entry = replies[cards[1] ?? ""] ?? { status: 200 };
      const reply = typeof entry === "function" ? entry() : entry;
      if (reply === "hang") return new Promise(() => undefined);
      if (reply.status !== 200) return { ok: false, status: reply.status, json: async () => ({}) };
      return { ok: true, status: 200, json: async () => ({ reportCards: reply.reportCards ?? [] }) };
    }
    if (/\/grades\/monthly/.test(u)) {
      return { ok: true, status: 200, json: async () => ({ cycleName: "2026-2027", months: ["2026-09"], subjects: [] }) };
    }
    if (u.endsWith("/family/students")) return { ok: true, status: 200, json: async () => ({ students: children }) };
    return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
  });
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  return fetchMock;
}

const openBoletas = async (): Promise<void> => {
  await waitFor(() => expect(screen.getByRole("tab", { name: "Boletas" })).toBeTruthy());
  await fireEvent.press(screen.getByRole("tab", { name: "Boletas" }));
};

describe("GradesScreen — Boletas", () => {
  beforeEach(async () => {
    jest.clearAllMocks();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("shows two segments and opens on Por mes", async () => {
    mockGateway({});
    await renderWithProviders(<GradesScreen currentMonth="2026-09" />);

    await waitFor(() => expect(screen.getByRole("tab", { name: "Por mes" })).toBeTruthy());
    expect(screen.getByRole("tab", { name: "Por mes" }).props.accessibilityState).toMatchObject({ selected: true });
    expect(screen.getByRole("tab", { name: "Boletas" }).props.accessibilityState).toMatchObject({ selected: false });
  });

  it("lists the issued boletas with version badge, average and date", async () => {
    mockGateway({ s1: { status: 200, reportCards: [second, first] } });
    await renderWithProviders(<GradesScreen currentMonth="2026-09" />);
    await openBoletas();

    await waitFor(() => expect(screen.getByText("2.º trimestre · 2026-2027")).toBeTruthy());
    expect(screen.getByText("1.er trimestre · 2026-2027")).toBeTruthy();
    expect(screen.getByText("Versión 2 · Actualizada")).toBeTruthy();
    expect(screen.getAllByText(/Versión/)).toHaveLength(1);
    expect(screen.getByText("8.75")).toBeTruthy();
    expect(screen.getByLabelText(/^2\.º trimestre 2026-2027, versión 2, actualizada, promedio general 8\.75, emitida el/)).toBeTruthy();
  });

  it("opens the boleta preview when a boleta is tapped", async () => {
    mockGateway({ s1: { status: 200, reportCards: [second] } });
    await renderWithProviders(<GradesScreen currentMonth="2026-09" />);
    await openBoletas();
    await waitFor(() => expect(screen.getByText("2.º trimestre · 2026-2027")).toBeTruthy());

    await fireEvent.press(screen.getByLabelText(/^2\.º trimestre/));

    expect(router.push).toHaveBeenCalledWith({ pathname: "/boleta/[id]", params: { id: "rc2" } });
  });

  it("says so when no boleta has been issued", async () => {
    mockGateway({ s1: { status: 200, reportCards: [] } });
    await renderWithProviders(<GradesScreen currentMonth="2026-09" />);
    await openBoletas();

    await waitFor(() => expect(screen.getByText("Aún no hay boletas emitidas")).toBeTruthy());
  });

  it("shows an error with retry, and retrying loads the boletas", async () => {
    let fail = true;
    mockGateway({ s1: () => (fail ? { status: 500 } : { status: 200, reportCards: [first] }) });
    await renderWithProviders(<GradesScreen currentMonth="2026-09" />);
    await openBoletas();
    await waitFor(() => expect(screen.getByText("No pudimos cargar las boletas")).toBeTruthy());

    fail = false;
    await fireEvent.press(screen.getByRole("button", { name: "Reintentar" }));

    await waitFor(() => expect(screen.getByText("1.er trimestre · 2026-2027")).toBeTruthy());
  });

  it("opens straight on Boletas when the route asks for it", async () => {
    mockGateway({ s1: { status: 200, reportCards: [first] } });
    await renderWithProviders(<GradesScreen currentMonth="2026-09" requestedView="boletas" />);

    await waitFor(() => expect(screen.getByText("1.er trimestre · 2026-2027")).toBeTruthy());
  });

  it("clears one child's boletas the moment another child is selected", async () => {
    mockGateway({ s1: { status: 200, reportCards: [first] }, s2: "hang" }, [mateo, lucia]);
    await renderWithProviders(
      <>
        <PickChild id="s2" />
        <GradesScreen currentMonth="2026-09" requestedView="boletas" />
      </>,
    );
    await waitFor(() => expect(screen.getByText("1.er trimestre · 2026-2027")).toBeTruthy());

    await fireEvent.press(screen.getByText("pick s2"));

    await waitFor(() => expect(screen.queryByText("1.er trimestre · 2026-2027")).toBeNull());
  });
});

function PickChild({ id }: { id: string }): ReactElement {
  const { selectChild } = useAppState();
  return (
    <Pressable onPress={() => selectChild(id)}>
      <Text>{`pick ${id}`}</Text>
    </Pressable>
  );
}
