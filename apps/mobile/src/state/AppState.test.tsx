import { screen, waitFor } from "@testing-library/react-native";
import { type ReactElement } from "react";
import { Text } from "react-native";
import { clearTokens, saveTokens } from "../api";
import { renderWithProviders } from "../test/renderWithProviders";
import { useAppState } from "./AppState";

/** Surfaces the notifications slice as plain text for assertions. */
function NotificationsProbe(): ReactElement {
  const { notifications, notificationsLoading, notificationsError } = useAppState();
  return (
    <>
      <Text testID="count">{String(notifications.length)}</Text>
      <Text testID="loading">{String(notificationsLoading)}</Text>
      <Text testID="error">{String(notificationsError)}</Text>
    </>
  );
}

const text = (id: string): unknown => screen.getByTestId(id).props.children;

describe("AppState notifications slice", () => {
  afterEach(async () => {
    await clearTokens();
  });

  it("no-ops to an empty, non-loading, non-error slice when signed out (no token)", async () => {
    await clearTokens();
    const fetchMock = jest.fn();
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await renderWithProviders(<NotificationsProbe />);

    await waitFor(() => expect(text("loading")).toBe("false"));
    expect(text("count")).toBe("0");
    expect(text("error")).toBe("false");
    // The security-relevant part: no unauthenticated request is fired.
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("fetches the inbox when an access token is present", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    const fetchMock = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        notifications: [
          {
            id: "n1",
            studentId: "s1",
            type: "attendance",
            title: "Asistencia registrada",
            body: "Mateo llegó a tiempo.",
            createdAt: "2026-08-12T10:00:00Z",
            readAt: "",
          },
        ],
      }),
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await renderWithProviders(<NotificationsProbe />);

    await waitFor(() => expect(text("count")).toBe("1"));
    expect(fetchMock).toHaveBeenCalled();
    expect(text("error")).toBe("false");
  });
});
