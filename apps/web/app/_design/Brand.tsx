import Image from "next/image";
import type { CSSProperties, ReactElement } from "react";
import { COLOR, type ColorKey } from "./colors";

/** The official school logo (full lockup: emblem + "Centro Educativo García Lorca"). */
const LOGO_SRC = "/cropped-logo-centro-educativo-lorca-nuevo.png";
const LOGO_W = 299;
const LOGO_H = 168;

/**
 * Logo renders the real school logo at a given height, preserving aspect ratio.
 * The lockup already contains the wordmark, so no separate text is needed. On
 * dark surfaces use `LogoBadge` (a white backing) so the dark-green text reads.
 */
export function Logo({ height = 40 }: { height?: number }): ReactElement {
  return (
    <Image
      src={LOGO_SRC}
      alt="Centro Educativo García Lorca"
      width={Math.round((height * LOGO_W) / LOGO_H)}
      height={height}
      priority
      style={{ height, width: "auto", display: "block" }}
    />
  );
}

/** Logo on a white rounded backing, for dark backgrounds (e.g. the login panel). */
export function LogoBadge({ height = 44 }: { height?: number }): ReactElement {
  return (
    <div
      style={{
        display: "inline-flex",
        alignItems: "center",
        background: "#fff",
        borderRadius: 16,
        padding: "10px 16px",
        boxShadow: "0 6px 20px rgba(0,0,0,.18)"
      }}
    >
      <Logo height={height} />
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
