import type { CSSProperties, ReactElement } from "react";
import { COLOR, type ColorKey } from "./colors";

/** Brand emblem: colorful 6-petal rosette with the GL monogram. */
export function Emblem({ size = 44 }: { size?: number }): ReactElement {
  const petals: ReadonlyArray<{ a: number; c: string }> = [
    { a: -90, c: "var(--lime)" },
    { a: -30, c: "var(--forest)" },
    { a: 30, c: "var(--yellow)" },
    { a: 90, c: "var(--orange)" },
    { a: 150, c: "var(--purple)" },
    { a: 210, c: "var(--lime-deep)" }
  ];
  const R = 50;
  const pr = 26;
  const dist = 22;
  return (
    <svg width={size} height={size} viewBox="0 0 100 100" aria-label="García Lorca">
      <g>
        {petals.map((p, i) => {
          const rad = (p.a * Math.PI) / 180;
          return (
            <circle
              key={i}
              cx={R + Math.cos(rad) * dist}
              cy={R + Math.sin(rad) * dist}
              r={pr}
              fill={p.c}
              opacity="0.92"
            />
          );
        })}
      </g>
      <circle cx={R} cy={R} r="24" fill="var(--surface)" />
      <text
        x="50"
        y="50"
        textAnchor="middle"
        dominantBaseline="central"
        fontFamily="var(--font-display)"
        fontWeight="800"
        fontSize="26"
        fill="var(--forest)"
      >
        GL
      </text>
    </svg>
  );
}

export function Wordmark({ size = 44 }: { size?: number }): ReactElement {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
      <Emblem size={size} />
      <div style={{ lineHeight: 1.05 }}>
        <div
          style={{
            fontFamily: "var(--font-display)",
            fontWeight: 800,
            fontSize: size * 0.36,
            color: "var(--text)",
            letterSpacing: "-.02em"
          }}
        >
          García Lorca
        </div>
        <div style={{ fontSize: size * 0.24, color: "var(--text-muted)", fontWeight: 600, marginTop: 2 }}>
          Centro Educativo
        </div>
      </div>
    </div>
  );
}

export function Avatar({
  name,
  color = "lime",
  size = 40,
  ring = false
}: {
  name: string;
  color?: ColorKey;
  size?: number;
  ring?: boolean;
}): ReactElement {
  const ini = name
    .split(" ")
    .slice(0, 2)
    .map((w) => w[0])
    .join("");
  const c = COLOR[color] ?? COLOR.lime;
  const style: CSSProperties = {
    width: size,
    height: size,
    borderRadius: "50%",
    flexShrink: 0,
    background: c.v,
    color: c.ink,
    display: "grid",
    placeItems: "center",
    fontFamily: "var(--font-display)",
    fontWeight: 700,
    fontSize: size * 0.38,
    boxShadow: ring ? "0 0 0 3px var(--surface), 0 0 0 5px var(--lime)" : "none"
  };
  return <div style={style}>{ini}</div>;
}
