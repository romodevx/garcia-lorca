/* HOME — child context + actionable summary cards. */
(function () {
  function Home({ D, level, go, paid }) {
    const { child, today, events } = D;
    const bal = today.balance;
    const att = today.attendance;

    return (
      <div className="gl-screen-anim">
        <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", padding: "8px 4px 2px" }}>
          <div>
            <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 22, letterSpacing: "-0.3px" }}>
              Hola, {child.parent}
            </div>
            <div style={{ fontSize: 13, color: "var(--text-3)", fontWeight: 700, marginTop: 1, textTransform: "capitalize" }}>
              {today.dateLabel} · Ciclo 2025–2026
            </div>
          </div>
        </div>

        {/* HERO: today's attendance */}
        <SectionLabel>Hoy</SectionLabel>
        <div className="gl-card tap gl-hero" onClick={() => go("attendance")}>
          <div className="gl-hero-top">
            <div className="gl-statuscircle tint-lime">
              <Icon name="check" size={28} stroke={3} />
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div className="gl-hero-label">Asistencia de hoy</div>
              <div className="gl-hero-value">{att.label}</div>
              <div className="gl-hero-sub">{att.detail}</div>
            </div>
            <Go />
          </div>
        </div>

        {/* Balance — actionable, traffic-light */}
        <div className="gl-card tap gl-hero" style={{ marginTop: 12 }} onClick={() => go("balance")}>
          <div className="gl-hero-top">
            <div className={"gl-statuscircle " + (paid ? "tint-lime" : "tint-orange")}>
              <Icon name="wallet" size={26} />
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div className="gl-hero-label">{paid ? "Estado de cuenta" : "Saldo por pagar"}</div>
              <div className="gl-hero-value">{paid ? money(0) : money(bal.amount)}</div>
              <div style={{ marginTop: 7 }}>
                {paid
                  ? <Chip kind="ok" icon="check">Al corriente</Chip>
                  : <Chip kind="warn" icon="clock">{bal.label} · {bal.due}</Chip>}
              </div>
            </div>
            <Go />
          </div>
          {!paid && (
            <button className="gl-btn primary sm" onClick={(e) => { e.stopPropagation(); go("balance", { pay: true }); }}>
              <Icon name="credit-card" size={19} /> Pagar ahora
            </button>
          )}
        </div>

        {/* Two-up: announcements + next event */}
        <div className="gl-row2" style={{ marginTop: 12 }}>
          <div className="gl-card tap gl-card-pad" onClick={() => go("announcements")}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
              <TintIcon name="megaphone" tint="purple" />
              <span className="gl-chip bad" style={{ height: 24 }}>{today.unread} nuevos</span>
            </div>
            <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 17, marginTop: 12 }}>Avisos</div>
            <div className="gl-hero-sub" style={{ marginTop: 1 }}>Sin leer</div>
          </div>

          <div className="gl-card tap gl-card-pad" onClick={() => go("eventos")}>
            <TintIcon name="calendar" tint="forest" />
            <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15, marginTop: 12, lineHeight: 1.2 }}>
              {today.nextEvent.title}
            </div>
            <div className="gl-hero-sub" style={{ marginTop: 4 }}>{today.nextEvent.when}</div>
          </div>
        </div>

        {/* Agenda */}
        <SectionLabel>Próximos eventos</SectionLabel>
        <div className="gl-card">
          <div className="gl-list">
            {events.map((ev, i) => (
              <div className="gl-listrow" key={i}>
                <div className={"tint-" + ev.tint} style={{
                  width: 46, height: 46, borderRadius: 14, flexShrink: 0,
                  display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", lineHeight: 1,
                }}>
                  <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 18 }}>{ev.day}</span>
                  <span style={{ fontSize: 9, fontWeight: 800, opacity: .8, marginTop: 1 }}>{ev.mon}</span>
                </div>
                <div className="gl-listrow-main">
                  <div className="gl-listrow-title">{ev.title}</div>
                  <div className="gl-listrow-sub">{ev.sub}</div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    );
  }
  window.Home = Home;
})();
