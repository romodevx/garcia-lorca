"use client";

import { useEffect, useMemo, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { DrawerShell } from "../../_design/DrawerShell";
import {
  TextArea,
  TextField,
  useFocusFirstInvalid,
  useFormatError,
  useRequired,
  type FormatRule,
} from "../../_design/form";
import { API_URL } from "../../api-url";
import { ConfigError, getSchoolProfile, saveSchoolProfile, type SchoolProfile } from "../../config/api";
import { ErrorAlert, Section } from "./parts";
import { useLatest } from "./useLatest";

const EMPTY: SchoolProfile = { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "" };

// The shapes the domain checks (config/domain/school_profile.go), said in the
// office's words. Checked on blur and again on save; the server is the last word.
const RULES: Readonly<Record<string, FormatRule>> = {
  rfc: (v) => (/^[A-ZÑ&]{3,4}[0-9]{6}[A-Z0-9]{3}$/.test(v.trim().toUpperCase()) ? undefined : "El RFC no tiene la forma del SAT (12 o 13 caracteres)."),
  cct: (v) => (/^[0-9]{2}[A-Z]{3}[0-9]{4}[A-Z]$/.test(v.trim().toUpperCase()) ? undefined : "La CCT no tiene la forma de la SEP (10 caracteres, p. ej. 09PPR1234X)."),
  phone: (v) => (v.replace(/\D/g, "").length === 10 ? undefined : "El teléfono lleva 10 dígitos."),
};

const REQUIRED = ["name"] as const;

export interface SchoolProfilePanelProps {
  /**
   * Notify-only: the profile once read (empty when the school has none yet) and
   * again after each save, or "failed". The setup view marks its step with it
   * instead of reading the profile twice.
   */
  onLoaded?: (profile: SchoolProfile | "failed") => void;
}

/**
 * Datos del colegio (task 12): what every receipt prints about the school. One
 * set per school, so it is a summary with "Editar", and a drawer with its seven
 * fields (the forms rule: 5–8 fields is a drawer).
 */
export function SchoolProfilePanel({ onLoaded }: SchoolProfilePanelProps = {}): ReactElement {
  const [profile, setProfile] = useState<SchoolProfile | undefined>(undefined);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);
  const [editing, setEditing] = useState(false);
  const notify = useLatest(onLoaded);

  useEffect(() => {
    getSchoolProfile(API_URL)
      .then((read) => {
        const loaded = read.found ? read.profile : EMPTY;
        setProfile(loaded);
        notify.current?.(loaded);
      })
      .catch(() => {
        setLoadError("No se pudieron leer los datos del colegio.");
        notify.current?.("failed");
      });
  }, [notify]);

  const filled = profile !== undefined && profile.name !== "";
  return (
    <Section title="Datos del colegio" subtitle="Lo que imprime cada comprobante">
      {loadError !== undefined && <ErrorAlert message={loadError} />}
      {profile !== undefined && (
        <div style={{ display: "flex", alignItems: "center", gap: 14, flexWrap: "wrap" }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            {filled ? (
              <>
                <p style={{ fontWeight: 700, fontSize: 16, color: "var(--text)" }}>{profile.name}</p>
                <p style={{ fontSize: 14, color: "var(--text-muted)" }}>{summaryOf(profile)}</p>
              </>
            ) : (
              <p style={{ fontSize: 14.5, color: "var(--text-muted)" }}>
                Aún no hay datos del colegio: los comprobantes saldrán sin nombre, RFC ni dirección.
              </p>
            )}
          </div>
          <Btn kind={filled ? "outline" : "primary"} onClick={() => setEditing(true)}>
            {filled ? "Editar datos" : "Capturar datos"}
          </Btn>
        </div>
      )}
      {editing && profile !== undefined && (
        <SchoolProfileDrawer
          initial={profile}
          onClose={() => setEditing(false)}
          onSaved={(saved) => {
            setProfile(saved);
            setEditing(false);
            notify.current?.(saved);
          }}
        />
      )}
    </Section>
  );
}

/** One line under the name: what is filled, in the order a receipt prints it. */
function summaryOf(p: SchoolProfile): string {
  const parts = [p.rfc !== "" ? `RFC ${p.rfc}` : "", p.cct !== "" ? `CCT ${p.cct}` : "", p.phone, p.address];
  const shown = parts.filter((x) => x !== "");
  return shown.length > 0 ? shown.join(" · ") : "Sin RFC, CCT, teléfono ni dirección";
}

function SchoolProfileDrawer({
  initial,
  onClose,
  onSaved,
}: {
  initial: SchoolProfile;
  onClose: () => void;
  onSaved: (saved: SchoolProfile) => void;
}): ReactElement {
  const [form, setForm] = useState<SchoolProfile>(initial);
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const required = useRequired(REQUIRED);
  const format = useFormatError(RULES);
  const focusFirstInvalid = useFocusFirstInvalid();
  const values = useMemo<Record<string, string>>(() => ({ ...form }), [form]);

  const edit = (key: keyof SchoolProfile) => (value: string): void => {
    const next = { ...form, [key]: value };
    setForm(next);
    format.onChange(key, value, next);
  };
  const blur = (key: keyof SchoolProfile) => (): void => format.onBlur(key, form[key], values);

  const save = async (): Promise<void> => {
    if (pending) return;
    const complete = required.check(values);
    const wellFormed = format.checkAll(values);
    if (!complete || !wellFormed) {
      focusFirstInvalid();
      return;
    }
    setPending(true);
    setFailure(undefined);
    try {
      onSaved(await saveSchoolProfile(API_URL, form));
    } catch (err) {
      setFailure(err instanceof ConfigError && err.isInvalid ? `Revisa los datos: ${err.detail}` : "No se pudieron guardar los datos.");
    } finally {
      setPending(false);
    }
  };

  return (
    <DrawerShell
      title="Datos del colegio"
      subtitle="Salen en cada comprobante"
      icon="settings"
      onClose={onClose}
      onSubmit={() => void save()}
      footer={
        <>
          <Btn kind="ghost" onClick={onClose}>Cancelar</Btn>
          <Btn kind="primary" onClick={() => void save()}>Guardar datos</Btn>
        </>
      }
    >
      <div style={{ display: "grid", gap: 16 }}>
        <TextField label="Nombre del colegio" required uppercase value={form.name} onChange={edit("name")}
          error={required.errors.name} onClearError={() => required.clear("name")} />
        <TextField label="Razón social" uppercase value={form.legalName} onChange={edit("legalName")} />
        <TextField label="RFC" uppercase value={form.rfc} onChange={edit("rfc")} onBlur={blur("rfc")} error={format.errors.rfc}
          help="El de la razón social: 12 caracteres si es persona moral." />
        <TextField label="CCT (clave de centro de trabajo)" uppercase value={form.cct} onChange={edit("cct")} onBlur={blur("cct")}
          error={format.errors.cct} help="La clave de la SEP, 10 caracteres." />
        <TextField label="Dirección" uppercase value={form.address} onChange={edit("address")} />
        <TextField label="Teléfono" value={form.phone} onChange={edit("phone")} onBlur={blur("phone")} error={format.errors.phone} />
        <TextArea label="Leyenda al pie del comprobante" value={form.legend} onChange={edit("legend")} />
        {failure !== undefined && <ErrorAlert message={failure} />}
      </div>
    </DrawerShell>
  );
}
