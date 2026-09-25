"use client";

import { useEffect, useRef, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { DrawerShell } from "../../_design/DrawerShell";
import { Field, inputStyle } from "../../_design/form";
import { API_URL } from "../../api-url";
import { getPhotoBlob, StudentsError, verifyCard, type VerifiedCard } from "../../students/api";

const MISSING = "Escanea o escribe el código de la credencial.";
const UNKNOWN = "Esa credencial no está registrada.";
const NOT_A_CODE = "Ese código no es de una credencial.";
const FAILED = "No se pudo verificar la credencial. Intenta de nuevo.";

/**
 * Verificar credencial (task 13): the school's door. A USB QR reader types the
 * card's code and presses Enter; the drawer says whose card it is, with the
 * photo, and whether it still counts. The field empties and keeps the focus, so
 * the next pupil in line is one scan away.
 */
export function VerifyCardDrawer({ onClose }: { onClose: () => void }): ReactElement {
  const [code, setCode] = useState("");
  const [result, setResult] = useState<VerifiedCard | undefined>(undefined);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const [pending, setPending] = useState(false);
  const field = useRef<HTMLInputElement>(null);

  useEffect(() => field.current?.focus(), []);

  const verify = async (): Promise<void> => {
    if (pending) return;
    if (code.trim() === "") {
      setProblem(MISSING);
      field.current?.focus();
      return;
    }
    setPending(true);
    setProblem(undefined);
    try {
      setResult(await verifyCard(API_URL, code));
    } catch (err) {
      setResult(undefined);
      setProblem(refusal(err));
    } finally {
      setPending(false);
      setCode("");
      field.current?.focus();
    }
  };

  return (
    <DrawerShell
      title="Verificar credencial"
      subtitle="Escanea el código QR de la credencial"
      icon="qr"
      size="sm"
      onClose={onClose}
      onSubmit={() => void verify()}
      footer={
        <>
          <Btn kind="ghost" onClick={onClose}>Cerrar</Btn>
          <Btn kind="primary" onClick={() => void verify()}>Verificar</Btn>
        </>
      }
    >
      <div style={{ display: "grid", gap: 18 }}>
        <Field label="Código de la credencial" error={problem === MISSING ? problem : undefined}>
          {(id, describedBy) => (
            <input
              id={id}
              ref={field}
              aria-describedby={describedBy}
              value={code}
              onChange={(e) => {
                setCode(e.target.value);
                if (problem === MISSING) setProblem(undefined);
              }}
              autoComplete="off"
              spellCheck={false}
              style={inputStyle(problem === MISSING)}
            />
          )}
        </Field>
        {problem !== undefined && problem !== MISSING && (
          <p role="alert" style={{ padding: "12px 14px", borderRadius: "var(--r-md)", background: "var(--bad-bg)", color: "var(--bad-fg)", fontSize: 15 }}>
            {problem}
          </p>
        )}
        {result !== undefined && <Verified card={result} />}
      </div>
    </DrawerShell>
  );
}

function refusal(err: unknown): string {
  if (err instanceof StudentsError && err.status === 404) return UNKNOWN;
  if (err instanceof StudentsError && err.status === 400) return NOT_A_CODE;
  return FAILED;
}

/** Whose card it is, with the photo the staff compare with the face in front of them. */
function Verified({ card }: { card: VerifiedCard }): ReactElement {
  const [photo, setPhoto] = useState<string | undefined>(undefined);
  useEffect(() => {
    let url: string | undefined;
    let live = true;
    getPhotoBlob(API_URL, card.studentId)
      .then((blob) => {
        if (!live || blob === null) return;
        url = URL.createObjectURL(blob);
        setPhoto(url);
      })
      .catch(() => setPhoto(undefined));
    return () => {
      live = false;
      if (url !== undefined) URL.revokeObjectURL(url);
    };
  }, [card.studentId]);

  return (
    <div style={{ display: "flex", gap: 16, alignItems: "center", padding: 16, borderRadius: "var(--r-lg)", border: "1px solid var(--border)", background: "var(--surface)" }}>
      <div style={{ width: 88, height: 88, borderRadius: "50%", overflow: "hidden", flexShrink: 0, background: "var(--surface-2)", display: "grid", placeItems: "center" }}>
        {photo !== undefined ? (
          <img src={photo} alt={`Foto de ${card.name}`} style={{ width: "100%", height: "100%", objectFit: "cover" }} />
        ) : (
          <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 26, color: "var(--text-muted)" }}>{initialsOf(card.name)}</span>
        )}
      </div>
      <div style={{ minWidth: 0, display: "grid", gap: 4 }}>
        <p style={{ fontWeight: 700, fontSize: 18, color: "var(--text)" }}>{card.name}</p>
        <p style={{ fontSize: 14.5, color: "var(--text-muted)" }}>{[card.level, card.gradeGroup].filter((x) => x !== "").join(" · ")}</p>
        <p style={{ fontSize: 14.5, color: "var(--text-muted)" }}>Matrícula {card.enrollmentNumber}</p>
        <span
          style={{
            justifySelf: "start", marginTop: 4, padding: "4px 12px", borderRadius: 99, fontWeight: 700, fontSize: 14,
            background: card.valid ? "var(--ok-bg)" : "var(--bad-bg)", color: card.valid ? "var(--ok-fg)" : "var(--bad-fg)",
          }}
        >
          {card.valid ? "Vigente" : "No vigente"}
        </span>
      </div>
    </div>
  );
}

function initialsOf(name: string): string {
  return name.split(/\s+/).filter((p) => p !== "").slice(0, 2).map((p) => p[0]?.toUpperCase() ?? "").join("");
}
