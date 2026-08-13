import { act, cleanup } from "@testing-library/react-native";
import { router } from "expo-router";
import { type ReactElement } from "react";
import { emitSessionLost } from "@/src/api";
import { renderWithProviders } from "@/src/test/renderWithProviders";
import { useSessionLostRedirect } from "./useSessionLostRedirect";

/** Mounts the hook under test; renders nothing itself. */
function Harness(): ReactElement | null {
  useSessionLostRedirect();
  return null;
}

describe("useSessionLostRedirect", () => {
  // Auto-cleanup is off in this project, so unmount any mounted tree between
  // tests — otherwise a prior Harness stays subscribed and leaks its redirect.
  afterEach(cleanup);

  beforeEach(() => {
    (router.replace as jest.Mock).mockClear();
  });

  it("forces navigation back to Login when the session is lost", async () => {
    await renderWithProviders(<Harness />);

    await act(async () => {
      emitSessionLost();
    });

    expect(router.replace).toHaveBeenCalledWith("/");
  });

  it("stops reacting once unmounted (subscription is cleaned up)", async () => {
    const { unmount } = await renderWithProviders(<Harness />);
    await act(async () => {
      unmount();
    });

    await act(async () => {
      emitSessionLost();
    });

    expect(router.replace).not.toHaveBeenCalled();
  });
});
