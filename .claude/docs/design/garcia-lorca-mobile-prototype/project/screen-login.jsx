/* LOGIN — consistent with the web portal: logo, lime primary, rounded cards. */
(function () {
  const { useState } = React;

  function Brandmark({ size = 76 }) {
    return (
      <div style={{
        width: size, height: size, borderRadius: size * 0.32,
        background: "linear-gradient(150deg, #7AC400, #0A7A2F)",
        display: "flex", alignItems: "center", justifyContent: "center",
        boxShadow: "0 14px 30px -10px rgba(10,122,47,0.6)", position: "relative", overflow: "hidden",
      }}>
        {/* leaf glyph + initials placeholder mark */}
        <svg width={size} height={size} viewBox="0 0 76 76" style={{ position: "absolute", opacity: .22 }}>
          <path d="M16 60s-3-22 13-34 33-9 33-9 1.5 22-14 34S16 60 16 60Z" fill="#fff" />
        </svg>
        <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: size * 0.42, color: "#fff", letterSpacing: "1px", position: "relative" }}>GL</span>
      </div>
    );
  }

  function Login({ onLogin }) {
    const [show, setShow] = useState(false);
    return (
      <div className="gl-screen-anim" style={{ display: "flex", flexDirection: "column", minHeight: "100%", justifyContent: "center", padding: "12px 4px 30px" }}>
        <div style={{ textAlign: "center", marginBottom: 26 }}>
          <div style={{ display: "flex", justifyContent: "center", marginBottom: 18 }}><Brandmark /></div>
          <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 23, lineHeight: 1.15, letterSpacing: "-0.4px" }}>
            Centro Educativo<br />García Lorca
          </div>
          <div style={{ fontSize: 13.5, color: "var(--text-3)", fontWeight: 700, marginTop: 8 }}>
            Portal para padres y tutores
          </div>
        </div>

        <div style={{ marginBottom: 14 }}>
          <div className="gl-field-label">Correo electrónico</div>
          <div className="gl-field">
            <Icon name="user" size={20} />
            <input type="email" defaultValue="laura.diaz@gmail.com" placeholder="tu@correo.com" />
          </div>
        </div>

        <div style={{ marginBottom: 8 }}>
          <div className="gl-field-label">Contraseña</div>
          <div className="gl-field">
            <Icon name="lock" size={20} />
            <input type={show ? "text" : "password"} defaultValue="123456" placeholder="••••••••" />
            <button className="gl-field-eye" onClick={() => setShow((s) => !s)} aria-label="Mostrar contraseña">
              <Icon name={show ? "eye-off" : "eye"} size={20} />
            </button>
          </div>
        </div>

        <div style={{ textAlign: "right", margin: "4px 4px 18px" }}>
          <button style={{ fontSize: 13, fontWeight: 800, color: "var(--primary-ink)" }}>¿Olvidaste tu contraseña?</button>
        </div>

        <button className="gl-btn primary" onClick={onLogin}>
          Ingresar <Icon name="arrow-right" size={20} stroke={2.4} />
        </button>

        <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "18px 0" }}>
          <div style={{ flex: 1, height: 1, background: "var(--line)" }} />
          <span style={{ fontSize: 12, fontWeight: 700, color: "var(--text-3)" }}>o</span>
          <div style={{ flex: 1, height: 1, background: "var(--line)" }} />
        </div>

        <button className="gl-btn outline" onClick={onLogin}>
          <Icon name="lock" size={19} /> Entrar con Face ID
        </button>

        <div style={{ textAlign: "center", fontSize: 12.5, color: "var(--text-3)", fontWeight: 700, marginTop: 22 }}>
          ¿Problemas para entrar? Contacta a la escuela
        </div>
      </div>
    );
  }

  window.Login = Login;
})();
