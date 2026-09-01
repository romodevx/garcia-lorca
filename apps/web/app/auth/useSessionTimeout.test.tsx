import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useSessionTimeout } from "./useSessionTimeout";

function Harness({ onExpire, onKeepAlive }: { onExpire: () => void; onKeepAlive: () => void }) {
  useSessionTimeout({ onExpire, onKeepAlive });
  return <div>panel</div>;
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  vi.runOnlyPendingTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("useSessionTimeout", () => {
  // The school's rule: fifteen minutes without touching it and the session is
  // over. On a shared office computer, walking away has to end the session.
  it("closes the session after fifteen idle minutes", () => {
    const onExpire = vi.fn();
    render(<Harness onExpire={onExpire} onKeepAlive={() => {}} />);

    act(() => { vi.advanceTimersByTime(14 * 60 * 1000); });
    expect(onExpire).not.toHaveBeenCalled();

    act(() => { vi.advanceTimersByTime(60 * 1000 + 1000); });
    expect(onExpire).toHaveBeenCalledTimes(1);
  });

  // Working is not idling. Somebody capturing payments for an hour must never be
  // thrown out mid-entry.
  it("does not close a session somebody is using", () => {
    const onExpire = vi.fn();
    render(<Harness onExpire={onExpire} onKeepAlive={() => {}} />);

    for (let i = 0; i < 10; i++) {
      act(() => { vi.advanceTimersByTime(10 * 60 * 1000); });
      act(() => { document.dispatchEvent(new Event("click")); });
    }

    expect(onExpire).not.toHaveBeenCalled();
  });

  // The session is renewed while somebody is working, so the token never lapses
  // under them and nobody meets a login screen mid-payment.
  it("renews the session while it is being used", () => {
    const onKeepAlive = vi.fn();
    render(<Harness onExpire={() => {}} onKeepAlive={onKeepAlive} />);

    // A minute in, somebody does something — the clock and the mount instant
    // are the same tick under fake timers, so the activity has to land after it.
    act(() => { vi.advanceTimersByTime(60 * 1000); });
    act(() => { document.dispatchEvent(new Event("click")); });
    act(() => { vi.advanceTimersByTime(10 * 60 * 1000); });

    expect(onKeepAlive).toHaveBeenCalledTimes(1);
  });

  // Renewing on the mere passing of time would keep an empty office logged in
  // on the server, which is the thing this exists to stop.
  it("does not renew a session nobody is touching", () => {
    const onKeepAlive = vi.fn();
    render(<Harness onExpire={() => {}} onKeepAlive={onKeepAlive} />);

    act(() => { vi.advanceTimersByTime(14 * 60 * 1000); });

    expect(onKeepAlive).not.toHaveBeenCalled();
  });

  // And once it has expired, nothing brings it back but a login — not even
  // somebody sitting down and moving the mouse.
  it("stops renewing once the session is over", () => {
    const onKeepAlive = vi.fn();
    render(<Harness onExpire={() => {}} onKeepAlive={onKeepAlive} />);

    act(() => { vi.advanceTimersByTime(16 * 60 * 1000); });
    act(() => { document.dispatchEvent(new Event("click")); });
    act(() => { vi.advanceTimersByTime(30 * 60 * 1000); });

    expect(onKeepAlive).not.toHaveBeenCalled();
  });

  it("stops listening when the panel unmounts", () => {
    const onExpire = vi.fn();
    const { unmount } = render(<Harness onExpire={onExpire} onKeepAlive={() => {}} />);

    unmount();
    act(() => { vi.advanceTimersByTime(60 * 60 * 1000); });

    expect(onExpire).not.toHaveBeenCalled();
  });
});
