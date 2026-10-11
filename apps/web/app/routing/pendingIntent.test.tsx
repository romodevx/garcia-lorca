import { render, screen } from "@testing-library/react";
import { StrictMode, type ReactElement } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { clearIntent, handOffIntent, requestIntent, takeIntent, toIntent, useIntent } from "./pendingIntent";

/**
 * The shortcut handoff (redesign F1). The shell remounts on every new path, so
 * the intent ("open Egresos ON the new-expense drawer") lives in module scope
 * and the destination screen takes it once.
 */
describe("pendingIntent", () => {
  afterEach(clearIntent);

  it("hands the intent to the screen it was meant for, once", () => {
    requestIntent("expenses", "new");
    expect(takeIntent("expenses")).toBe("new");
    // A second mount (back, F5, a later visit) opens the plain screen.
    expect(takeIntent("expenses")).toBeUndefined();
  });

  it("never hands one module's intent to another screen", () => {
    requestIntent("expenses", "new");
    expect(takeIntent("scholarships")).toBeUndefined();
  });

  it("drops a stale intent when another screen mounts first", () => {
    // e.g. the shortcut's module was off and the stub showed; the next screen
    // that mounts must not leave it armed for a later, unrelated visit.
    requestIntent("expenses", "new");
    takeIntent("students");
    expect(takeIntent("expenses")).toBeUndefined();
  });

  it("is cleared by a plain open, so a later visit is a plain visit", () => {
    requestIntent("students", "search");
    clearIntent();
    expect(takeIntent("students")).toBeUndefined();
  });

  it("the newest request wins", () => {
    requestIntent("students", "new");
    requestIntent("students", "search");
    expect(takeIntent("students")).toBe("search");
  });

  describe("toIntent (route strings from ⌘K)", () => {
    it("accepts an intent the destination screen understands", () => {
      expect(toIntent("finance", "overdue")).toEqual({ module: "finance", intent: "overdue" });
      expect(toIntent("students", "search")).toEqual({ module: "students", intent: "search" });
    });

    it("rejects an intent the screen does not understand, or none at all", () => {
      expect(toIntent("finance", "payment")).toBeUndefined();
      expect(toIntent("announcements", "new")).toBeUndefined();
      expect(toIntent("students", undefined)).toBeUndefined();
    });

    it("never mistakes an object's built-ins for a module", () => {
      expect(toIntent("constructor", "new")).toBeUndefined();
      expect(toIntent("toString", "new")).toBeUndefined();
    });
  });

  describe("handOffIntent (the shell, on every open)", () => {
    it("arms an intent the screen understands", () => {
      handOffIntent("expenses", "new");
      expect(takeIntent("expenses")).toBe("new");
    });

    it("a plain open disarms a leftover", () => {
      requestIntent("expenses", "new");
      handOffIntent("expenses", undefined);
      expect(takeIntent("expenses")).toBeUndefined();
    });
  });

  describe("useIntent", () => {
    function Probe(): ReactElement {
      return <p>{useIntent("expenses") ?? "plain"}</p>;
    }

    // React calls a state initializer twice in development; the intent is taken
    // by the first call, so the screen must keep THAT result, not the second's.
    it("keeps the intent under StrictMode's double render", () => {
      requestIntent("expenses", "new");
      render(
        <StrictMode>
          <Probe />
        </StrictMode>,
      );
      expect(screen.getByText("new")).toBeInTheDocument();
    });
  });
});
