import { fireEvent, screen, waitFor } from "@testing-library/react-native";
import { clearTokens, saveTokens } from "../api";
import { AnnouncementsScreen } from "./AnnouncementsScreen";
import { renderWithProviders } from "../test/renderWithProviders";

interface Notif {
  id: string;
  studentId: string;
  type: string;
  title: string;
  body: string;
  createdAt: string;
  readAt: string;
}

function mockFetchNotifications(list: Notif[]): void {
  globalThis.fetch = jest
    .fn()
    .mockResolvedValue({ ok: true, status: 200, json: async () => ({ notifications: list }) }) as unknown as typeof fetch;
}

function mockFetchError(): void {
  globalThis.fetch = jest
    .fn()
    .mockResolvedValue({ ok: false, status: 500, json: async () => ({}) }) as unknown as typeof fetch;
}

const unread: Notif = {
  id: "n1",
  studentId: "s1",
  type: "attendance",
  title: "Asistencia registrada",
  body: "Mateo llegó a tiempo.",
  createdAt: "2026-08-12T10:00:00Z",
  readAt: "",
};

describe("AnnouncementsScreen", () => {
  // The inbox only loads for a signed-in guardian: AppState skips the fetch when
  // there is no access token (see AppState refreshNotifications). Seed a token so
  // these render the network-backed path.
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });

  afterEach(async () => {
    await clearTokens();
  });

  it("renders notifications loaded from the family API", async () => {
    mockFetchNotifications([unread]);
    await renderWithProviders(<AnnouncementsScreen />);
    await waitFor(() => expect(screen.getByText("Asistencia registrada")).toBeTruthy());
  });

  it("clears the unread state when Marcar leídos is pressed", async () => {
    mockFetchNotifications([unread]);
    await renderWithProviders(<AnnouncementsScreen />);

    const markAll = await screen.findByRole("button", { name: "Marcar leídos" });
    fireEvent.press(markAll);

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Marcar leídos" })).toBeNull();
    });
  });

  it("persists a read acknowledgement via the family API (not a session-only hack)", async () => {
    const fetchMock = jest
      .fn()
      .mockResolvedValue({ ok: true, status: 200, json: async () => ({ notifications: [unread] }) });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await renderWithProviders(<AnnouncementsScreen />);
    fireEvent.press(await screen.findByRole("button", { name: "Marcar leídos" }));

    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(
          ([url, init]) =>
            String(url).includes("/family/notifications/n1/read") &&
            (init as RequestInit | undefined)?.method === "POST",
        ),
      ).toBe(true),
    );
  });

  it("renders a friendly empty state when there are no notifications", async () => {
    mockFetchNotifications([]);
    await renderWithProviders(<AnnouncementsScreen />);
    await waitFor(() => expect(screen.getByText("No hay avisos nuevos")).toBeTruthy());
    expect(screen.getByText("Te avisaremos cuando la escuela publique algo.")).toBeTruthy();
  });

  it("shows an error state with a retry action when the load fails", async () => {
    mockFetchError();
    await renderWithProviders(<AnnouncementsScreen />);
    await waitFor(() => expect(screen.getByText(/No pudimos cargar los avisos/)).toBeTruthy());
    expect(screen.getByRole("button", { name: "Reintentar" })).toBeTruthy();
  });

  // ISSUE-CC-303: with two children the inbox mixes both; each notice says whose
  // it is, and the family can narrow it to one child.
  describe("with two children", () => {
    const students = [
      { id: "s4", firstName: "ALUMNA", lastName: "CUATRO", level: "Primaria", levelId: "l1", gradeGroup: "B" },
      { id: "s2", firstName: "ALUMNA", lastName: "DOS", level: "Primaria", levelId: "l1", gradeGroup: "A" },
    ];
    const junta: Notif = { ...unread, id: "n4", studentId: "s4", title: "Junta de padres 3º B" };
    const recordatorio: Notif = { ...unread, id: "n2", studentId: "s2", title: "Recordatorio QA para DOS" };

    function mockSiblings(list: Notif[], children: unknown[] = students): void {
      globalThis.fetch = jest.fn(async (url: string) => {
        const body = String(url).endsWith("/family/students") ? { students: children } : { notifications: list };
        return { ok: true, status: 200, json: async () => body };
      }) as unknown as typeof fetch;
    }

    it("says which child each notice is for", async () => {
      mockSiblings([junta, recordatorio]);
      await renderWithProviders(<AnnouncementsScreen />);
      await waitFor(() => expect(screen.getByText("Para Alumna Cuatro")).toBeTruthy());
      expect(screen.getByText("Para Alumna Dos")).toBeTruthy();
    });

    it("narrows the inbox to one child and back to all", async () => {
      mockSiblings([junta, recordatorio]);
      await renderWithProviders(<AnnouncementsScreen />);
      await screen.findByText("Junta de padres 3º B");

      await fireEvent.press(screen.getByRole("tab", { name: "Alumna Dos" }));
      expect(screen.queryByText("Junta de padres 3º B")).toBeNull();
      expect(screen.getByText("Recordatorio QA para DOS")).toBeTruthy();

      await fireEvent.press(screen.getByRole("tab", { name: "Todos" }));
      expect(screen.getByText("Junta de padres 3º B")).toBeTruthy();
    });

    // CC-303 gate: "Marcar leídos" acts on what the family is looking at.
    it("marks only the filtered child's notices as read", async () => {
      mockSiblings([junta, recordatorio]);
      await renderWithProviders(<AnnouncementsScreen />);
      await screen.findByText("Junta de padres 3º B");

      await fireEvent.press(screen.getByRole("tab", { name: "Alumna Dos" }));
      await fireEvent.press(screen.getByRole("button", { name: "Marcar leídos" }));

      const posts = (): string[] =>
        (globalThis.fetch as jest.Mock).mock.calls
          .filter(([, init]) => (init as RequestInit | undefined)?.method === "POST")
          .map(([url]) => String(url));
      await waitFor(() => expect(posts().some((u) => u.includes("/family/notifications/n2/read"))).toBe(true));
      expect(posts().some((u) => u.includes("/family/notifications/n4/read"))).toBe(false);
    });

    it("neither labels nor filters for a single child", async () => {
      mockSiblings([junta], [students[0]]);
      await renderWithProviders(<AnnouncementsScreen />);
      await screen.findByText("Junta de padres 3º B");
      expect(screen.queryByText(/^Para /)).toBeNull();
      expect(screen.queryByRole("tab", { name: "Todos" })).toBeNull();
    });
  });
});
