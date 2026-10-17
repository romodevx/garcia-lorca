import AsyncStorage from "@react-native-async-storage/async-storage";
import { act, fireEvent, screen, waitFor } from "@testing-library/react-native";
import { type ReactElement } from "react";
import { Pressable, Text } from "react-native";
import { clearTokens, saveTokens } from "../api";
import { renderWithProviders } from "../test/renderWithProviders";
import { useAppState } from "./AppState";
import { requestFreshData } from "./freshness";

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

/** Surfaces the child slice as plain text for assertions. */
function ChildProbe(): ReactElement {
  const { children, child, childLoading, childError } = useAppState();
  return (
    <>
      <Text testID="childCount">{String(children.length)}</Text>
      <Text testID="selected">{child?.id ?? "none"}</Text>
      <Text testID="childLoading">{String(childLoading)}</Text>
      <Text testID="childError">{String(childError)}</Text>
    </>
  );
}

const text = (id: string): unknown => screen.getByTestId(id).props.children;

/** Routes the family fetch by path so both slices get well-formed envelopes. */
function mockFamilyFetch(students: unknown[]): jest.Mock {
  const fetchMock = jest.fn(async (url: string) => {
    const body = String(url).includes("/family/students") ? { students } : { notifications: [] };
    return { ok: true, status: 200, json: async () => body };
  });
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  return fetchMock;
}

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

describe("AppState child slice", () => {
  afterEach(async () => {
    await clearTokens();
  });

  it("no-ops to an empty, non-loading, non-error slice when signed out (no token)", async () => {
    await clearTokens();
    const fetchMock = jest.fn();
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await renderWithProviders(<ChildProbe />);

    await waitFor(() => expect(text("childLoading")).toBe("false"));
    expect(text("childCount")).toBe("0");
    expect(text("selected")).toBe("none");
    expect(text("childError")).toBe("false");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("bootstraps the linked children and selects the first when signed in", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    mockFamilyFetch([
      { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "primaria", gradeGroup: "3° A" },
      { id: "s2", firstName: "Sofía", lastName: "Herrera", level: "kinder", gradeGroup: "K3" },
    ]);

    await renderWithProviders(<ChildProbe />);

    await waitFor(() => expect(text("childCount")).toBe("2"));
    expect(text("selected")).toBe("s1");
    expect(text("childError")).toBe("false");
  });

  it("flags an error when the child load fails", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    globalThis.fetch = jest
      .fn()
      .mockResolvedValue({ ok: false, status: 500, json: async () => ({}) }) as unknown as typeof fetch;

    await renderWithProviders(<ChildProbe />);

    await waitFor(() => expect(text("childError")).toBe("true"));
    expect(text("selected")).toBe("none");
  });
});

/** Selects a child by id, like the header's switcher does. */
function SelectProbe({ id }: { id: string }): ReactElement {
  const { selectChild } = useAppState();
  return (
    <Pressable accessibilityRole="button" accessibilityLabel={`select ${id}`} onPress={() => selectChild(id)}>
      <Text>select</Text>
    </Pressable>
  );
}

// ISSUE-CC-303: the parent's choice of child survives a relaunch.
describe("AppState remembers the selected child", () => {
  const siblings = [
    { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "primaria", gradeGroup: "3° A" },
    { id: "s2", firstName: "Sofía", lastName: "Herrera", level: "kinder", gradeGroup: "K3" },
  ];

  afterEach(async () => {
    await clearTokens();
    await AsyncStorage.clear();
  });

  it("restores the child chosen before the app was closed", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    mockFamilyFetch(siblings);
    const first = await renderWithProviders(
      <>
        <ChildProbe />
        <SelectProbe id="s2" />
      </>,
    );
    await waitFor(() => expect(text("selected")).toBe("s1"));
    fireEvent.press(screen.getByRole("button", { name: "select s2" }));
    await waitFor(() => expect(text("selected")).toBe("s2"));
    await first.unmount();

    await renderWithProviders(<ChildProbe />);

    await waitFor(() => expect(text("childCount")).toBe("2"));
    await waitFor(() => expect(text("selected")).toBe("s2"));
  });

  it("falls back to the first child when the remembered one is no longer linked", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    await AsyncStorage.setItem("gl.selectedChildId", "gone");
    mockFamilyFetch(siblings);

    await renderWithProviders(<ChildProbe />);

    await waitFor(() => expect(text("childCount")).toBe("2"));
    expect(text("selected")).toBe("s1");
  });
});

describe("AppState freshness", () => {
  afterEach(async () => {
    await clearTokens();
  });

  it("refetches the inbox when fresh data is requested (foreground / attendance push)", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    const fetchMock = jest.fn(async () => ({ ok: true, status: 200, json: async () => ({ notifications: [], students: [] }) }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await renderWithProviders(<NotificationsProbe />);
    await waitFor(() => expect(screen.getByTestId("loading").props.children).toBe("false"));
    const inboxCalls = (): number =>
      fetchMock.mock.calls.filter((c) => String((c as unknown[])[0]).endsWith("/family/notifications")).length;
    const before = inboxCalls();

    await act(async () => requestFreshData());

    expect(inboxCalls()).toBe(before + 1);
  });
});
