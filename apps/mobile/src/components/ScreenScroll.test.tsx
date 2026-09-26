import { act, screen } from "@testing-library/react-native";
import { type ReactElement } from "react";
import { Text } from "react-native";
import { renderWithProviders } from "../test/renderWithProviders";
import { useRefreshHandler } from "./refresh";
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
