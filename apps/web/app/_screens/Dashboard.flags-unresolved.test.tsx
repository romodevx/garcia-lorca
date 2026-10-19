import { render, screen } from "@testing-library/react";
import type { FlagSet } from "@garcia-lorca/ui/flags";
import type { ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { USERS } from "../_design/data";
import { listStudents } from "../students/api";
import { FlagsResolvedProvider } from "../flags-resolved";
import { ALL_MODULES_ON } from "../home/testing/renderHome";
import { LOOK_STORAGE_KEY } from "../look/look";
import { Providers } from "../test-utils";
import { Dashboard } from "./Dashboard";

vi.mock("../students/api", () => ({ listStudents: vi.fn().mockResolvedValue([]) }));
vi.mock("../config/api", () => ({ listCycles: vi.fn().mockResolvedValue([]) }));
vi.mock("../finance/api", () => ({ listCharges: vi.fn().mockResolvedValue([]) }));

/*
 * Review I1: until /features answers, the flags are the OFF fallback while the
 * skin is already the remembered one (pre-paint). The home must give the same
 * answer as the skin, or the refreshed skin dresses the classic home and then
 * swaps it — re-firing every KPI read — or never swaps if the gateway is down.
 */

const NOW = new Date("2026-06-12T09:00:00");
const PERMS = ["students:read", "config:read", "finance:read"] as const;
const html = (): HTMLElement => document.documentElement;

function home(resolved: boolean, flags: FlagSet): ReactElement {
  return (
    <Providers flags={flags}>
      <FlagsResolvedProvider resolved={resolved}>
        <Dashboard user={USERS.admin} onOpen={() => {}} permissions={PERMS} now={NOW} />
      </FlagsResolvedProvider>
    </Providers>
  );
}

const isRefreshed = (): boolean => document.querySelector(".home") !== null;
const isClassic = (): boolean => screen.queryByText("Todo el sistema, a la vista") !== null;

beforeEach(() => vi.mocked(listStudents).mockClear());

afterEach(() => {
  localStorage.clear();
  html().removeAttribute("data-look");
});

describe("Dashboard before /features answers", () => {
  it("shows the refreshed home when the remembered look is refresh, and keeps it (no second read) once the flag confirms", () => {
    localStorage.setItem(LOOK_STORAGE_KEY, "refresh");
    html().setAttribute("data-look", "refresh");

    const { rerender } = render(home(false, {}));
    expect(isRefreshed()).toBe(true);
    expect(isClassic()).toBe(false);

    rerender(home(true, { module_ui_refresh: true }));
    expect(isRefreshed()).toBe(true);
    expect(listStudents).toHaveBeenCalledTimes(1);
  });

  it("shows the classic home when no look is remembered", () => {
    render(home(false, {}));
    expect(isClassic()).toBe(true);
    expect(isRefreshed()).toBe(false);
  });

  it("follows the flag, not the memory, once resolved", () => {
    localStorage.setItem(LOOK_STORAGE_KEY, "refresh");
    html().setAttribute("data-look", "refresh");
    render(home(true, ALL_MODULES_ON));
    expect(isClassic()).toBe(true);
  });

  // C2-013: the cards follow the flags, and until /features answers every flag
  // reads OFF. Waiting must look like waiting, not like "no modules for you".
  it("never says there are no modules while the flags are still on their way", () => {
    render(home(false, {}));
    expect(screen.queryByText("No tienes módulos asignados")).toBeNull();
  });
});
