/* ParentApp — shell: header, routing, tab bar, sheets, toast. */
(function () {
  const { useState, useRef } = React;

  const TABS = [
    { id: "home",          label: "Inicio",     icon: "home" },
    { id: "attendance",    label: "Asistencia", icon: "calendar-check" },
    { id: "grades",        label: "Notas",      icon: "award" },
    { id: "balance",       label: "Cuenta",     icon: "wallet" },
    { id: "announcements", label: "Avisos",     icon: "megaphone" },
    { id: "more",          label: "Más",        icon: "grid" },
  ];

  function ParentApp({ theme = "light", device = "ios", skin = "classic", onToggleTheme,
                       initialScreen = "home", initialLoggedIn = true, levelId = "primaria" }) {
    const D = window.GLDATA;
    const [loggedIn, setLoggedIn] = useState(initialLoggedIn);
    const [tab, setTab] = useState(TABS.some(t => t.id === initialScreen) ? initialScreen : "home");
    const [detail, setDetail] = useState(
      initialScreen === "ann" ? { type: "ann", data: D.announcements[0] } :
      initialScreen === "more-detail" ? { type: "module", data: D.modules[0] } : null);
    const level = D.levels[levelId] || D.levels.primaria;

    const [overrides, setOverrides] = useState({});      // attendance excused
    const [readIds, setReadIds] = useState(new Set());   // announcements read
    const [paid, setPaid] = useState(false);

    const [sheet, setSheet] = useState(null);            // {type, data}
    const [reason, setReason] = useState(null);
    const [toast, setToast] = useState(null);
    const [scrolled, setScrolled] = useState(false);
    const toastTimer = useRef(null);

    const unread = D.announcements.filter(a => a.unread && !readIds.has(a.id)).length;

    // ---- navigation ----
    function go(target, params = {}) {
      if (target === "eventos") { setDetail({ type: "eventos" }); return; }
      setDetail(null);
      setTab(target);
      if (params.pay) setTimeout(() => openPay(), 260);
    }
    function back() { setDetail(null); }
    function selectTab(id) { setDetail(null); setTab(id); document.activeElement && document.activeElement.blur(); }

    function openAnnouncement(a) { markRead(a.id); setDetail({ type: "ann", data: a }); }
    function openModule(m) { setDetail({ type: "module", data: m }); }
    function openSubject(s) { setSheet({ type: "subject", data: s }); }
    function openExcuse(d) { setReason(null); setSheet({ type: "excuse", data: d }); }
    function openPay() { setSheet({ type: "pay" }); }
    function closeSheet() { setSheet(null); }

    function markRead(id) { setReadIds(prev => { const n = new Set(prev); n.add(id); return n; }); }
    function markAll() { setReadIds(new Set(D.announcements.map(a => a.id))); }

    function fireToast(msg, icon = "check-circle") {
      setToast({ msg, icon });
      clearTimeout(toastTimer.current);
      toastTimer.current = setTimeout(() => setToast(null), 2600);
    }

    function confirmExcuse() {
      const d = sheet.data;
      setOverrides(prev => ({ ...prev, [d.day]: "excused" }));
      closeSheet();
      fireToast("Justificante enviado a la escuela");
    }
    function confirmPay() {
      setPaid(true);
      closeSheet();
      fireToast("¡Pago realizado con éxito!");
    }

    // ---- header ----
    function renderHeader() {
      if (detail) {
        const title = detail.type === "ann" ? "Aviso"
          : detail.type === "module" ? detail.data.title
          : detail.type === "eventos" ? "Eventos" : "";
        return (
          <div className={"gl-header" + (scrolled ? " scrolled" : "")}>
            <button className="gl-iconbtn" onClick={back} aria-label="Atrás"><Icon name="chevron-left" size={22} /></button>
            <div className="gl-detail-title" style={{ fontSize: 19 }}>{title}</div>
          </div>
        );
      }
      return (
        <div className={"gl-header" + (scrolled ? " scrolled" : "")}>
          <div className="gl-header-id">
            <Avatar child={D.child} className="sm" size={42} radius={15} />
            <div style={{ minWidth: 0 }}>
              <div className="gl-greet">{D.child.short} · {level.group}</div>
              <div className="gl-childname">{D.child.name.split(" ").slice(0, 2).join(" ")}</div>
            </div>
          </div>
          <div className="gl-header-actions">
            <button className="gl-iconbtn" onClick={onToggleTheme} aria-label="Tema">
              <Icon name={theme === "dark" ? "sun" : "moon"} size={20} />
            </button>
            <button className="gl-iconbtn" onClick={() => selectTab("announcements")} aria-label="Avisos">
              <Icon name="bell" size={20} />
              {unread > 0 && <span className="gl-dot-badge">{unread}</span>}
            </button>
          </div>
        </div>
      );
    }

    // ---- body ----
    function renderBody() {
      if (detail) {
        if (detail.type === "ann") return <AnnouncementDetail D={D} ann={detail.data} />;
        if (detail.type === "module") return <ModuleDetail D={D} mod={detail.data} />;
        if (detail.type === "eventos") return <Eventos D={D} />;
      }
      switch (tab) {
        case "home": return <Home D={D} level={level} go={go} paid={paid} />;
        case "attendance": return <Attendance D={D} overrides={overrides} openExcuse={openExcuse} />;
        case "grades": return <Grades D={D} level={level} openSubject={openSubject} />;
        case "balance": return <Balance D={D} openPay={openPay} paid={paid} />;
        case "announcements": return <Announcements D={D} readIds={readIds} openAnnouncement={openAnnouncement} markAll={markAll} />;
        case "more": return <More D={D} openModule={openModule} go={go} onLogout={() => setLoggedIn(false)} />;
        default: return null;
      }
    }

    if (!loggedIn) {
      return (
        <div className="gl-app" data-theme={theme} data-device={device} data-skin={skin}>
          <div className="gl-scroll" style={{ paddingTop: device === "ios" ? 60 : 28 }}>
            <Login onLogin={() => { setLoggedIn(true); setTab("home"); }} />
          </div>
        </div>
      );
    }

    return (
      <div className="gl-app" data-theme={theme} data-device={device} data-skin={skin}>
        {renderHeader()}
        <div className="gl-scroll" onScroll={(e) => setScrolled(e.target.scrollTop > 6)}>
          {renderBody()}
        </div>

        {/* bottom tab bar */}
        <div className="gl-tabbar">
          {TABS.map(t => (
            <button key={t.id} className={"gl-tab" + (!detail && tab === t.id ? " active" : "")} onClick={() => selectTab(t.id)}>
              <span className="gl-tab-ico">
                <Icon name={t.icon} size={23} stroke={!detail && tab === t.id ? 2.4 : 2} />
                {t.id === "announcements" && unread > 0 && <span className="gl-tab-badge">{unread}</span>}
              </span>
              <span className="gl-tab-label">{t.label}</span>
            </button>
          ))}
        </div>

        {/* toast */}
        {toast && (
          <div className={"gl-toast show"}>
            <Icon name={toast.icon} size={22} stroke={2.4} /> {toast.msg}
          </div>
        )}

        {/* sheets */}
        <div className={"gl-sheet-scrim" + (sheet ? " open" : "")} onClick={closeSheet}>
          <div className="gl-sheet" onClick={(e) => e.stopPropagation()}>
            <div className="gl-sheet-grab" />
            {sheet && sheet.type === "excuse" && (
              <ExcuseSheet D={D} data={sheet.data} reason={reason} setReason={setReason} onConfirm={confirmExcuse} onCancel={closeSheet} />
            )}
            {sheet && sheet.type === "pay" && (
              <PaySheet D={D} onConfirm={confirmPay} onCancel={closeSheet} />
            )}
            {sheet && sheet.type === "subject" && (
              <SubjectSheet s={sheet.data} onClose={closeSheet} />
            )}
          </div>
        </div>
      </div>
    );
  }

  // ---- Eventos (agenda) detail ----
  function Eventos({ D }) {
    return (
      <div className="gl-screen-anim">
        <div className="gl-card"><div className="gl-list">
          {D.events.map((ev, i) => (
            <div className="gl-listrow" key={i}>
              <div className={"tint-" + ev.tint} style={{ width: 50, height: 50, borderRadius: 15, flexShrink: 0, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", lineHeight: 1 }}>
                <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 19 }}>{ev.day}</span>
                <span style={{ fontSize: 9, fontWeight: 800, opacity: .8, marginTop: 1 }}>{ev.mon}</span>
              </div>
              <div className="gl-listrow-main">
                <div className="gl-listrow-title" style={{ fontSize: 16 }}>{ev.title}</div>
                <div className="gl-listrow-sub">{ev.sub}</div>
              </div>
            </div>
          ))}
        </div></div>
      </div>
    );
  }

  // ---- Excuse sheet ----
  function ExcuseSheet({ D, data, reason, setReason, onConfirm, onCancel }) {
    return (
      <>
        <div className="gl-sheet-title">Justificar inasistencia</div>
        <div className="gl-sheet-sub">Falta del <b>{data.label}</b>. Elige el motivo y la escuela lo revisará.</div>
        <div style={{ margin: "16px 0 6px", fontSize: 12.5, fontWeight: 800, color: "var(--text-3)", letterSpacing: ".4px" }}>MOTIVO</div>
        {D.reasons.map(r => (
          <div key={r.id} className={"gl-reason" + (reason === r.id ? " sel" : "")} onClick={() => setReason(r.id)}>
            <div className="gl-reason-radio"><div className="gl-reason-dot" /></div>
            <div className="gl-reason-label">{r.label}</div>
          </div>
        ))}
        <button className="gl-btn primary" style={{ marginTop: 20, opacity: reason ? 1 : .5, pointerEvents: reason ? "auto" : "none" }} onClick={onConfirm}>
          <Icon name="check" size={20} stroke={2.6} /> Enviar justificante
        </button>
        <button className="gl-btn ghost sm" style={{ marginTop: 10 }} onClick={onCancel}>Cancelar</button>
      </>
    );
  }

  // ---- Pay sheet ----
  function PaySheet({ D, onConfirm, onCancel }) {
    const [method, setMethod] = useState("card");
    const methods = [
      { id: "card", icon: "credit-card", title: "Tarjeta", sub: "•••• 4242" },
      { id: "spei", icon: "arrow-up-right", title: "Transferencia SPEI", sub: "CLABE de la escuela" },
      { id: "oxxo", icon: "receipt", title: "Pago en OXXO", sub: "Referencia para tienda" },
    ];
    return (
      <>
        <div className="gl-sheet-title">Pagar colegiatura</div>
        <div className="gl-sheet-sub">Colegiatura de junio · vence 15 jun</div>
        <div style={{ textAlign: "center", margin: "18px 0 8px" }}>
          <div className="gl-balance-amount" style={{ fontSize: 40 }}><span className="gl-balance-cur">$</span>{D.balance.amount.toLocaleString("es-MX", { minimumFractionDigits: 2 })}</div>
        </div>
        <div style={{ margin: "8px 0 6px", fontSize: 12.5, fontWeight: 800, color: "var(--text-3)", letterSpacing: ".4px" }}>MÉTODO DE PAGO</div>
        {methods.map(m => (
          <div key={m.id} className={"gl-reason" + (method === m.id ? " sel" : "")} onClick={() => setMethod(m.id)} style={{ gap: 14 }}>
            <TintIcon name={m.icon} tint="forest" size={40} />
            <div style={{ flex: 1 }}>
              <div className="gl-reason-label">{m.title}</div>
              <div className="gl-hero-sub">{m.sub}</div>
            </div>
            <div className="gl-reason-radio"><div className="gl-reason-dot" /></div>
          </div>
        ))}
        <button className="gl-btn primary" style={{ marginTop: 20 }} onClick={onConfirm}>
          <Icon name="lock" size={19} /> Pagar {window.money(D.balance.amount)}
        </button>
        <button className="gl-btn ghost sm" style={{ marginTop: 10 }} onClick={onCancel}>Cancelar</button>
      </>
    );
  }

  // ---- Subject grade sheet ----
  function SubjectSheet({ s, onClose }) {
    const terms = s.terms || [];
    return (
      <>
        <div style={{ display: "flex", alignItems: "center", gap: 13, marginBottom: 6 }}>
          <TintIcon name={s.icon} tint={s.tint} size={50} radius={16} icon={26} />
          <div style={{ flex: 1 }}>
            <div className="gl-sheet-title" style={{ fontSize: 19 }}>{s.name}</div>
            <div className="gl-hero-sub">Promedio del periodo</div>
          </div>
          <div className="gl-grade-badge" style={{
            minWidth: 56, height: 56, fontSize: 24,
            background: s.grade >= 9 ? "var(--ok-bg)" : s.grade >= 8 ? "var(--primary-soft)" : s.grade >= 7 ? "var(--warn-bg)" : "var(--bad-bg)",
            color: s.grade >= 9 ? "var(--ok)" : s.grade >= 8 ? "var(--primary-ink)" : s.grade >= 7 ? "var(--warn)" : "var(--bad)",
          }}>{s.grade === 10 ? "10" : s.grade.toFixed(1)}</div>
        </div>
        <div style={{ margin: "14px 0 6px", fontSize: 12.5, fontWeight: 800, color: "var(--text-3)", letterSpacing: ".4px" }}>POR PARCIAL</div>
        <div className="gl-card"><div className="gl-list">
          {terms.map((t, i) => (
            <div className="gl-listrow" key={i}>
              <div className="gl-listrow-main"><div className="gl-listrow-title">{i + 1}.er parcial</div></div>
              <div style={{ flex: 1, maxWidth: 120 }}>
                <div className="gl-bar"><span style={{ width: (t / 10 * 100) + "%", background: t >= 9 ? "var(--ok)" : t >= 8 ? "var(--lime)" : "var(--warn)" }} /></div>
              </div>
              <div className="gl-amount" style={{ minWidth: 36, textAlign: "right" }}>{t === 10 ? "10" : t.toFixed(1)}</div>
            </div>
          ))}
        </div></div>
        <button className="gl-btn ghost" style={{ marginTop: 18 }} onClick={onClose}>Cerrar</button>
      </>
    );
  }

  Object.assign(window, { ParentApp, TABS });
})();
