import type { FamilyNotification } from "../api";
import { isUnread, notificationKind } from "./notificationView";

function make(overrides: Partial<FamilyNotification>): FamilyNotification {
  return {
    id: "n1",
    studentId: "s1",
    type: "attendance",
    title: "T",
    body: "B",
    createdAt: "2026-08-12T10:00:00Z",
    readAt: "",
    ...overrides,
  };
}

describe("notificationKind", () => {
  it("maps known comms types to display meta", () => {
    expect(notificationKind("payment").label).toBe("Pagos");
    expect(notificationKind("attendance").label).toBe("Asistencia");
    expect(notificationKind("absence").label).toBe("Asistencia");
  });

  it("falls back to a neutral school kind for unknown types", () => {
    const meta = notificationKind("something-new");
    expect(meta.label).toBe("Escuela");
    expect(meta.icon).toBeTruthy();
    expect(meta.tint).toBeTruthy();
  });
});

describe("isUnread", () => {
  const noneRead: ReadonlySet<string> = new Set();

  it("is unread when readAt is empty and not locally read", () => {
    expect(isUnread(make({ id: "a", readAt: "" }), noneRead)).toBe(true);
  });

  it("is read when the server marked it read", () => {
    expect(isUnread(make({ readAt: "2026-08-12T11:00:00Z" }), noneRead)).toBe(false);
  });

  it("is read when it was marked locally", () => {
    expect(isUnread(make({ id: "a", readAt: "" }), new Set(["a"]))).toBe(false);
  });
});
