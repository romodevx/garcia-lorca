/* GRADES — numeric (6–10) or qualitative (kínder), with average. */
(function () {
  function gradeKind(g) {
    if (g >= 9) return "ok";
    if (g >= 8) return "lime";
    if (g >= 7) return "warn";
    return "bad";
  }
  function badgeStyle(kind) {
    const map = {
      ok:   { background: "var(--ok-bg)",   color: "var(--ok)" },
      lime: { background: "var(--primary-soft)", color: "var(--primary-ink)" },
      warn: { background: "var(--warn-bg)", color: "var(--warn)" },
      bad:  { background: "var(--bad-bg)",  color: "var(--bad)" },
    };
    return map[kind];
  }

  function Grades({ D, level, openSubject }) {
    const qual = level.grading === "qual";
    const g = qual ? D.gradesQual : D.gradesNum;

    return (
      <div className="gl-screen-anim">
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "8px 4px 12px" }}>
          <div className="gl-detail-title">Calificaciones</div>
          <Chip kind="lime" icon="calendar">{g.period}</Chip>
        </div>

        {qual ? (
          <QualHero D={D} g={g} />
        ) : (
          <div className="gl-card gl-avg-hero">
            <Ring pct={g.average / 10} color="var(--ok)" track="var(--surface-sunken)">
              <div className="gl-avg-num" style={{ color: "var(--ok)" }}>{g.average.toFixed(1)}</div>
              <div className="gl-avg-of">de 10</div>
            </Ring>
            <div style={{ flex: 1 }}>
              <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 19 }}>Promedio general</div>
              <div className="gl-hero-sub" style={{ marginTop: 2 }}>{g.period} · {g.subjects.length} materias</div>
              <div style={{ marginTop: 10 }}>
                <Chip kind="ok" icon="arrow-up-right">Muy bien</Chip>
              </div>
            </div>
          </div>
        )}

        <SectionLabel>{qual ? "Campos formativos" : "Materias"}</SectionLabel>
        <div className="gl-card">
          <div className="gl-list">
            {g.subjects.map((s, i) => qual ? (
              <QualRow key={i} D={D} s={s} />
            ) : (
              <div className="gl-listrow tap" key={i} onClick={() => openSubject(s)}>
                <TintIcon name={s.icon} tint={s.tint} />
                <div className="gl-listrow-main">
                  <div className="gl-listrow-title">{s.name}</div>
                  <div className="gl-listrow-sub">3 parciales · prom. {s.grade.toFixed(1)}</div>
                </div>
                <div className="gl-grade-badge" style={badgeStyle(gradeKind(s.grade))}>
                  {s.grade === 10 ? "10" : s.grade.toFixed(1)}
                </div>
              </div>
            ))}
          </div>
        </div>
        {!qual && (
          <div style={{ fontSize: 12, color: "var(--text-3)", fontWeight: 700, marginTop: 12, textAlign: "center" }}>
            Toca una materia para ver los parciales
          </div>
        )}
      </div>
    );
  }

  function QualHero({ D, g }) {
    const m = D.qualMeta;
    const items = [
      { k: "logrado", n: g.summary.logrado, chip: "ok" },
      { k: "proceso", n: g.summary.proceso, chip: "warn" },
      { k: "apoyo", n: g.summary.apoyo, chip: "bad" },
    ];
    return (
      <div className="gl-card gl-card-pad">
        <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 19, marginBottom: 4 }}>
          Evaluación cualitativa
        </div>
        <div className="gl-hero-sub" style={{ marginBottom: 14 }}>{g.period} · {g.subjects.length} campos</div>
        <div className="gl-statgrid">
          {items.map((it) => (
            <div className="gl-stat" key={it.k} style={{ background: "var(--surface-2)", borderRadius: 16 }}>
              <div className="gl-stat-num" style={{
                color: it.chip === "ok" ? "var(--ok)" : it.chip === "warn" ? "var(--warn)" : "var(--bad)",
              }}>{it.n}</div>
              <div className="gl-stat-lab">{m[it.k].label}</div>
            </div>
          ))}
        </div>
      </div>
    );
  }

  function QualRow({ D, s }) {
    const meta = D.qualMeta[s.level];
    return (
      <div className="gl-listrow">
        <TintIcon name={s.icon} tint={s.level === "logrado" ? "forest" : s.level === "proceso" ? "orange" : "red"} />
        <div className="gl-listrow-main">
          <div className="gl-listrow-title">{s.name}</div>
          <div className="gl-bar" style={{ marginTop: 8 }}>
            <span style={{
              width: meta.pct + "%",
              background: meta.chip === "ok" ? "var(--ok)" : meta.chip === "warn" ? "var(--warn)" : "var(--bad)",
            }} />
          </div>
        </div>
        <span className={"gl-qual-badge " + (meta.chip === "ok" ? "" : "")} style={badgeStyle(meta.chip)}>
          {meta.label}
        </span>
      </div>
    );
  }

  window.Grades = Grades;
})();
