#!/usr/bin/env node
/**
 * Fills an empty school with a realistic year, through the real API.
 *
 * Everything here goes through the gateway exactly as the panel does — same
 * routes, same auth, same validation — so what the demo shows is what the system
 * actually produced, not rows pushed into Postgres behind its own back. That is
 * the whole point: a seed that bypassed the services would prove nothing, and
 * would drift the day a rule changed.
 *
 * It is NOT idempotent by design: it expects an empty database and says so. Run
 * scripts/reset-demo-db.sh first (or `pnpm demo:seed`, which does both).
 *
 * ORDER MATTERS on demo day, because `pnpm demo:verify` EMPTIES the database —
 * it is the from-zero suite, and starting from zero is its whole subject. So:
 *
 *   pnpm demo:ready    # verify (wipes) → seed → smoke, in that order
 *
 * Running demo:verify after seeding would leave an empty school an hour before
 * walking on stage, which is the one mistake this comment exists to prevent.
 *
 * Usage:  node scripts/seed-demo.mjs
 *         API_URL=http://localhost:8080 node scripts/seed-demo.mjs
 */

const API = process.env.API_URL ?? "http://localhost:8080";
const OWNER_EMAIL = process.env.SEED_OWNER_EMAIL ?? "director@garcialorca.edu.mx";
const OWNER_PASSWORD = process.env.SEED_OWNER_PASSWORD ?? "Admin123.";

// The school year the owner gave: it opens on the last day of August and closes
// in the second week of July.
const CYCLE = { name: "2026-2027", startsOn: "2026-08-31", endsOn: "2027-07-09" };

const STUDENTS_PER_LEVEL = Number(process.env.SEED_STUDENTS_PER_LEVEL ?? 100);
const TEACHER_COUNT = Number(process.env.SEED_TEACHERS ?? 20);
const UNIFORM_SALES = Number(process.env.SEED_UNIFORM_SALES ?? 5);
/** Roughly two families in three have paid the inscription by now. */
const INSCRIPTION_PAID_RATIO = 0.65;

// How many requests may be in flight at once. Enough to fill 400 pupils quickly,
// low enough not to look like an attack to the gateway's own limiters.
const CONCURRENCY = 8;

// --- the school -------------------------------------------------------------

/**
 * Four levels, each with its grades, its subjects and how many groups it splits
 * into. Groups are sized so a class holds about seventeen pupils, which is what a
 * private school of this size actually looks like — one group per grade in
 * primaria (six grades), two per grade everywhere else.
 */
const LEVELS = [
  {
    key: "kinder",
    name: "Kínder",
    sepCode: "KIN",
    grades: ["1° Kínder", "2° Kínder", "3° Kínder"],
    groupsPerGrade: ["A", "B"],
    ageAtStart: 4,
    subjects: ["Lenguaje y comunicación", "Pensamiento matemático", "Exploración del mundo", "Educación socioemocional", "Artes"],
    enrollmentCents: 450000,
    monthlyCents: 280000,
  },
  {
    key: "primaria",
    name: "Primaria",
    sepCode: "PRI",
    grades: ["1° Primaria", "2° Primaria", "3° Primaria", "4° Primaria", "5° Primaria", "6° Primaria"],
    groupsPerGrade: ["A"],
    ageAtStart: 7,
    subjects: ["Español", "Matemáticas", "Ciencias Naturales", "Historia", "Geografía", "Formación Cívica y Ética", "Educación Física", "Inglés"],
    enrollmentCents: 500000,
    monthlyCents: 320000,
  },
  {
    key: "secundaria",
    name: "Secundaria",
    sepCode: "SEC",
    grades: ["1° Secundaria", "2° Secundaria", "3° Secundaria"],
    groupsPerGrade: ["A", "B"],
    ageAtStart: 13,
    subjects: ["Español", "Matemáticas", "Ciencias", "Historia", "Geografía", "Inglés", "Educación Física", "Artes", "Tecnología"],
    enrollmentCents: 550000,
    monthlyCents: 380000,
  },
  {
    key: "preparatoria",
    name: "Preparatoria",
    sepCode: "PRE",
    grades: ["1° Preparatoria", "2° Preparatoria", "3° Preparatoria"],
    groupsPerGrade: ["A", "B"],
    ageAtStart: 16,
    subjects: ["Matemáticas", "Literatura", "Física", "Química", "Biología", "Historia de México", "Inglés", "Filosofía"],
    enrollmentCents: 650000,
    monthlyCents: 450000,
  },
];

/** Trimestres, the shape every level here uses. */
const TERMS = [
  { name: "Primer trimestre", startsOn: "2026-08-31", endsOn: "2026-11-27" },
  { name: "Segundo trimestre", startsOn: "2026-11-30", endsOn: "2027-03-19" },
  { name: "Tercer trimestre", startsOn: "2027-03-22", endsOn: "2027-07-09" },
];

const UNIFORMS = [
  { name: "Suéter institucional", priceCents: 65000, levelId: "" },
  { name: "Playera polo blanca", priceCents: 32000, levelId: "" },
  { name: "Pantalón gris escolar", priceCents: 48000, levelId: "" },
  { name: "Falda de cuadros", priceCents: 52000, levelId: "" },
  { name: "Sudadera deportiva", priceCents: 58000, levelId: "" },
  { name: "Short deportivo", priceCents: 29000, levelId: "" },
];

const SIZES = ["4", "6", "8", "10", "12", "CH", "M", "G", "XG"];

// Names common enough in a Mexican school register to look like a real roll.
const GIVEN_F = ["Sofía", "Valentina", "Regina", "Ximena", "Camila", "Renata", "Isabella", "Victoria", "Romina", "Fernanda", "Daniela", "Mariana", "Andrea", "Paulina", "Emilia", "Jimena", "Ana Paula", "Luciana", "Natalia", "Carolina"];
const GIVEN_M = ["Santiago", "Mateo", "Sebastián", "Diego", "Emiliano", "Leonardo", "Maximiliano", "Alejandro", "Daniel", "Rodrigo", "Ángel", "Iker", "Bruno", "Patricio", "Tadeo", "Julián", "Gael", "Andrés", "Héctor", "Ricardo"];
const SURNAMES = ["Hernández", "García", "Martínez", "López", "González", "Pérez", "Rodríguez", "Sánchez", "Ramírez", "Cruz", "Flores", "Gómez", "Morales", "Vázquez", "Reyes", "Jiménez", "Torres", "Díaz", "Ruiz", "Mendoza", "Aguilar", "Ortiz", "Castillo", "Romero", "Álvarez", "Guerrero", "Medina", "Rojas", "Vargas", "Contreras", "Salazar", "Cabrera", "Ibarra", "Zamora", "Villalobos", "Cavazos", "Escobar", "Peña", "Navarro", "Lozano"];

// --- transport --------------------------------------------------------------

let sessionCookie = "";

async function call(method, path, body, extraHeaders = {}) {
  const res = await fetch(`${API}${path}`, {
    method,
    headers: {
      "Content-Type": "application/json",
      ...(sessionCookie === "" ? {} : { Cookie: sessionCookie }),
      ...extraHeaders,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const raw = await res.text();
  if (!res.ok) {
    throw new Error(`${method} ${path} → ${res.status}: ${raw.slice(0, 300)}`);
  }
  return raw === "" ? {} : JSON.parse(raw);
}

const get = (path) => call("GET", path);
const post = (path, body, headers) => call("POST", path, body, headers);
const put = (path, body) => call("PUT", path, body);

async function signIn() {
  const res = await fetch(`${API}/api/v1/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email: OWNER_EMAIL, password: OWNER_PASSWORD }),
  });
  if (!res.ok) {
    throw new Error(`login failed (${res.status}). Is the stack up, and is the owner seeded?`);
  }
  // Node's fetch does not keep a jar, so the session is carried by hand.
  const jar = res.headers.getSetCookie?.() ?? [];
  sessionCookie = jar.map((c) => c.split(";")[0]).join("; ");
  if (sessionCookie === "") throw new Error("login returned no session cookie");
}

/**
 * Runs tasks with a bounded number in flight.
 *
 * Sequential would take minutes for four hundred pupils; unbounded would open
 * four hundred sockets at once and trip the gateway's own protections, which is
 * a self-inflicted failure with a confusing error message.
 */
async function inBatches(items, worker) {
  const out = new Array(items.length);
  let next = 0;
  const runners = Array.from({ length: Math.min(CONCURRENCY, items.length) }, async () => {
    for (;;) {
      const i = next++;
      if (i >= items.length) return;
      out[i] = await worker(items[i], i);
    }
  });
  await Promise.all(runners);
  return out;
}

// --- identity ---------------------------------------------------------------

const VOWELS = "AEIOU";
const CONSONANTS = "BCDFGHJKLMNPQRSTVWXYZ";
const STATES = ["DF", "JC", "NL", "MC", "PL", "GT", "VZ", "CH"];

/**
 * A CURP that satisfies the students service's shape check, unique per pupil.
 *
 * It is not a real CURP and does not pretend to be — the check digit is not the
 * official algorithm — but it has the right shape, which is what the boundary
 * validates. Uniqueness is carried by the three internal consonants, giving far
 * more combinations than a school will ever have pupils.
 */
function curpFor(index, birthDate, sex) {
  const a = CONSONANTS[index % CONSONANTS.length];
  const b = VOWELS[index % VOWELS.length];
  const c = CONSONANTS[(index >> 2) % CONSONANTS.length];
  const d = CONSONANTS[(index >> 5) % CONSONANTS.length];
  const [y, m, day] = birthDate.split("-");
  const state = STATES[index % STATES.length];
  const inner =
    CONSONANTS[index % CONSONANTS.length] +
    CONSONANTS[Math.floor(index / CONSONANTS.length) % CONSONANTS.length] +
    CONSONANTS[Math.floor(index / (CONSONANTS.length * CONSONANTS.length)) % CONSONANTS.length];
  const homonym = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"[index % 36];
  const check = String(index % 10);
  return `${a}${b}${c}${d}${y.slice(2)}${m}${day}${sex}${state}${inner}${homonym}${check}`;
}

/** A birth date that puts the pupil at the right age for their grade. */
function birthDateFor(ageAtStart, gradeIndex, index) {
  const year = 2026 - ageAtStart - gradeIndex;
  const month = String((index % 12) + 1).padStart(2, "0");
  const day = String((index % 27) + 1).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function pick(list, i) {
  return list[i % list.length];
}

// --- the run ----------------------------------------------------------------

function log(step, detail) {
  console.log(`[seed] ${step}${detail === undefined ? "" : ` — ${detail}`}`);
}

async function assertEmpty() {
  const { students } = await get("/api/v1/students?pageSize=1");
  if (Array.isArray(students) && students.length > 0) {
    throw new Error(
      "the database already has pupils. This seed builds a school from nothing; " +
        "run scripts/reset-demo-db.sh first (or use `pnpm demo:seed`).",
    );
  }
}

async function seedCatalogue() {
  const created = await post("/api/v1/config/cycles", CYCLE);
  // Cycles are born as drafts. A demo school is a school in session, and every
  // screen that asks "which cycle is active" — the roster header, the group
  // dropdowns, the tuition table, the billing run — needs an answer.
  const cycle = await post(`/api/v1/config/cycles/${created.id}/activate`, {});
  log("ciclo", `${cycle.name} (${CYCLE.startsOn} → ${CYCLE.endsOn}) · ${cycle.status}`);

  const levels = [];
  for (const spec of LEVELS) {
    const level = await post("/api/v1/config/levels", {
      name: spec.name,
      sepCode: spec.sepCode,
      enrollmentGranularity: "cycle",
    });

    for (const [ordinal, term] of TERMS.entries()) {
      await post("/api/v1/config/terms", {
        cycleId: cycle.id,
        levelId: level.id,
        name: term.name,
        ordinal: ordinal + 1,
        startsOn: term.startsOn,
        endsOn: term.endsOn,
      });
    }

    const grades = [];
    for (const [gradeIndex, gradeName] of spec.grades.entries()) {
      const grade = await post("/api/v1/config/grade-levels", {
        levelId: level.id,
        name: gradeName,
        ordinal: gradeIndex + 1,
      });

      const subjects = [];
      for (const subjectName of spec.subjects) {
        subjects.push(await post("/api/v1/config/subjects", { gradeLevelId: grade.id, name: subjectName }));
      }

      const groups = [];
      for (const suffix of spec.groupsPerGrade) {
        groups.push(
          await post("/api/v1/config/groups", {
            cycleId: cycle.id,
            gradeLevelId: grade.id,
            name: `${gradeName.split(" ")[0]} ${suffix}`,
            capacity: 25,
          }),
        );
      }
      grades.push({ ...grade, gradeIndex, subjects, groups });
    }
    levels.push({ spec, level, grades });
    log("nivel", `${spec.name}: ${grades.length} grados, ${grades.reduce((n, g) => n + g.groups.length, 0)} grupos, ${spec.subjects.length} materias por grado`);
  }
  return { cycle, levels };
}

async function seedTeachers(levels) {
  const teachers = [];
  for (let i = 0; i < TEACHER_COUNT; i++) {
    const first = i % 2 === 0 ? pick(GIVEN_F, i) : pick(GIVEN_M, i + 3);
    const last = `${pick(SURNAMES, i * 3)} ${pick(SURNAMES, i * 7 + 11)}`;
    teachers.push(
      await post("/api/v1/teachers", {
        firstName: first,
        lastName: last,
        email: `${first.split(" ")[0].toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "")}.${i}@garcialorca.edu.mx`,
        userId: "",
      }),
    );
  }
  log("docentes", `${teachers.length} en plantilla`);

  // Every group gets a teacher for every subject its grade teaches. Teachers are
  // cycled rather than assigned at random, so each ends up with a believable
  // load instead of one carrying forty groups and another none.
  const assignments = [];
  let cursor = 0;
  for (const { grades } of levels) {
    for (const grade of grades) {
      for (const group of grade.groups) {
        for (const subject of grade.subjects) {
          assignments.push({ groupId: group.id, subjectId: subject.id, teacherId: teachers[cursor++ % teachers.length].id });
        }
      }
    }
  }
  await inBatches(assignments, (a) => post("/api/v1/teaching-assignments", a));
  log("asignaciones", `${assignments.length} (materia × grupo × docente)`);
  return teachers;
}

async function seedStudents(levels) {
  const planned = [];
  let serial = 0;
  for (const { spec, level, grades } of levels) {
    const groups = grades.flatMap((g) => g.groups.map((group) => ({ group, gradeIndex: g.gradeIndex })));
    for (let i = 0; i < STUDENTS_PER_LEVEL; i++) {
      const { group, gradeIndex } = groups[i % groups.length];
      const female = i % 2 === 0;
      const first = female ? pick(GIVEN_F, i) : pick(GIVEN_M, i);
      const paternal = pick(SURNAMES, serial * 3 + 1);
      const maternal = pick(SURNAMES, serial * 5 + 7);
      const birth = birthDateFor(spec.ageAtStart, gradeIndex, serial);
      planned.push({
        curp: curpFor(serial, birth, female ? "M" : "H"),
        firstName: first,
        lastName: `${paternal} ${maternal}`,
        // A pupil points at the config level itself (ADR 0014).
        levelId: level.id,
        gradeGroup: group.name,
        guardianName: `${pick(female ? GIVEN_M : GIVEN_F, serial + 5)} ${paternal}`,
        groupId: group.id,
      });
      serial++;
    }
  }

  const unique = new Set(planned.map((p) => p.curp));
  if (unique.size !== planned.length) {
    throw new Error(`CURP collision: ${planned.length - unique.size} duplicates among ${planned.length}`);
  }

  const created = await inBatches(planned, async (p) => {
    const student = await post("/api/v1/students", {
      curp: p.curp,
      firstName: p.firstName,
      lastName: p.lastName,
      levelId: p.levelId,
      gradeGroup: p.gradeGroup,
      guardianName: p.guardianName,
    });
    return { ...student, groupId: p.groupId };
  });
  log("alumnos", `${created.length} dados de alta`);

  await inBatches(created, (s) => post("/api/v1/enrollments", { studentId: s.id, groupId: s.groupId }));
  log("inscripciones", `${created.length} colocados en su grupo`);
  return created;
}

async function seedTuition(cycle, levels) {
  for (const { spec, level } of levels) {
    await put("/api/v1/finance/tuition-plans", {
      cycleId: cycle.id,
      levelId: level.id,
      enrollmentCents: spec.enrollmentCents,
      monthlyCents: spec.monthlyCents,
      // Ten payments from September, due on the 10th, with a $150 surcharge per
      // month a family runs late.
      months: 10,
      firstBilledMonth: 9,
      dueDay: 10,
      lateFeeCents: 15000,
      active: true,
    });
  }
  log("tabulador", LEVELS.map((l) => `${l.name} $${(l.monthlyCents / 100).toFixed(0)}/mes`).join(", "));

  const run = await post("/api/v1/finance/billing-runs", { cycleId: cycle.id });
  log(
    "corrida",
    `${run.tuitionCreated} colegiaturas, ${run.enrollmentCreated} inscripciones, ${run.lateFeesCreated} recargos` +
      (run.levelsWithoutPlan.length > 0 ? ` · sin tabulador: ${run.levelsWithoutPlan.join(", ")}` : ""),
  );
  return run;
}

async function seedPayments(students, cycle) {
  // Two families in three have already paid the inscription; the rest are the
  // school's live receivable, which is the point of showing Adeudos at all.
  const payers = students.filter((_, i) => i % 100 < INSCRIPTION_PAID_RATIO * 100);
  let paid = 0;
  await inBatches(payers, async (s) => {
    const { charges } = await get(`/api/v1/finance/charges?studentId=${s.id}&cycleId=${cycle.id}`);
    const inscription = (charges ?? []).find((c) => c.description === "Inscripción" && c.status === "pending");
    if (inscription === undefined) return;
    await post("/api/v1/finance/payments", {
      chargeId: inscription.id,
      amountCents: inscription.remainingCents,
      method: pick(["cash", "transfer", "card"], paid),
      reference: `REC-${String(paid + 1).padStart(5, "0")}`,
      idempotencyKey: `seed-inscription-${s.id}`,
    });
    paid++;
  });
  log("pagos", `${paid} inscripciones liquidadas · ${students.length - paid} siguen como adeudo`);
}

async function seedUniforms(students) {
  const items = [];
  for (const spec of UNIFORMS) {
    const item = await post("/api/v1/uniforms/items", {
      name: spec.name,
      description: "",
      levelId: spec.levelId,
      priceCents: spec.priceCents,
    });
    const sizes = SIZES.map((label, i) => ({ sizeLabel: label, sizeOrder: i + 1, minStock: 3 }));
    const withSizes = await post(`/api/v1/uniforms/items/${item.id}/sizes`, { sizes });
    items.push({ ...item, sizes: withSizes.sizes ?? withSizes });
  }
  log("catálogo de uniformes", `${items.length} prendas × ${SIZES.length} tallas`);

  // One delivery from the supplier, stocking every size of every garment.
  const stock = await get("/api/v1/uniforms/stock");
  const lines = (stock.stock ?? []).map((s, i) => ({ stockId: s.id, qty: 12 + (i % 9) }));
  await post("/api/v1/uniforms/stock/receive", { lines, note: "Entrega inicial del proveedor · ciclo 2026-2027" });
  log("inventario", `${lines.length} tallas surtidas (${lines.reduce((n, l) => n + l.qty, 0)} piezas)`);

  for (let i = 0; i < UNIFORM_SALES; i++) {
    const student = students[i * 37 % students.length];
    const garment = items[i % items.length];
    const size = pick(SIZES, i + 4);
    await post(
      "/api/v1/uniforms/counter-sale",
      {
        studentId: student.id,
        lines: [{ itemId: garment.id, sizeLabel: size, qty: 1 }],
        method: pick(["cash", "card", "transfer"], i),
        reference: "",
        note: "",
      },
      { "Idempotency-Key": `seed-sale-${i}` },
    );
  }
  log("pedidos de uniforme", `${UNIFORM_SALES} ventas de mostrador`);
}

async function main() {
  log("conectando", API);
  await signIn();
  await assertEmpty();

  const { cycle, levels } = await seedCatalogue();
  await seedTeachers(levels);
  const students = await seedStudents(levels);
  await seedTuition(cycle, levels);
  await seedPayments(students, cycle);
  await seedUniforms(students);

  console.log("");
  log("listo", `${students.length} alumnos, ${TEACHER_COUNT} docentes, ${LEVELS.length} niveles, ciclo ${CYCLE.name}`);
  log("acceso", `${OWNER_EMAIL} — el ciclo abre el ${CYCLE.startsOn}`);
}

main().catch((err) => {
  console.error(`[seed] falló: ${err.message}`);
  process.exit(1);
});
