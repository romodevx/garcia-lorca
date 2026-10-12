import { render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppFlagsProvider } from "./flags-provider";
import { fetchFeatures } from "./features";

vi.mock("./features", () => ({ fetchFeatures: vi.fn() }));
vi.mock("./api-url", () => ({ API_URL: "http://gateway", IS_DESIGN_DEMO: false }));

const html = (): HTMLElement => document.documentElement;

afterEach(() => {
  localStorage.clear();
  html().removeAttribute("data-look");
  vi.mocked(fetchFeatures).mockReset();
});

/*
 * The skin is synced where the flags are fetched (review E7): AppFlagsProvider
 * lives in the root layout and survives navigation, and it covers pages that
 * never mount AppShell (/activar, /comprobantes).
 */
describe("AppFlagsProvider and the look", () => {
  it("applies the look once /features says module_ui_refresh is ON", async () => {
    vi.mocked(fetchFeatures).mockResolvedValue({ module_ui_refresh: true });
    render(<AppFlagsProvider>page</AppFlagsProvider>);
    await waitFor(() => expect(html().getAttribute("data-look")).toBe("refresh"));
  });

  it("clears a remembered look once /features says it is OFF", async () => {
    html().setAttribute("data-look", "refresh");
    vi.mocked(fetchFeatures).mockResolvedValue({ module_students: true });
    render(<AppFlagsProvider>page</AppFlagsProvider>);
    await waitFor(() => expect(html().hasAttribute("data-look")).toBe(false));
  });

  it("keeps the remembered look while /features is pending or unreachable", async () => {
    html().setAttribute("data-look", "refresh");
    vi.mocked(fetchFeatures).mockRejectedValue(new Error("offline"));
    render(<AppFlagsProvider>page</AppFlagsProvider>);
    await waitFor(() => expect(fetchFeatures).toHaveBeenCalled());
    await Promise.resolve();
    expect(html().getAttribute("data-look")).toBe("refresh");
  });
});
