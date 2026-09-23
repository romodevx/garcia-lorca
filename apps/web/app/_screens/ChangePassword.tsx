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

const etiqueta: CSSProperties = {
  display: "block",
  fontSize: 13,
  fontWeight: 600,
  color: "var(--text-muted)",
  marginBottom: 6,
  marginLeft: 4,
};

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
        {/*
          Las tres etiquetas se VEN. Antes el nombre de cada campo vivía en su
          placeholder, así que desaparecía en cuanto alguien empezaba a teclear:
          quien se distraía a mitad de la forma ya no sabía cuál de las tres
          casillas era la nueva y cuál la confirmación.
        */}
        <div style={{ marginTop: 22, display: "flex", flexDirection: "column", gap: 14 }}>
          <label style={{ display: "block" }}>
            <span style={etiqueta}>Contraseña temporal</span>
            <input type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} style={inp} />
          </label>
          <label style={{ display: "block" }}>
            <span style={etiqueta}>Nueva contraseña</span>
            <input type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} style={inp} />
          </label>
          <label style={{ display: "block" }}>
            <span style={etiqueta}>Confirmar contraseña</span>
            <input type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} style={inp} />
          </label>
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
