import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useSessionKeepAlive } from "./useSessionKeepAlive";

function Harness({ onKeepAlive }: { onKeepAlive: () => void }) {
  useSessionKeepAlive({ onKeepAlive });
  return <div>panel</div>;
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  vi.runOnlyPendingTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("useSessionKeepAlive", () => {
  // The session is renewed while somebody is working, so the token never lapses
  // under them and nobody meets a login screen mid-payment.
  it("renews the session while it is being used", () => {
    const onKeepAlive = vi.fn();
    render(<Harness onKeepAlive={onKeepAlive} />);

    // A minute in, somebody does something — the clock and the mount instant
    // are the same tick under fake timers, so the activity has to land after it.
    act(() => { vi.advanceTimersByTime(60 * 1000); });
    act(() => { document.dispatchEvent(new Event("click")); });
    act(() => { vi.advanceTimersByTime(10 * 60 * 1000); });

    expect(onKeepAlive).toHaveBeenCalledTimes(1);
  });

  // Renewing on the mere passing of time would hold the session open on the
  // server for an office nobody is in, and the day-long window is what the
  // server grants — this is the half that decides whether it is being used.
  it("does not renew a session nobody is touching", () => {
    const onKeepAlive = vi.fn();
    render(<Harness onKeepAlive={onKeepAlive} />);

    act(() => { vi.advanceTimersByTime(14 * 60 * 1000); });

    expect(onKeepAlive).not.toHaveBeenCalled();
  });

  // THE POINT OF THE WHOLE CHANGE. A panel left open and untouched all night
  // used to be dead by morning: a watchdog in this hook ended the session at
  // fifteen idle minutes and latched, so nothing brought it back but a login.
  // The window is now the server's day, and the browser does not get a vote.
  it("leaves a panel that sat untouched all day still alive", () => {
    const onKeepAlive = vi.fn();
    render(<Harness onKeepAlive={onKeepAlive} />);

    // Nobody there, for far longer than the old fifteen minutes.
    act(() => { vi.advanceTimersByTime(12 * 60 * 60 * 1000); });
    expect(onKeepAlive).not.toHaveBeenCalled();

    // Somebody sits down. The session renews, which it could not do if an
    // expiry had latched while they were gone.
    act(() => { document.dispatchEvent(new Event("click")); });
    act(() => { vi.advanceTimersByTime(10 * 60 * 1000); });

    expect(onKeepAlive).toHaveBeenCalledTimes(1);
  });

  it("stops listening when the panel unmounts", () => {
    const onKeepAlive = vi.fn();
    const { unmount } = render(<Harness onKeepAlive={onKeepAlive} />);

    act(() => { document.dispatchEvent(new Event("click")); });
    unmount();
    act(() => { vi.advanceTimersByTime(60 * 60 * 1000); });

    expect(onKeepAlive).not.toHaveBeenCalled();
  });
});
