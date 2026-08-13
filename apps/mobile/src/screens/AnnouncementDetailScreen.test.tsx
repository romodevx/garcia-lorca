import { screen, waitFor } from "@testing-library/react-native";
import { clearTokens, saveTokens } from "../api";
import { renderWithProviders } from "../test/renderWithProviders";
import { AnnouncementDetailScreen } from "./AnnouncementDetailScreen";

interface Notif {
  id: string;
  studentId: string;
  type: string;
  title: string;
  body: string;
  createdAt: string;
  readAt: string;
}

function mockFetch(list: Notif[]): void {
  globalThis.fetch = jest
    .fn()
    .mockResolvedValue({ ok: true, status: 200, json: async () => ({ notifications: list }) }) as unknown as typeof fetch;
}

function mockFetchError(): void {
  globalThis.fetch = jest
    .fn()
    .mockResolvedValue({ ok: false, status: 500, json: async () => ({}) }) as unknown as typeof fetch;
}

function mockFetchPending(): void {
  globalThis.fetch = jest.fn(() => new Promise(() => undefined)) as unknown as typeof fetch;
}

const notification: Notif = {
  id: "n1",
  studentId: "s1",
  type: "attendance",
  title: "Asistencia registrada",
  body: "Mateo llegó a tiempo.",
  createdAt: "2026-08-12T10:00:00Z",
  readAt: "",
};

describe("AnnouncementDetailScreen", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });

  afterEach(async () => {
    await clearTokens();
  });

  it("renders the notification when it is in the loaded inbox", async () => {
    mockFetch([notification]);
    await renderWithProviders(<AnnouncementDetailScreen id="n1" />);
    await waitFor(() => expect(screen.getByText("Asistencia registrada")).toBeTruthy());
  });

  it("shows an error state with retry when the inbox failed to load", async () => {
    mockFetchError();
    await renderWithProviders(<AnnouncementDetailScreen id="n1" />);
    await waitFor(() => expect(screen.getByText(/No pudimos cargar los avisos/)).toBeTruthy());
    expect(screen.getByRole("button", { name: "Reintentar" })).toBeTruthy();
    // Never mislabel a load failure as a retracted notice.
    expect(screen.queryByText("Aviso no disponible")).toBeNull();
  });

  it("shows a spinner (not the retracted empty state) while the inbox is still loading", async () => {
    mockFetchPending();
    await renderWithProviders(<AnnouncementDetailScreen id="n1" />);
    expect(screen.getByTestId("notifications-loading")).toBeTruthy();
    expect(screen.queryByText("Aviso no disponible")).toBeNull();
  });

  it("shows the retracted empty state when the notice is absent from a loaded inbox", async () => {
    mockFetch([{ ...notification, id: "other" }]);
    await renderWithProviders(<AnnouncementDetailScreen id="n1" />);
    await waitFor(() => expect(screen.getByText("Aviso no disponible")).toBeTruthy());
  });
});
