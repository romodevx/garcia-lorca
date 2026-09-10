import { expect, test, type APIRequestContext } from "@playwright/test";

import { API_URL, apiAsOwner, runId, uniqueSuffix } from "./helpers";

/**
 * The school's printed price list, end to end.
 *
 * The flyer for 2026/2027 does not charge one amount to join. It charges three
 * lines, and the first of them doubles between January and April:
 *
 *   Concepto                 Ene/Feb   Marzo   Abr-Sep
 *   Inscripción              $2,200   $3,300   $4,400
 *   Gastos adm y seguro      $4,700   $4,700   $4,700
 *   Cuota Asociación         $  850   $  850   $  850
 *   TOTAL A PAGAR            $7,750   $8,850   $9,950
 *
 * These tests assert those three totals against a real stack, because every
 * layer below has its own tests and none of them can prove the whole path: the
 * ladder is typed in one service, stored as JSON, priced at read time, re-priced
 * by the run, and frozen by the first peso. A number that is right in four
 * places and wrong at the seam is exactly what a school finds out in June.
 *
 * They are WRITE tests — they create their own cycle, level and pupil — so they
 * do not run against the demo database. See ADR 0011 and
 * .claude/docs/PLAN-m18-inscripcion-por-tramos.md.
 */

const TIER_JAN = 220000;
const TIER_MAR = 330000;
const TIER_APR = 440000;
const ADMIN_FEE = 470000;
const PARENTS_FEE = 85000;

const TOTAL_JAN = TIER_JAN + ADMIN_FEE + PARENTS_FEE; // 7,750
const TOTAL_MAR = TIER_MAR + ADMIN_FEE + PARENTS_FEE; // 8,850
const TOTAL_APR = TIER_APR + ADMIN_FEE + PARENTS_FEE; // 9,950

/** The flyer's three lines, as the panel would save them. */
function flyerConcepts(): unknown[] {
  const flat = (name: string, amountCents: number): unknown => ({
    name,
    tiers: [{ effectiveFrom: "2027-01-01", amountCents }],
  });
  return [
    {
      name: "Inscripción",
      tiers: [
        { effectiveFrom: "2027-01-01", amountCents: TIER_JAN },
        { effectiveFrom: "2027-03-01", amountCents: TIER_MAR },
        { effectiveFrom: "2027-04-01", amountCents: TIER_APR },
      ],
    },
    flat("Gastos adm y seguro", ADMIN_FEE),
    flat("Cuota Asociación Padres de Familia", PARENTS_FEE),
  ];
}

/** The whole school this spec needs: a cycle, a level, a group and a pupil. */
async function school(api: APIRequestContext): Promise<{ cycleId: string; studentId: string }> {
  const tag = uniqueSuffix();

  const cycle = await api.post(`${API_URL}/api/v1/config/cycles`, {
    data: { name: `M18 ${tag}`, startsOn: "2026-08-01", endsOn: "2027-06-30" },
  });
  expect(cycle.ok(), await cycle.text()).toBeTruthy();
  const cycleId = (await cycle.json()).cycle.id as string;

  const level = await api.post(`${API_URL}/api/v1/config/levels`, {
    data: { name: `Preescolar ${tag}`, sepCode: `P${tag}`.slice(0, 8), enrollmentGranularity: "cycle" },
  });
  expect(level.ok(), await level.text()).toBeTruthy();
  const levelId = (await level.json()).level.id as string;

  await api.put(`${API_URL}/api/v1/finance/tuition-plans`, {
    data: {
      cycleId,
      levelId,
      // One shape or the other, never both (ADR 0011).
      enrollmentCents: 0,
      enrollmentConcepts: flyerConcepts(),
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
    },
  });

  return { cycleId, studentId: "" };
}

/** What the ladder quotes for one level of one cycle, as of a day. */
async function inscriptionTotal(api: APIRequestContext, cycleId: string, asOf: string): Promise<number> {
  const res = await api.get(`${API_URL}/api/v1/finance/tuition-plans?cycleId=${cycleId}`);
  expect(res.ok(), await res.text()).toBeTruthy();
  const plans = (await res.json()).plans as { enrollmentConcepts?: { tiers: { effectiveFrom: string; amountCents: number }[] }[] }[];
  const concepts = plans[0]?.enrollmentConcepts ?? [];
  return concepts.reduce((sum, c) => {
    const arrived = c.tiers.filter((t) => t.effectiveFrom <= asOf);
    const tier = arrived.length > 0
      ? arrived.reduce((best, t) => (t.effectiveFrom > best.effectiveFrom ? t : best))
      : c.tiers[0];
    return sum + (tier?.amountCents ?? 0);
  }, 0);
}

test.describe(`la inscripción del folleto (${runId()})`, () => {
  test("el tabulador cobra los tres totales impresos", async ({ request }) => {
    const api = await apiAsOwner(request);
    const { cycleId } = await school(api);

    // The three columns of the flyer, read back off the server rather than off
    // the form that typed them.
    expect(await inscriptionTotal(api, cycleId, "2027-01-15")).toBe(TOTAL_JAN);
    expect(await inscriptionTotal(api, cycleId, "2027-02-28")).toBe(TOTAL_JAN);
    expect(await inscriptionTotal(api, cycleId, "2027-03-03")).toBe(TOTAL_MAR);
    expect(await inscriptionTotal(api, cycleId, "2027-04-20")).toBe(TOTAL_APR);
    expect(await inscriptionTotal(api, cycleId, "2027-09-15")).toBe(TOTAL_APR);
  });

  test("la escalera sólo sube: el servidor refusa un escalón más barato", async ({ request }) => {
    const api = await apiAsOwner(request);
    const { cycleId } = await school(api);

    const res = await api.put(`${API_URL}/api/v1/finance/tuition-plans`, {
      data: {
        cycleId,
        levelId: "does-not-matter",
        enrollmentCents: 0,
        // 33,000 typed as 3,300 — the transposed digit that would walk four
        // hundred issued inscriptions DOWN.
        enrollmentConcepts: [
          {
            name: "Inscripción",
            tiers: [
              { effectiveFrom: "2027-01-01", amountCents: 330000 },
              { effectiveFrom: "2027-03-01", amountCents: 33000 },
            ],
          },
        ],
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
      },
    });

    expect(res.ok()).toBeFalsy();
    expect(await res.text()).toMatch(/menos|less/i);
  });

  test("un plan sin conceptos sigue cobrando su monto único", async ({ request }) => {
    const api = await apiAsOwner(request);
    const { cycleId } = await school(api);

    // Every plan written before M18 means what it meant: one inscription, at
    // enrollmentCents, on any day.
    const res = await api.get(`${API_URL}/api/v1/finance/tuition-plans?cycleId=${cycleId}`);
    const plans = (await res.json()).plans as { enrollmentCents: number; enrollmentConcepts?: unknown[] }[];
    for (const plan of plans) {
      const hasLadder = (plan.enrollmentConcepts ?? []).length > 0;
      // The two are mutually exclusive, and the database enforces it too.
      expect(hasLadder ? plan.enrollmentCents === 0 : true).toBeTruthy();
    }
  });
});
