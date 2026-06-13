/* Page: interactive hero + side-by-side overview of every screen. */
(function () {
  const { useState, useEffect } = React;

  const W = 402, H = 874;

  function Phone({ device = "ios", theme = "light", skin = "modern", ...appProps }) {
    const dark = theme === "dark";
    const bg = dark
      ? (skin === "modern" ? "#051A0E" : "#04331A")
      : (skin === "modern" ? "#FFFFFF" : "#F5F7F5");
    const app = <ParentApp theme={theme} device={device} skin={skin} {...appProps} />;
    return device === "android"
      ? <AndroidDevice dark={dark} screenBg={bg}>{app}</AndroidDevice>
      : <IOSDevice dark={dark} screenBg={bg}>{app}</IOSDevice>;
  }

  function ScaledPhone({ scale = 0.5, ...props }) {
    return (
      <div style={{ width: W * scale, height: H * scale, overflow: "hidden", flexShrink: 0 }}>
        <div style={{ width: W, height: H, transform: `scale(${scale})`, transformOrigin: "top left" }}>
          <Phone {...props} />
        </div>
      </div>
    );
  }

  const SEG = (opts, val, set) => (
    <div className="pg-seg">
      {opts.map(o => (
        <button key={o.v} className={val === o.v ? "active" : ""} onClick={() => set(o.v)}>{o.l}</button>
      ))}
    </div>
  );

  const OVERVIEW = [
    { key: "login", screen: "home", loggedIn: false, label: "Acceso" },
    { key: "home", screen: "home", label: "Inicio" },
    { key: "attendance", screen: "attendance", label: "Asistencia" },
    { key: "grades", screen: "grades", level: "primaria", label: "Calificaciones · numérico" },
    { key: "gradesq", screen: "grades", level: "kinder", label: "Calificaciones · cualitativo" },
    { key: "balance", screen: "balance", label: "Cuenta / Saldo" },
    { key: "announcements", screen: "announcements", label: "Avisos" },
    { key: "more", screen: "more", label: "Más (módulos)" },
  ];

  function Page() {
    const [theme, setTheme] = useState(() => localStorage.getItem("gl_theme") || "light");
    const [device, setDevice] = useState(() => localStorage.getItem("gl_device") || "ios");
    const [levelId, setLevelId] = useState(() => localStorage.getItem("gl_level") || "primaria");
    const [skin, setSkin] = useState(() => localStorage.getItem("gl_skin") || "modern");
    useEffect(() => { localStorage.setItem("gl_theme", theme); }, [theme]);
    useEffect(() => { localStorage.setItem("gl_device", device); }, [device]);
    useEffect(() => { localStorage.setItem("gl_level", levelId); }, [levelId]);
    useEffect(() => { localStorage.setItem("gl_skin", skin); }, [skin]);

    const toggleTheme = () => setTheme(t => t === "light" ? "dark" : "light");

    return (
      <div className="pg">
        <header className="pg-head">
          <div className="pg-mark">GL</div>
          <div>
            <h1>Centro Educativo García Lorca</h1>
            <p>App para padres y tutores · prototipo interactivo iOS / Android</p>
          </div>
        </header>

        {/* Interactive hero */}
        <section className="pg-hero">
          <div className="pg-controls">
            <div className="pg-ctl">
              <span className="pg-ctl-lab">Estilo</span>
              {SEG([{ v: "modern", l: "Moderno" }, { v: "classic", l: "Clásico" }], skin, setSkin)}
            </div>
            <div className="pg-ctl">
              <span className="pg-ctl-lab">Dispositivo</span>
              {SEG([{ v: "ios", l: "iPhone" }, { v: "android", l: "Android" }], device, setDevice)}
            </div>
            <div className="pg-ctl">
              <span className="pg-ctl-lab">Tema</span>
              {SEG([{ v: "light", l: "Claro" }, { v: "dark", l: "Oscuro" }], theme, setTheme)}
            </div>
            <div className="pg-ctl">
              <span className="pg-ctl-lab">Nivel del alumno <em>(afecta calificaciones)</em></span>
              {SEG([{ v: "kinder", l: "Kínder" }, { v: "primaria", l: "Primaria" }, { v: "secundaria", l: "Secundaria" }], levelId, setLevelId)}
            </div>
          </div>

          <div className="pg-stage">
            <Phone device={device} theme={theme} skin={skin} onToggleTheme={toggleTheme}
              levelId={levelId} initialScreen="home" initialLoggedIn={true} />
          </div>
          <p className="pg-hint">Es completamente interactivo — navega con la barra inferior, toca las tarjetas, justifica una falta, paga el saldo, abre un aviso.</p>
        </section>

        {/* Overview */}
        <section className="pg-overview">
          <h2>Todas las pantallas · claro y oscuro</h2>
          <p className="pg-sub">Estilo <b>{skin === "modern" ? "Moderno" : "Clásico"}</b>. El prototipo de arriba permite cambiar de estilo, dispositivo y tema en vivo.</p>
          <div className="pg-grid">
            {OVERVIEW.map(o => (
              <div className="pg-cell" key={o.key}>
                <div className="pg-cell-label">{o.label}</div>
                <div className="pg-pair">
                  <div className="pg-one">
                    <ScaledPhone scale={0.46} device="ios" theme="light" skin={skin}
                      initialScreen={o.screen} initialLoggedIn={o.loggedIn !== false} levelId={o.level || "primaria"} onToggleTheme={() => {}} />
                    <span className="pg-tag">Claro</span>
                  </div>
                  <div className="pg-one">
                    <ScaledPhone scale={0.46} device="ios" theme="dark" skin={skin}
                      initialScreen={o.screen} initialLoggedIn={o.loggedIn !== false} levelId={o.level || "primaria"} onToggleTheme={() => {}} />
                    <span className="pg-tag">Oscuro</span>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </section>

        <footer className="pg-foot">García Lorca · prototipo de diseño · paleta del logotipo de la escuela</footer>
      </div>
    );
  }

  ReactDOM.createRoot(document.getElementById("root")).render(<Page />);
})();
