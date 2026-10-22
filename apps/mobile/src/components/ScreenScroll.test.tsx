import { act, screen, waitFor } from "@testing-library/react-native";
import { type ReactElement } from "react";
import { StyleSheet, Text } from "react-native";
import { clearTokens, saveTokens } from "../api";
import { demoFlags } from "../flags/bootstrap";
import { renderWithProviders } from "../test/renderWithProviders";
import { useRefreshHandler } from "./refresh";
import { FAB_CLEARANCE } from "./CardFab";
import { ScreenScroll } from "./ScreenScroll";

interface Deferred {
  promise: Promise<void>;
  resolve: () => void;
}
function deferred(): Deferred {
  let resolve: () => void = () => undefined;
  const promise = new Promise<void>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

type RefreshProps = { refreshing: boolean; onRefresh: () => void };
const refreshControl = (): RefreshProps | undefined =>
  (screen.getByTestId("screen-scroll").props.refreshControl as { props: RefreshProps } | undefined)?.props;

function Registers({ handler }: { handler: () => Promise<void> }): ReactElement {
  useRefreshHandler(handler);
  return <Text>contenido</Text>;
}

describe("ScreenScroll pull-to-refresh", () => {
  it("shows the spinner until the refresh actually resolves (no timer)", async () => {
    const pending = deferred();
    const onRefresh = jest.fn(() => pending.promise);
    await renderWithProviders(
      <ScreenScroll onRefresh={onRefresh}>
        <Text>contenido</Text>
      </ScreenScroll>,
    );

    await act(async () => refreshControl()?.onRefresh());
    expect(onRefresh).toHaveBeenCalledTimes(1);
    expect(refreshControl()?.refreshing).toBe(true);

    await act(async () => pending.resolve());
    expect(refreshControl()?.refreshing).toBe(false);
  });

  it("runs the handlers the screen's content registered", async () => {
    const handler = jest.fn(async () => undefined);
    await renderWithProviders(
      <ScreenScroll>
        <Registers handler={handler} />
      </ScreenScroll>,
    );

    await act(async () => refreshControl()?.onRefresh());
    expect(handler).toHaveBeenCalledTimes(1);
    expect(refreshControl()?.refreshing).toBe(false);
  });

  it("settles the spinner even when a refresh fails", async () => {
    const onRefresh = jest.fn(async () => {
      throw new Error("offline");
    });
    await renderWithProviders(
      <ScreenScroll onRefresh={onRefresh}>
        <Text>contenido</Text>
      </ScreenScroll>,
    );

    await act(async () => refreshControl()?.onRefresh());
    expect(refreshControl()?.refreshing).toBe(false);
  });

  it("offers no pull-to-refresh when there is nothing to reload (never a fake spinner)", async () => {
    await renderWithProviders(
      <ScreenScroll>
        <Text>contenido</Text>
      </ScreenScroll>,
    );
    expect(refreshControl()).toBeUndefined();
  });
});

// The credencial button floats over the tab content; the last row must be
// scrollable out from under it.
describe("ScreenScroll bottom clearance", () => {
  const paddingBottom = (): number =>
    StyleSheet.flatten(screen.getByTestId("screen-scroll").props.contentContainerStyle).paddingBottom as number;

  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    globalThis.fetch = jest.fn(async (url: string) => {
      const students = [{ id: "s1", firstName: "Zoe", lastName: "Villalobos", level: "Primaria", gradeGroup: "1° A" }];
      const body = String(url).endsWith("/family/students") ? { students } : { notifications: [] };
      return { ok: true, status: 200, json: async () => body };
    }) as unknown as typeof fetch;
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("leaves room for the card button when it shows", async () => {
    await renderWithProviders(<ScreenScroll><Text>contenido</Text></ScreenScroll>, {
      flags: { ...demoFlags, module_student_card: true },
    });

    await waitFor(() => expect(paddingBottom()).toBeGreaterThanOrEqual(FAB_CLEARANCE));
  });

  it("keeps the standard inset when there is no card button", async () => {
    await renderWithProviders(<ScreenScroll><Text>contenido</Text></ScreenScroll>, {
      flags: { ...demoFlags, module_student_card: false },
    });

    await act(async () => undefined);
    expect(paddingBottom()).toBe(40);
  });
});
