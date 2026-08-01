"use client";

import { useState, type CSSProperties, type ReactElement } from "react";
import { Btn, ThemeToggle } from "../_design/Controls";
import type { Theme } from "../theme/theme-script";

export interface ChangePasswordProps {
  theme: Theme;
  onToggle: () => void;
  onSubmit: (currentPassword: string, newPassword: string) => Promise<void>;
  error?: string;
  pending?: boolean;
}

const inp: CSSProperties = {
  width: "100%",
  height: 52,
  padding: "0 16px",
  borderRadius: "var(--r-md)",
  border: "1.5px solid var(--border)",
  background: "var(--surface)",
  color: "var(--text)",
  fontSize: 16,
  outline: "none"
};

const MIN_LEN = 8;

export function ChangePassword({ theme, onToggle, onSubmit, error, pending }: ChangePasswordProps): ReactElement {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");

  const tooShort = next.length > 0 && next.length < MIN_LEN;
  const mismatch = confirm.length > 0 && next !== confirm;
  const canSubmit = !pending && current.length > 0 && next.length >= MIN_LEN && next === confirm;

  return (
    <div className="app-root" style={{ minHeight: "100vh", display: "grid", placeItems: "center", padding: 40 }}>
      <div style={{ position: "absolute", top: 24, right: 28 }}>
        <ThemeToggle theme={theme} onToggle={onToggle} />
      </div>
      <div className="fade-up" style={{ width: "100%", maxWidth: 412 }}>
        <h2 style={{ fontSize: 28, fontWeight: 700, color: "var(--text)" }}>Crea tu contraseña</h2>
        <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 8 }}>
          Por seguridad, cambia la contraseña temporal antes de continuar.
        </p>
        <div style={{ marginTop: 22, display: "flex", flexDirection: "column", gap: 14 }}>
          <input aria-label="Contraseña actual" type="password" autoComplete="current-password" placeholder="Contraseña temporal" value={current} onChange={(e) => setCurrent(e.target.value)} style={inp} />
          <input aria-label="Nueva contraseña" type="password" autoComplete="new-password" placeholder="Nueva contraseña" value={next} onChange={(e) => setNext(e.target.value)} style={inp} />
          <input aria-label="Confirmar contraseña" type="password" autoComplete="new-password" placeholder="Confirmar contraseña" value={confirm} onChange={(e) => setConfirm(e.target.value)} style={inp} />
        </div>
        {tooShort ? <p style={{ color: "var(--text-muted)", fontSize: 13, marginTop: 10 }}>Mínimo {MIN_LEN} caracteres.</p> : null}
        {mismatch ? <p style={{ color: "var(--orange)", fontSize: 13, marginTop: 10 }}>Las contraseñas no coinciden.</p> : null}
        <Btn full icon="shield" onClick={() => void onSubmit(current, next)} disabled={!canSubmit} style={{ height: 54, fontSize: 17, marginTop: 20 }}>
          {pending ? "Guardando…" : "Guardar y entrar"}
        </Btn>
        {error ? <p role="alert" style={{ color: "var(--orange)", fontSize: 14, marginTop: 12, textAlign: "center" }}>{error}</p> : null}
      </div>
    </div>
  );
}
