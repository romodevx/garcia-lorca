/* MORE — module grid + account list, plus per-module detail. */
(function () {
  function More({ D, openModule, go, onLogout }) {
    const { child, modules } = D;
    return (
      <div className="gl-screen-anim">
        <div className="gl-detail-title" style={{ padding: "8px 4px 14px" }}>Más</div>

        {/* student profile card */}
        <div className="gl-card gl-card-pad" style={{ display: "flex", alignItems: "center", gap: 14 }}>
          <Avatar child={child} size={52} radius={18} />
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 17 }}>{child.name}</div>
            <div className="gl-hero-sub" style={{ marginTop: 2 }}>{child.teacher}</div>
          </div>
          <span className="gl-level-chip" style={{ height: 26 }}>{child.short}</span>
        </div>

        <SectionLabel>Módulos</SectionLabel>
        <div className="gl-modgrid">
          {modules.map((m) => (
            <div className="gl-mod tap" key={m.id} onClick={() => openModule(m)}>
              <TintIcon name={m.icon} tint={m.tint} size={48} radius={15} icon={26} />
              <div>
                <div className="gl-mod-title">{m.title}</div>
                <div className="gl-mod-sub">{m.sub}</div>
              </div>
            </div>
          ))}
        </div>

        <SectionLabel>Cuenta</SectionLabel>
        <div className="gl-card">
          <div className="gl-list">
            {[["user", "Perfil del alumno"], ["bell", "Notificaciones"], ["phone", "Ayuda y soporte"]].map(([ic, t], i) => (
              <div className="gl-listrow tap" key={i}>
                <TintIcon name={ic} tint="forest" />
                <div className="gl-listrow-main"><div className="gl-listrow-title">{t}</div></div>
                <Go />
              </div>
            ))}
            <div className="gl-listrow tap" onClick={onLogout}>
              <div className="gl-listrow-icon tint-red"><Icon name="lock" size={20} /></div>
              <div className="gl-listrow-main"><div className="gl-listrow-title" style={{ color: "var(--bad)" }}>Cerrar sesión</div></div>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // Per-module detail content (back header rendered by shell).
  function ModuleDetail({ D, mod }) {
    const inner = {
      documentos: (
        <div className="gl-card"><div className="gl-list">
          {[["Boleta 2.º periodo", "PDF · 240 KB"], ["Constancia de estudios", "PDF · 180 KB"], ["Reglamento escolar", "PDF · 1.2 MB"]].map(([t, s], i) => (
            <div className="gl-listrow tap" key={i}>
              <TintIcon name="document" tint="forest" />
              <div className="gl-listrow-main"><div className="gl-listrow-title">{t}</div><div className="gl-listrow-sub">{s}</div></div>
              <span className="gl-go"><Icon name="download" size={20} /></span>
            </div>
          ))}
        </div></div>
      ),
      uniformes: (
        <>
          <div className="gl-card gl-card-pad" style={{ display: "flex", gap: 12, alignItems: "center" }}>
            <TintIcon name="shirt" tint="lime" size={48} radius={15} icon={26} />
            <div><div style={{ fontWeight: 800, fontSize: 15 }}>Tallas registradas</div><div className="gl-hero-sub">Playera 10 · Pants 12 · Calzado 24</div></div>
          </div>
          <button className="gl-btn primary" style={{ marginTop: 14 }}><Icon name="arrow-up-right" size={19} /> Ir a la tienda de uniformes</button>
        </>
      ),
      facturas: (
        <div className="gl-card"><div className="gl-list">
          {[["Colegiatura mayo", "CFDI · 02 jun"], ["Colegiatura abril", "CFDI · 02 may"], ["Inscripción", "CFDI · 12 ago"]].map(([t, s], i) => (
            <div className="gl-listrow tap" key={i}>
              <TintIcon name="receipt" tint="orange" />
              <div className="gl-listrow-main"><div className="gl-listrow-title">{t}</div><div className="gl-listrow-sub">{s}</div></div>
              <span className="gl-go"><Icon name="download" size={20} /></span>
            </div>
          ))}
        </div></div>
      ),
      becas: (
        <>
          <div className="gl-card gl-balance-hero">
            <div className="gl-balance-cap">Beca aplicada</div>
            <div className="gl-balance-amount" style={{ color: "var(--purple)", fontSize: 40 }}>23%</div>
            <div style={{ display: "flex", justifyContent: "center", marginTop: 12 }}><Chip kind="purple" icon="check">Vigente · Ciclo 2025–2026</Chip></div>
          </div>
          <div className="gl-card gl-card-pad" style={{ marginTop: 12 }}>
            <div style={{ fontSize: 13.5, color: "var(--text-2)", fontWeight: 600, lineHeight: 1.5 }}>
              Tu beca por desempeño académico se renueva manteniendo un promedio mínimo de 9.0. Próxima revisión: julio 2026.
            </div>
          </div>
        </>
      ),
    };
    return <div className="gl-screen-anim">{inner[mod.id]}</div>;
  }

  Object.assign(window, { More, ModuleDetail });
})();
