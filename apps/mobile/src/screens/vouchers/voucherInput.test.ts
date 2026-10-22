import { VoucherSendError } from "../../api";
import { dayInputOf, parseAmountCents, parseDayInput, schoolToday, sendErrorMessage } from "./voucherInput";

describe("parseAmountCents (with or without separators)", () => {
  it.each([
    ["2500", 250000],
    ["2,500", 250000],
    ["2,500.50", 250050],
    ["$2,500.5", 250050],
    [" 2500.00 ", 250000],
    ["2500,50", 250050],
    ["1,234,567", 123456700],
    ["0.5", 50],
  ])("reads %s as %d cents", (typed, cents) => {
    expect(parseAmountCents(typed)).toBe(cents);
  });

  it("reads an empty field as not said", () => {
    expect(parseAmountCents("  ")).toBeNull();
  });

  it.each(["abc", "0", "-20", "2.5.5", "1.234"])("refuses %s", (typed) => {
    expect(parseAmountCents(typed)).toBe("invalid");
  });
});

describe("the school's day (Morelia, ADR 0035)", () => {
  it("is still yesterday in Morelia at 03:00 UTC", () => {
    expect(schoolToday(new Date("2026-10-21T03:00:00Z"))).toBe("2026-10-20");
  });

  it("is today once Morelia passes midnight", () => {
    expect(schoolToday(new Date("2026-10-21T06:00:00Z"))).toBe("2026-10-21");
  });
});

describe("the payment day field", () => {
  const today = "2026-10-21";

  it("shows a day as the family writes it", () => {
    expect(dayInputOf("2026-10-05")).toBe("05/10/2026");
  });

  it.each([
    ["21/10/2026", "2026-10-21"],
    ["5/10/2026", "2026-10-05"],
    ["05-10-2026", "2026-10-05"],
    ["05102026", "2026-10-05"],
  ])("reads %s", (typed, iso) => {
    expect(parseDayInput(typed, today)).toBe(iso);
  });

  it("reads an empty field as not said", () => {
    expect(parseDayInput("", today)).toBeNull();
  });

  it.each([["31/02/2026"], ["hoy"], ["2026"]])("refuses %s as no date", (typed) => {
    expect(parseDayInput(typed, today)).toBe("invalid");
  });

  it("refuses a day after today", () => {
    expect(parseDayInput("22/10/2026", today)).toBe("future");
  });
});

describe("sendErrorMessage", () => {
  it.each([
    ["too_large", "El archivo pesa más de 5 MB. Elige una foto más ligera o un PDF más pequeño."],
    ["limit", "Ya tienes 3 comprobantes por revisar para este alumno. Espera a que la escuela los revise."],
    ["network", "No pudimos enviar el comprobante. Revisa tu conexión e inténtalo de nuevo."],
  ] as const)("words %s for a family", (reason, message) => {
    expect(sendErrorMessage(new VoucherSendError(reason))).toBe(message);
  });

  it("says how long to wait when the school limits the uploads", () => {
    expect(sendErrorMessage(new VoucherSendError("rate_limited", 120))).toBe(
      "Enviaste varios comprobantes seguidos. Espera 2 minutos e inténtalo de nuevo.",
    );
    expect(sendErrorMessage(new VoucherSendError("rate_limited"))).toBe(
      "Enviaste varios comprobantes seguidos. Espera unos minutos e inténtalo de nuevo.",
    );
  });

  it("treats anything unexpected as a server problem", () => {
    expect(sendErrorMessage(new Error("boom"))).toBe("La escuela no pudo recibir el comprobante en este momento. Inténtalo más tarde.");
  });
});
