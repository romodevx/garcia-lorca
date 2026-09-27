import { act, renderHook } from "@testing-library/react-native";
import { AppState as RNAppState, type AppStateStatus } from "react-native";
import { requestFreshData, useForegroundRefresh, useFreshDataSignal } from "./freshness";

describe("freshness signal", () => {
  it("runs every subscribed reload when fresh data is requested, and stops after unmount", async () => {
    const reload = jest.fn(async () => undefined);
    const { unmount } = await renderHook(() => useFreshDataSignal(reload));

    await act(async () => requestFreshData());
    expect(reload).toHaveBeenCalledTimes(1);

    await unmount();
    await act(async () => requestFreshData());
    expect(reload).toHaveBeenCalledTimes(1);
  });
});

describe("idle subscribers", () => {
  it("does not subscribe while inactive (an idle resource has nothing to reload)", async () => {
    const reload = jest.fn(async () => undefined);
    const { rerender } = await renderHook(({ active }: { active: boolean }) => useFreshDataSignal(reload, active), {
      initialProps: { active: false },
    });

    await act(async () => requestFreshData());
    expect(reload).not.toHaveBeenCalled();

    await rerender({ active: true });
    await act(async () => requestFreshData());
    expect(reload).toHaveBeenCalledTimes(1);
  });
});

describe("useForegroundRefresh", () => {
  let listener: ((state: AppStateStatus) => void) | null = null;
  const remove = jest.fn();

  beforeEach(() => {
    listener = null;
    jest.spyOn(RNAppState, "addEventListener").mockImplementation((_type, handler) => {
      listener = handler as (state: AppStateStatus) => void;
      return { remove } as unknown as ReturnType<typeof RNAppState.addEventListener>;
    });
  });
  afterEach(() => jest.restoreAllMocks());

  it("requests fresh data when the app comes back to the foreground, not on every state change", async () => {
    const reload = jest.fn(async () => undefined);
    await renderHook(() => {
      useForegroundRefresh();
      useFreshDataSignal(reload);
    });

    await act(async () => listener?.("background"));
    expect(reload).not.toHaveBeenCalled();

    await act(async () => listener?.("active"));
    expect(reload).toHaveBeenCalledTimes(1);

    // Already active: a repeated "active" is not a return to the foreground.
    await act(async () => listener?.("active"));
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("removes its AppState subscription on unmount", async () => {
    const { unmount } = await renderHook(() => useForegroundRefresh());
    await unmount();
    expect(remove).toHaveBeenCalled();
  });
});
