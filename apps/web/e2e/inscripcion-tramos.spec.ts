import { expect, test, type APIRequestContext } from "@playwright/test";

import { API_URL, apiAsOwner, uniqueSuffix } from "./helpers";

/**
 * The school's printed price list, end to end.
 *
 *   Concepto                 Ene/Feb   Marzo   Abr-Sep
 *   Inscripción              $2,200   $3,300   $4,400
 *   Gastos adm y seguro      $4,700   $4,700   $4,700
 *   Cuota Asociación         $  850   $  850   $  850
 *   TOTAL A PAGAR            $7,750   $8,850   $9,950
 *
 * These tests walk a real pupil through a real stack: the ladder is typed in one
 * service, stored as JSON, issued as charges by the run, re-priced by a later
 * run, and frozen by the first peso. Every layer below has its own tests and
 * none can prove the seams between them — a number right in four places and
 * wrong at a seam is exactly what a school finds out in June.
 *
 * Deliberately NOT reimplemented here. An earlier version of this file computed
 * the totals in the spec and compared them against the plan it had just sent,
 * which proved only that JSON survives a round trip: a test that brings its own
 * copy of the rule cannot fail when the rule is wrong.
 *
 * ONE seam is out of reach from outside. The QUOTE — `GET /finance/accounts/{id}`
 * and the payment window — prices at the SERVER's clock, which no request can
 * move, so "consult in March" cannot be staged from here. That path is covered
 * where the clock is injectable, by TestWhatIsQuotedIsWhatTheLadderSaysToday in
 * services/finance/internal/usecase. Everything the RUN reaches is here, because
 * RunBilling takes the date it should pretend it is.
 *
 * WRITE tests: they build their own cycle, level, group and pupil.
 * See ADR 0011 and .claude/docs/PLAN-m18-inscripcion-por-tramos.md.
 */

const TIER_JAN = 220000;
const TIER_MAR = 330000;
const TIER_APR = 440000;
const ADMIN_FEE = 470000;
const PARENTS_FEE = 85000;

const TOTAL_JAN = TIER_JAN + ADMIN_FEE + PARENTS_FEE; // 7,750
const TOTAL_MAR = TIER_MAR + ADMIN_FEE + PARENTS_FEE; // 8,850

const LINES = ["Inscripción", "Gastos adm y seguro", "Cuota Asociación Padres de Familia"];

interface School {
  cycleId: string;
  levelId: string;
  studentId: string;
}

/** The ladder as the panel would save it, plus the two lines that never move. */
function flyerConcepts(): unknown[] {
  const flat = (name: string, amountCents: number): unknown => ({
    name,
    tiers: [{ effectiveFrom: "2026-08-01", amountCents }],
  });
  return [
    {
      name: "Inscripción",
      tiers: [
        { effectiveFrom: "2026-08-01", amountCents: TIER_JAN },
        { effectiveFrom: "2027-03-01", amountCents: TIER_MAR },
        { effectiveFrom: "2027-04-01", amountCents: TIER_APR },
      ],
    },
    flat("Gastos adm y seguro", ADMIN_FEE),
    flat("Cuota Asociación Padres de Familia", PARENTS_FEE),
  ];
}

/** Everything a tuition table needs beyond its inscription. */
function planShell(cycleId: string, levelId: string, concepts: unknown[]): unknown {
  return {
    cycleId,
    levelId,
    // One shape or the other, never both (ADR 0011).
    enrollmentCents: 0,
    enrollmentConcepts: concepts,
    monthlyCents: 320000,
    months: 10,
    firstBilledMonth: 9,
    dueDay: 10,
    lateFeeCents: 0,
    monthlyDiscountCents: 0,
    schedule: [],
    fees: [],
    active: true,
    enrollmentDueOn: "",
    policy: {
      dailyMode: false,
      lateFeeDailyCents: 0,
      surchargeCountsAllDays: true,
      earlyPaymentCents: 0,
      earlyPaymentDays: 0,
    },
  };
}

/**
 * A CURP with the right SHAPE — four letters, a date, a sex, a state, three
 * consonants and two trailing characters — which is what the students boundary
 * validates. Not a real one, and it does not pretend to be; the random middle is
 * what keeps two runs of this spec from colliding.
 */
function shapedCurp(): string {
  const consonants = "BCDFGHJKLMNPQRSTVWXYZ";
  const pick = (from: string): string => from[Math.floor(Math.random() * from.length)]!;
  const inner = pick(consonants) + pick(consonants) + pick(consonants);
  return `${pick(consonants)}A${pick(consonants)}${pick(consonants)}200315M` +
    `DF${inner}${pick("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")}${pick("0123456789")}`;
}

/** A cycle, a level, a grade, a group, one pupil enrolled, and the flyer priced. */
async function school(api: APIRequestContext): Promise<School> {
  const tag = uniqueSuffix();
  const post = async (path: string, data: unknown): Promise<{ id: string }> => {
    const res = await api.post(`${API_URL}${path}`, { data });
    expect(res.ok(), `${path}: ${await res.text()}`).toBeTruthy();
    return (await res.json()) as { id: string };
  };

  const cycle = await post("/api/v1/config/cycles", {
    name: `M18 ${tag}`,
    startsOn: "2026-08-01",
    endsOn: "2027-06-30",
  });
  const level = await post("/api/v1/config/levels", {
    name: `Preescolar ${tag}`,
    sepCode: tag.slice(0, 8),
    enrollmentGranularity: "cycle",
  });
  const grade = await post("/api/v1/config/grade-levels", {
    levelId: level.id,
    name: `1° ${tag}`,
    ordinal: 1,
  });
  const group = await post("/api/v1/config/groups", {
    cycleId: cycle.id,
    gradeLevelId: grade.id,
    name: `A ${tag}`,
    capacity: 25,
  });
  const student = await post("/api/v1/students", {
    curp: shapedCurp(),
    firstName: "Ana",
    lastName: `Tramos ${tag}`,
    // The pupil goes in the config level itself — named "Preescolar", exactly
    // what the school prints (the fixed SEP list is gone, ADR 0014).
    levelId: level.id,
    gradeGroup: `1A${tag}`,
    guardianName: "Tutor de prueba",
  });
  await post("/api/v1/enrollments", { studentId: student.id, groupId: group.id });

  const plan = await api.put(`${API_URL}/api/v1/finance/tuition-plans`, {
    data: planShell(cycle.id, level.id, flyerConcepts()),
  });
  expect(plan.ok(), await plan.text()).toBeTruthy();

  return { cycleId: cycle.id, levelId: level.id, studentId: student.id };
}

/** Runs billing pretending it is `asOfDate`. */
async function bill(
  api: APIRequestContext,
  cycleId: string,
  asOfDate: string,
): Promise<Record<string, number>> {
  const res = await api.post(`${API_URL}/api/v1/finance/billing-runs`, { data: { cycleId, asOfDate } });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()) as Record<string, number>;
}

/** The pupil's inscription lines as the LEDGER holds them, by description. */
async function inscription(
  api: APIRequestContext,
  s: School,
): Promise<{ byName: Map<string, number>; idOf: Map<string, string> }> {
  const res = await api.get(
    `${API_URL}/api/v1/finance/charges?studentId=${s.studentId}&cycleId=${s.cycleId}`,
  );
  expect(res.ok(), await res.text()).toBeTruthy();
  const { charges } = (await res.json()) as {
    charges: { id: string; description: string; amountCents: number }[];
  };
  const byName = new Map<string, number>();
  const idOf = new Map<string, string>();
  for (const c of charges) {
    if (!LINES.includes(c.description)) continue;
    byName.set(c.description, c.amountCents);
    idOf.set(c.description, c.id);
  }
  return { byName, idOf };
}

function total(byName: Map<string, number>): number {
  return [...byName.values()].reduce((sum, cents) => sum + cents, 0);
}

test.describe("la inscripción del folleto", () => {
  test("se emite al tramo de enero, sube al de marzo, y el primer peso la congela", async ({ request }) => {
    const api = await apiAsOwner(request);
    const s = await school(api);

    // January: the flyer's first column, as three separate lines a family can
    // read on a statement.
    await bill(api, s.cycleId, "2027-01-15");
    const january = await inscription(api, s);
    expect(january.byName.size, "the flyer's three lines").toBe(3);
    expect(total(january.byName)).toBe(TOTAL_JAN);

    // March: the same charges, re-priced where the ladder moved. The two flat
    // lines must not budge — only the inscription climbs.
    const run = await bill(api, s.cycleId, "2027-03-03");
    expect(run.enrollmentRepriced, "the run must report the prices it moved").toBeGreaterThan(0);
    const march = await inscription(api, s);
    expect(total(march.byName)).toBe(TOTAL_MAR);
    expect(march.byName.get("Gastos adm y seguro")).toBe(ADMIN_FEE);
    expect(march.byName.get("Cuota Asociación Padres de Familia")).toBe(PARENTS_FEE);

    // A single peso against the inscription. From here the price is the
    // family's, whatever the calendar does next (ADR 0011 §3).
    const paid = await api.post(`${API_URL}/api/v1/finance/payments`, {
      data: {
        chargeId: march.idOf.get("Inscripción"),
        amountCents: 100,
        method: "cash",
        reference: "e2e",
      },
    });
    expect(paid.ok(), await paid.text()).toBeTruthy();

    // April, when the ladder says $4,400. The paid line stays at March's rung.
    await bill(api, s.cycleId, "2027-04-20");
    const april = await inscription(api, s);
    expect(april.byName.get("Inscripción"), "the first peso freezes the price").toBe(TIER_MAR);
  });

  test("la escalera sólo sube: el servidor rechaza un escalón más barato", async ({ request }) => {
    const api = await apiAsOwner(request);
    const s = await school(api);

    // 33,000 typed as 3,300 — the transposed digit that would walk four hundred
    // issued inscriptions DOWN, below what the families were quoted.
    const res = await api.put(`${API_URL}/api/v1/finance/tuition-plans`, {
      data: planShell(s.cycleId, s.levelId, [
        {
          name: "Inscripción",
          tiers: [
            { effectiveFrom: "2026-08-01", amountCents: 330000 },
            { effectiveFrom: "2027-03-01", amountCents: 33000 },
          ],
        },
      ]),
    });

    expect(res.ok()).toBeFalsy();
    expect(await res.text()).toMatch(/menos|less/i);
  });

  test("editar el tabulador más barato no camina hacia abajo lo ya emitido", async ({ request }) => {
    const api = await apiAsOwner(request);
    const s = await school(api);
    await bill(api, s.cycleId, "2027-03-03");
    expect((await inscription(api, s)).byName.get("Inscripción")).toBe(TIER_MAR);

    // A legal plan — its own ladder climbs — but cheaper than what was issued.
    const cheaper = await api.put(`${API_URL}/api/v1/finance/tuition-plans`, {
      data: planShell(s.cycleId, s.levelId, [
        { name: "Inscripción", tiers: [{ effectiveFrom: "2026-08-01", amountCents: 22000 }] },
      ]),
    });
    expect(cheaper.ok(), await cheaper.text()).toBeTruthy();

    const run = await bill(api, s.cycleId, "2027-03-10");

    expect(run.enrollmentRepriced, "nothing may be walked down").toBe(0);
    expect((await inscription(api, s)).byName.get("Inscripción")).toBe(TIER_MAR);
  });
});
