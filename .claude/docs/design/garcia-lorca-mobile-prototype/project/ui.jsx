/* Shared UI atoms for the parent app. Exposed on window. */
(function () {
  const { useState } = React;

  // Friendly initials avatar (photo placeholder).
  function Avatar({ child, size = 44, radius = 16, className = "" }) {
    return (
      <div className={"gl-avatar " + className} style={{
        width: size, height: size, borderRadius: radius,
        background: `linear-gradient(145deg, ${child.avatar.from}, ${child.avatar.to})`,
        display: "flex", alignItems: "center", justifyContent: "center",
      }}>
        <span style={{
          fontFamily: "var(--font-display)", fontWeight: 700,
          fontSize: size * 0.38, color: "#fff", letterSpacing: ".5px",
        }}>{child.initials}</span>
      </div>
    );
  }

  // Icon inside a tinted rounded tile.
  function TintIcon({ name, tint = "lime", size = 40, radius = 13, icon = 22 }) {
    return (
      <div className={"tint-" + tint} style={{
        width: size, height: size, borderRadius: radius, flexShrink: 0,
        display: "flex", alignItems: "center", justifyContent: "center",
      }}>
        <Icon name={name} size={icon} />
      </div>
    );
  }

  // Traffic-light status dot.
  function StatusDot({ status, size = 10 }) {
    const c = { present: "var(--ok)", paid: "var(--ok)", ok: "var(--ok)",
      warn: "var(--orange)", excused: "var(--warn)", retardo: "var(--warn)",
      absent: "var(--bad)", bad: "var(--bad)" }[status] || "var(--text-3)";
    return <span style={{ width: size, height: size, borderRadius: "50%", background: c, display: "inline-block", flexShrink: 0 }} />;
  }

  function Chip({ kind = "neutral", icon, children }) {
    return (
      <span className={"gl-chip " + kind}>
        {icon && <Icon name={icon} size={14} stroke={2.4} />}
        {children}
      </span>
    );
  }

  // Card chevron affordance
  function Go() { return <span className="gl-go"><Icon name="chevron-right" size={20} stroke={2.4} /></span>; }

  // Section label
  function SectionLabel({ children, style }) {
    return <div className="gl-section-label" style={style}>{children}</div>;
  }

  // SVG ring progress (for grade average)
  function Ring({ pct, color, track, size = 92, sw = 9, children }) {
    const r = (size - sw) / 2;
    const c = 2 * Math.PI * r;
    return (
      <div className="gl-avg-ring" style={{ width: size, height: size }}>
        <svg width={size} height={size} style={{ position: "absolute", transform: "rotate(-90deg)" }}>
          <circle cx={size/2} cy={size/2} r={r} fill="none" stroke={track} strokeWidth={sw} />
          <circle cx={size/2} cy={size/2} r={r} fill="none" stroke={color} strokeWidth={sw}
            strokeLinecap="round" strokeDasharray={c} strokeDashoffset={c * (1 - pct)}
            style={{ transition: "stroke-dashoffset .7s cubic-bezier(.22,.61,.36,1)" }} />
        </svg>
        <div style={{ position: "relative", textAlign: "center" }}>{children}</div>
      </div>
    );
  }

  function money(n) {
    const abs = Math.abs(n).toLocaleString("es-MX", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    return (n < 0 ? "-$" : "$") + abs;
  }

  Object.assign(window, { Avatar, TintIcon, StatusDot, Chip, Go, SectionLabel, Ring, money });
})();
