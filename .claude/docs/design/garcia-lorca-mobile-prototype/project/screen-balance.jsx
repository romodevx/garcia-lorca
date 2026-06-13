/* BALANCE / ACCOUNT — big clear balance, traffic-light, movements. */
(function () {
  const KIND = {
    pago:      { icon: "check-circle", tint: "forest", chip: ["ok", "Pago"] },
    cargo:     { icon: "receipt",      tint: "orange", chip: ["warn", "Cargo"] },
    descuento: { icon: "sparkles",     tint: "purple", chip: ["purple", "Beca"] },
  };

  function Balance({ D, openPay, paid }) {
    const b = D.balance;
    if (paid) {
      return (
        <div className="gl-screen-anim">
          <div className="gl-detail-title" style={{ padding: "8px 4px 12px" }}>Cuenta</div>
          <div className="gl-card gl-balance-hero">
            <div className="gl-balance-cap">Saldo actual</div>
            <div className="gl-balance-amount" style={{ color: "var(--ok)" }}>
              <span className="gl-balance-cur">$</span>0.00
            </div>
            <div style={{ display: "flex", justifyContent: "center", marginTop: 12 }}>
              <Chip kind="ok" icon="check">Al corriente · ¡Gracias!</Chip>
            </div>
            <button className="gl-btn ghost sm" style={{ marginTop: 18 }}>
              <Icon name="document" size={18} /> Solicitar factura (CFDI)
            </button>
          </div>
          <SectionLabel>Movimientos recientes</SectionLabel>
          <div className="gl-card"><div className="gl-list">
            <div className="gl-listrow">
              <TintIcon name="check-circle" tint="forest" />
              <div className="gl-listrow-main">
                <div className="gl-listrow-title">Pago recibido</div>
                <div className="gl-listrow-sub">Colegiatura junio · hoy</div>
              </div>
              <div className="gl-listrow-trail">
                <div className="gl-amount pos">{money(-b.amount)}</div>
                <Chip kind="ok">Pagado</Chip>
              </div>
            </div>
            {b.movements.map((m, i) => <MovRow key={i} m={m} D={D} />)}
          </div></div>
        </div>
      );
    }
    return (
      <div className="gl-screen-anim">
        <div className="gl-detail-title" style={{ padding: "8px 4px 12px" }}>Cuenta</div>

        {/* balance hero */}
        <div className="gl-card gl-balance-hero">
          <div className="gl-balance-cap">Saldo por pagar</div>
          <div className="gl-balance-amount" style={{ color: "var(--text)" }}>
            <span className="gl-balance-cur">$</span>{b.amount.toLocaleString("es-MX", { minimumFractionDigits: 2 })}
          </div>
          <div style={{ display: "flex", justifyContent: "center", marginTop: 12 }}>
            <Chip kind="warn" icon="clock">{b.label} · {b.due}</Chip>
          </div>
          <button className="gl-btn primary" style={{ marginTop: 18 }} onClick={openPay}>
            <Icon name="credit-card" size={21} /> Pagar {money(b.amount)}
          </button>
          <button className="gl-btn ghost sm" style={{ marginTop: 10 }}>
            <Icon name="document" size={18} /> Solicitar factura (CFDI)
          </button>
        </div>

        {/* traffic-light legend */}
        <div className="gl-card gl-card-pad" style={{ marginTop: 12, display: "flex", justifyContent: "space-around" }}>
          {[["var(--ok)", "Al corriente"], ["var(--orange)", "Por vencer"], ["var(--bad)", "Vencido"]].map(([c, l], i) => (
            <div key={i} style={{ display: "flex", alignItems: "center", gap: 7, fontSize: 12.5, fontWeight: 700, color: "var(--text-2)" }}>
              <span style={{ width: 11, height: 11, borderRadius: "50%", background: c }} />
              {l}
            </div>
          ))}
        </div>

        <SectionLabel>Movimientos recientes</SectionLabel>
        <div className="gl-card">
          <div className="gl-list">
            {b.movements.map((m, i) => <MovRow key={i} m={m} D={D} />)}
          </div>
        </div>
      </div>
    );
  }

  function MovRow({ m, D }) {
    const k = KIND[m.kind];
    return (
      <div className="gl-listrow">
        <TintIcon name={k.icon} tint={k.tint} />
        <div className="gl-listrow-main">
          <div className="gl-listrow-title">{m.title}</div>
          <div className="gl-listrow-sub">{m.sub} · {m.date}</div>
        </div>
        <div className="gl-listrow-trail">
          <div className={"gl-amount " + (m.amount < 0 ? "pos" : "neg")}>{money(m.amount)}</div>
          {m.paid && <Chip kind="ok">Pagado</Chip>}
        </div>
      </div>
    );
  }
  window.Balance = Balance;
})();
