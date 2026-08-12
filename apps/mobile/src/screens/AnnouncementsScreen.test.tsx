import { fireEvent, screen, waitFor } from "@testing-library/react-native";
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
});
