import { describe, expect, it } from "vitest";

import type { Activity } from "../../activities/api";
import { draftFromActivity, emptyDraft, readDraft, type ActivityDraft } from "./activityDraft";

const OPT = "0f000000-0000-0000-0000-000000000001";

function valid(): ActivityDraft {
  return {
    ...emptyDraft("2026-10-01"),
    name: "Natación",
    slots: [{ key: "s1", weekday: "2", start: "15:00", end: "16:30" }, { key: "s2", weekday: "4", start: "16:00", end: "17:00" }],
    firstDueOn: "2026-10-10",
    options: [
      { key: "o1", kind: "1", amount: "600.00" },
      { key: "o2", kind: "flat", amount: "900" },
    ],
  };
}

describe("emptyDraft", () => {
  it("starts monthly, active, without late fee, billing from today, with one slot and one option", () => {
    const d = emptyDraft("2026-10-02");
    expect(d.periodicity).toBe("monthly");
    expect(d.active).toBe(true);
    expect(d.lateFee).toBe("");
    expect(d.billingStartsOn).toBe("2026-10-02");
    expect(d.slots).toHaveLength(1);
    expect(d.options).toHaveLength(1);
  });
});

describe("readDraft", () => {
  it("turns a valid draft into the contract's body, pesos to cents", () => {
    const got = readDraft(valid());
    expect(got.errors).toEqual({});
    expect(got.input).toEqual({
      name: "Natación",
      schedule: [{ weekday: 2, start: "15:00", end: "16:30" }, { weekday: 4, start: "16:00", end: "17:00" }],
      periodicity: "monthly",
      billingStartsOn: "2026-10-01",
      firstDueOn: "2026-10-10",
      lateFeeCents: 0,
      active: true,
      options: [{ daysPerWeek: 1, amountCents: 60000 }, { daysPerWeek: null, amountCents: 90000 }],
    });
  });

  it("sends the id of an existing option so it is edited in place", () => {
    const d = valid();
    d.options = [{ key: "o1", id: OPT, kind: "1", amount: "650" }];
    expect(readDraft(d).input?.options).toEqual([{ id: OPT, daysPerWeek: 1, amountCents: 65000 }]);
  });

  it("marks every missing field at once, by its key", () => {
    const d = { ...emptyDraft("2026-10-01"), billingStartsOn: "" };
    const { errors, input } = readDraft(d);
    expect(input).toBeUndefined();
    expect(Object.keys(errors).sort()).toEqual(["billingStartsOn", "firstDueOn", "name", "option-0-amount", "option-0-kind", "slot-0-end", "slot-0-start", "slot-0-weekday"].sort());
  });

  it("refuses an end before the start, and two overlapping slots on one day", () => {
    const d = valid();
    d.slots = [
      { key: "a", weekday: "2", start: "15:00", end: "14:00" },
      { key: "b", weekday: "3", start: "15:00", end: "16:00" },
      { key: "c", weekday: "3", start: "15:30", end: "16:30" },
    ];
    const { errors } = readDraft(d);
    expect(errors["slot-0-end"]).toMatch(/después del inicio/);
    expect(errors["slot-2-start"]).toMatch(/se encima/);
  });

  it("refuses two options for the same days and a second flat fee", () => {
    const d = valid();
    d.options = [
      { key: "a", kind: "2", amount: "1" },
      { key: "b", kind: "2", amount: "2" },
      { key: "c", kind: "flat", amount: "3" },
      { key: "d", kind: "flat", amount: "4" },
    ];
    const { errors } = readDraft(d);
    expect(errors["option-1-kind"]).toMatch(/ya hay/i);
    expect(errors["option-3-kind"]).toMatch(/ya hay/i);
  });

  it("refuses an amount that is not pesos, a zero amount and an unreadable late fee", () => {
    const d = valid();
    d.options = [{ key: "a", kind: "1", amount: "1850,50" }, { key: "b", kind: "2", amount: "0" }];
    d.lateFee = "abc";
    const { errors } = readDraft(d);
    expect(errors["option-0-amount"]).toBeDefined();
    expect(errors["option-1-amount"]).toBeDefined();
    expect(errors.lateFee).toBeDefined();
  });

  // ISSUE-013: "Agregar precio" opens a blank row. A price already typed and
  // then saved must go through, not stop on "Falta este dato" in an empty row.
  it("ignores an untouched blank price row when another price is filled", () => {
    const d = valid();
    d.options = [{ key: "a", kind: "1", amount: "600" }, { key: "b", kind: "", amount: " " }];
    const got = readDraft(d);
    expect(got.errors).toEqual({});
    expect(got.input?.options).toEqual([{ daysPerWeek: 1, amountCents: 60000 }]);
  });

  it("keeps the error keys of the rows that are not blank", () => {
    const d = valid();
    d.options = [{ key: "a", kind: "", amount: "" }, { key: "b", kind: "2", amount: "" }];
    expect(Object.keys(readDraft(d).errors)).toEqual(["option-1-amount"]);
  });

  it("a half-filled price row is still an error", () => {
    const d = valid();
    d.options = [{ key: "a", kind: "1", amount: "600" }, { key: "b", kind: "", amount: "700" }];
    expect(readDraft(d).errors["option-1-kind"]).toBeDefined();
  });

  it("with every price row blank, the first one is asked for", () => {
    const d = valid();
    d.options = [{ key: "a", kind: "", amount: "" }, { key: "b", kind: "", amount: "" }];
    expect(Object.keys(readDraft(d).errors).sort()).toEqual(["option-0-amount", "option-0-kind"]);
  });

  it("refuses a first due date before the first billing date", () => {
    const d = { ...valid(), firstDueOn: "2026-09-30" };
    expect(readDraft(d).errors.firstDueOn).toMatch(/antes/);
  });

  it("takes a late fee in pesos", () => {
    expect(readDraft({ ...valid(), lateFee: "50" }).input?.lateFeeCents).toBe(5000);
  });
});

describe("draftFromActivity", () => {
  it("round-trips an activity into the form and back", () => {
    const activity: Activity = {
      id: "a1", enrolledCount: 2, name: "Natación", periodicity: "weekly",
      schedule: [{ weekday: 2, start: "15:00", end: "16:30" }],
      billingStartsOn: "2026-10-01", firstDueOn: "2026-10-03", lateFeeCents: 5000, active: false,
      options: [{ id: OPT, daysPerWeek: null, amountCents: 90000 }],
    };
    const { id: _id, enrolledCount: _n, ...input } = activity;
    expect(readDraft(draftFromActivity(activity)).input).toEqual(input);
  });

  it("opens money with thousands separators, as the field shows it after blur (T12)", () => {
    const activity: Activity = {
      id: "a1", enrolledCount: 0, name: "Robótica", periodicity: "monthly",
      schedule: [], billingStartsOn: "2026-10-01", firstDueOn: "2026-10-03", lateFeeCents: 125000, active: true,
      options: [{ id: OPT, daysPerWeek: null, amountCents: 120000 }],
    };
    const draft = draftFromActivity(activity);
    expect(draft.lateFee).toBe("1,250.00");
    expect(draft.options[0].amount).toBe("1,200.00");
  });
});
