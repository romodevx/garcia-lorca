import { cleanup, render } from "@testing-library/react-native";
import { router, useGlobalSearchParams, usePathname, useSegments } from "expo-router";
import { type ReactElement } from "react";
import { clearTokens, saveTokens } from "@/src/api";
import { RETURN_TO_PARAM } from "@/src/navigation/routes";
import { useRequireSession } from "./useRequireSession";

jest.mock("expo-router", () => ({
  router: { push: jest.fn(), replace: jest.fn(), back: jest.fn(), navigate: jest.fn() },
  usePathname: jest.fn(),
  useSegments: jest.fn(),
  useGlobalSearchParams: jest.fn(),
}));

/** Mounts the hook under test; renders nothing itself. */
function Harness(): ReactElement | null {
  useRequireSession();
  return null;
}

async function openAt(pathname: string, segments: string[] = [], params: Record<string, string> = {}): Promise<void> {
  (usePathname as jest.Mock).mockReturnValue(pathname);
  (useSegments as jest.Mock).mockReturnValue(segments);
  (useGlobalSearchParams as jest.Mock).mockReturnValue(params);
  await render(<Harness />);
}

describe("useRequireSession", () => {
  beforeEach(() => {
    (router.replace as jest.Mock).mockClear();
  });
  afterEach(async () => {
    cleanup();
    await clearTokens();
  });

  it("sends a family without a session to the login screen, remembering where they were going", async () => {
    await openAt("/avisos");

    expect(router.replace).toHaveBeenCalledWith({ pathname: "/", params: { [RETURN_TO_PARAM]: "/avisos" } });
  });

  it("keeps the link's query, so a boleta still knows its student after the login", async () => {
    await openAt("/boleta/r1", ["boleta", "[id]"], { id: "r1", studentId: "s1" });

    expect(router.replace).toHaveBeenCalledWith({ pathname: "/", params: { [RETURN_TO_PARAM]: "/boleta/r1?studentId=s1" } });
  });

  it("leaves the login screen alone, so it never redirects to itself", async () => {
    await openAt("/");

    expect(router.replace).not.toHaveBeenCalled();
  });

  it("lets a family with a session stay on the route they opened", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });

    await openAt("/avisos");

    expect(router.replace).not.toHaveBeenCalled();
  });
});
