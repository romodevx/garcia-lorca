/**
 * The activity form as typed (T17), and the one function that reads it into
 * the contract's body or says, field by field, what is wrong.
 *
 * Every value is the TEXT on screen: money stays in pesos until `readDraft`,
 * so nothing typed is ever silently turned into a zero.
 */
import { formatPesosInput, pesosToCents } from "../../_lib/money";
import type { Activity, ActivityInput, ActivityPriceOption, ActivitySlot, Periodicity } from "../../activities/api";

/** "flat" is the fee whatever the days; otherwise days per week, "1".."7". */
export type OptionKind = "" | "flat" | `${number}`;

export interface SlotDraft {
  /** React key only. */
  key: string;
  weekday: string;
  start: string;
  end: string;
}

export interface OptionDraft {
  key: string;
  /** The stored option's id: sent so the server edits it in place. */
  id?: string;
  kind: OptionKind;
  amount: string;
}

export interface ActivityDraft {
  name: string;
  slots: SlotDraft[];
  periodicity: Periodicity;
  billingStartsOn: string;
  firstDueOn: string;
  lateFee: string;
  active: boolean;
  options: OptionDraft[];
}

export const MAX_SLOTS = 21;
export const MAX_OPTIONS = 8;
export const MAX_NAME = 80;
export const FLAT: OptionKind = "flat";

const MISSING = "Falta este dato";

let seq = 0;
/** A fresh React key for a new row. */
export function rowKey(): string {
  seq += 1;
  return `row-${seq}`;
}

export function newSlot(): SlotDraft {
  return { key: rowKey(), weekday: "", start: "", end: "" };
}

export function newOption(): OptionDraft {
  return { key: rowKey(), kind: "", amount: "" };
}

export function emptyDraft(today: string): ActivityDraft {
  return {
    name: "",
    slots: [newSlot()],
    periodicity: "monthly",
    billingStartsOn: today,
    firstDueOn: "",
    lateFee: "",
    active: true,
    options: [newOption()],
  };
}

export function draftFromActivity(a: Activity): ActivityDraft {
  return {
    name: a.name,
    slots: a.schedule.map((s) => ({ key: rowKey(), weekday: String(s.weekday), start: s.start, end: s.end })),
    periodicity: a.periodicity,
    billingStartsOn: a.billingStartsOn,
    firstDueOn: a.firstDueOn,
    lateFee: a.lateFeeCents === 0 ? "" : formatPesosInput(a.lateFeeCents),
    active: a.active,
    options: a.options.map((o) => ({
      key: rowKey(),
      id: o.id,
      kind: o.daysPerWeek === null ? FLAT : (String(o.daysPerWeek) as OptionKind),
      amount: formatPesosInput(o.amountCents),
    })),
  };
}

export const slotKey = (i: number, field: "weekday" | "start" | "end"): string => `slot-${i}-${field}`;
export const optionKey = (i: number, field: "kind" | "amount"): string => `option-${i}-${field}`;

type Errors = Record<string, string>;

function readSlots(slots: readonly SlotDraft[], errors: Errors): ActivitySlot[] {
  const out: ActivitySlot[] = [];
  slots.forEach((s, i) => {
    if (s.weekday === "") errors[slotKey(i, "weekday")] = MISSING;
    if (s.start === "") errors[slotKey(i, "start")] = MISSING;
    if (s.end === "") errors[slotKey(i, "end")] = MISSING;
    if (s.start === "" || s.end === "" || s.weekday === "") return;
    if (s.end <= s.start) {
      errors[slotKey(i, "end")] = "La hora de fin va después del inicio.";
      return;
    }
    // HH:MM compares as text. Half-open: one class may start when another ends.
    const clash = slots.slice(0, i).some((o) => o.weekday === s.weekday && o.start !== "" && o.end !== "" && s.start < o.end && o.start < s.end);
    if (clash) errors[slotKey(i, "start")] = "Este horario se encima con otro del mismo día.";
    out.push({ weekday: Number(s.weekday), start: s.start, end: s.end });
  });
  return out;
}

const isBlankOption = (o: OptionDraft): boolean => o.kind === "" && o.amount.trim() === "";

/**
 * Untouched blank rows are skipped while another row has something (ISSUE-013):
 * "Agregar precio" opens one, and people press it as if it confirmed the price
 * above — the save then stopped on "Falta este dato" in a row nobody meant.
 * When EVERY row is blank, the first is still asked for: an activity needs a price.
 */
function readOptions(options: readonly OptionDraft[], errors: Errors): ActivityPriceOption[] {
  const out: ActivityPriceOption[] = [];
  const seen = new Set<string>();
  const someFilled = options.some((o) => !isBlankOption(o));
  options.forEach((o, i) => {
    if (isBlankOption(o) && (someFilled || i > 0)) return;
    if (o.kind === "") errors[optionKey(i, "kind")] = MISSING;
    else if (seen.has(o.kind)) errors[optionKey(i, "kind")] = o.kind === FLAT ? "Ya hay una cuota fija." : "Ya hay un precio para esos días.";
    else seen.add(o.kind);

    const cents = o.amount.trim() === "" ? Number.NaN : pesosToCents(o.amount);
    if (o.amount.trim() === "") errors[optionKey(i, "amount")] = MISSING;
    else if (Number.isNaN(cents)) errors[optionKey(i, "amount")] = "Escribe el monto en pesos, por ejemplo 600.00";
    else if (cents <= 0) errors[optionKey(i, "amount")] = "El monto debe ser mayor a cero.";
    if (o.kind === "" || Number.isNaN(cents) || cents <= 0) return;
    const option: ActivityPriceOption = { daysPerWeek: o.kind === FLAT ? null : Number(o.kind), amountCents: cents };
    out.push(o.id === undefined ? option : { id: o.id, ...option });
  });
  return out;
}

/**
 * The contract's body, or every problem at once keyed like the fields
 * (`name`, `slot-0-start`, `option-1-amount`…) so each shows under its input.
 */
export function readDraft(d: ActivityDraft): { input?: ActivityInput; errors: Errors } {
  const errors: Errors = {};
  const name = d.name.trim();
  if (name === "") errors.name = MISSING;
  else if (name.length > MAX_NAME) errors.name = `El nombre lleva a lo más ${MAX_NAME} letras.`;
  if (d.billingStartsOn === "") errors.billingStartsOn = MISSING;
  if (d.firstDueOn === "") errors.firstDueOn = MISSING;
  else if (d.billingStartsOn !== "" && d.firstDueOn < d.billingStartsOn) errors.firstDueOn = "La fecha límite no puede ser antes del primer cobro.";

  const lateFeeCents = d.lateFee.trim() === "" ? 0 : pesosToCents(d.lateFee);
  if (Number.isNaN(lateFeeCents)) errors.lateFee = "Escribe el recargo en pesos, o déjalo vacío para no cobrar recargo.";

  const schedule = readSlots(d.slots, errors);
  const options = readOptions(d.options, errors);
  if (Object.keys(errors).length > 0) return { errors };

  return {
    errors,
    input: {
      name,
      schedule,
      periodicity: d.periodicity,
      billingStartsOn: d.billingStartsOn,
      firstDueOn: d.firstDueOn,
      lateFeeCents,
      active: d.active,
      options,
    },
  };
}
