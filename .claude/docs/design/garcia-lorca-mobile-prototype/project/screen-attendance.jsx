/* ATTENDANCE — calendar + list, one-tap excuse. */
(function () {
  const { useState } = React;

  const STATUS_LABEL = {
    present: "Asistió", absent: "Falta", excused: "Justificada",
    retardo: "Retardo", holiday: "Sin clases",
  };
  const STATUS_CHIP = { present: "ok", absent: "bad", excused: "warn", retardo: "warn", holiday: "neutral" };

  function Attendance({ D, overrides, openExcuse }) {
    const [view, setView] = useState("cal");
    const att = D.att;
    const stat = (d) => overrides[d] || att.days[d];

    // first unjustified absence
    let pendingAbsence = null;
    for (let d = 1; d <= 30; d++) {
      if (stat(d) === "absent") { pendingAbsence = { day: d }; break; }
    }
    const pendingLabel = "jue 4 jun";

    const lead = att.firstDow === 0 ? 6 : att.firstDow - 1; // Monday-first
    const cells = [];
    for (let i = 0; i < lead; i++) cells.push(null);
    for (let d = 1; d <= 30; d++) cells.push(d);

    const cellClass = (d) => {
      const s = stat(d);
      const col = (cells.indexOf(d)) % 7;
      const weekend = col >= 5;
      let c = "gl-cal-cell tap";
      if (s === "present") c += " present";
      else if (s === "absent") c += " absent";
      else if (s === "excused") c += " excused";
      else if (s === "retardo") c += " excused";
      else c += weekend ? " muted" : " muted";
      if (d === att.todayDate) c += " today";
      return c;
    };

    return (
      <div className="gl-screen-anim">
        <div className="gl-detail-title" style={{ padding: "8px 4px 12px" }}>Asistencia</div>

        {/* stats */}
        <div className="gl-card">
          <div className="gl-statgrid">
            <div className="gl-stat">
              <div className="gl-stat-num" style={{ color: "var(--ok)" }}>{att.stats.pct}%</div>
              <div className="gl-stat-lab">Asistencia</div>
            </div>
            <div className="gl-stat" style={{ borderLeft: "1px solid var(--line)", borderRight: "1px solid var(--line)" }}>
              <div className="gl-stat-num" style={{ color: "var(--bad)" }}>{att.stats.faltas}</div>
              <div className="gl-stat-lab">Faltas</div>
            </div>
            <div className="gl-stat">
              <div className="gl-stat-num" style={{ color: "var(--warn)" }}>{att.stats.retardos}</div>
              <div className="gl-stat-lab">Retardos</div>
            </div>
          </div>
        </div>

        {/* unjustified absence callout */}
        {pendingAbsence && (
          <div className="gl-card gl-card-pad" style={{ marginTop: 12, display: "flex", alignItems: "center", gap: 12 }}>
            <div className="gl-statuscircle tint-red" style={{ width: 44, height: 44 }}>
              <Icon name="alert" size={22} />
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontWeight: 800, fontSize: 14.5 }}>1 falta sin justificar</div>
              <div className="gl-hero-sub">{pendingLabel}</div>
            </div>
            <button className="gl-btn primary sm" style={{ width: "auto", padding: "0 16px" }}
              onClick={() => openExcuse({ day: pendingAbsence.day, label: pendingLabel })}>
              Justificar
            </button>
          </div>
        )}

        <div className="gl-segment" style={{ marginTop: 14 }}>
          <button className={view === "cal" ? "active" : ""} onClick={() => setView("cal")}>Calendario</button>
          <button className={view === "list" ? "active" : ""} onClick={() => setView("list")}>Lista</button>
        </div>

        {view === "cal" ? (
          <div className="gl-card gl-card-pad gl-fade" style={{ marginTop: 14 }}>
            <div className="gl-cal-head">
              <button className="gl-iconbtn" style={{ width: 34, height: 34, borderRadius: 11 }}>
                <Icon name="chevron-left" size={18} />
              </button>
              <div className="gl-cal-month">{att.monthLabel}</div>
              <button className="gl-iconbtn" style={{ width: 34, height: 34, borderRadius: 11 }}>
                <Icon name="chevron-right" size={18} />
              </button>
            </div>
            <div className="gl-cal-grid" style={{ marginBottom: 6 }}>
              {["L", "M", "M", "J", "V", "S", "D"].map((d, i) => (
                <div className="gl-cal-dow" key={i}>{d}</div>
              ))}
            </div>
            <div className="gl-cal-grid">
              {cells.map((d, i) => d === null
                ? <div className="gl-cal-cell empty" key={"e" + i} />
                : (
                  <div className={cellClass(d)} key={d}
                    onClick={() => stat(d) === "absent" && openExcuse({ day: d, label: pendingLabel })}>
                    {d}
                  </div>
                ))}
            </div>
            <div className="gl-legend">
              {[["present", "Asistió"], ["absent", "Falta"], ["excused", "Justificada / retardo"]].map(([s, l]) => (
                <div className="gl-legend-item" key={s}>
                  <span className={"gl-legend-swatch " + s} style={{
                    background: s === "present" ? "var(--ok)" : s === "absent" ? "var(--bad)" : "var(--warn)",
                  }} />
                  {l}
                </div>
              ))}
            </div>
            <div style={{ fontSize: 12, color: "var(--text-3)", fontWeight: 700, marginTop: 12, textAlign: "center" }}>
              Toca un día con falta para justificarlo
            </div>
          </div>
        ) : (
          <div className="gl-card gl-fade" style={{ marginTop: 14 }}>
            <div className="gl-list">
              {att.log.map((e, i) => (
                <div className="gl-listrow" key={i}>
                  <StatusDot status={e.status} size={12} />
                  <div className="gl-listrow-main">
                    <div className="gl-listrow-title">{e.date}</div>
                    <div className="gl-listrow-sub">{e.note}</div>
                  </div>
                  {e.status === "absent"
                    ? <button className="gl-btn primary sm" style={{ width: "auto", padding: "0 14px", height: 38 }}
                        onClick={() => openExcuse({ day: 4, label: e.date })}>Justificar</button>
                    : <Chip kind={STATUS_CHIP[e.status]}>{STATUS_LABEL[e.status]}</Chip>}
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    );
  }
  window.Attendance = Attendance;
})();
