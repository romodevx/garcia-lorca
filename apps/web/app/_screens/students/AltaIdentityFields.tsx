"use client";

import type { ReactElement } from "react";

import { DateField, TextField } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import { todayISO } from "../attendance/sheetModel";
import { CurpNotices } from "./CurpNotices";
import type { AltaForm } from "./useAltaForm";

/**
 * Who the pupil is. Once they exist (only the placement failed), every field
 * here is read-only: the retry places them and nothing else, so an edit here
 * would be silently dropped.
 */
export function AltaIdentityFields({ form }: { form: AltaForm }): ReactElement {
  const { draft, set, errors, clearError, created } = form;
  return (
    <>
      <TextField
        label="CURP"
        value={draft.curp}
        onChange={(v) => set("curp", v)}
        uppercase
        required
        readOnly={created}
        help="Los 18 caracteres de la CURP del alumno, tal como vienen en el acta. Es lo que impide que el mismo alumno se registre dos veces."
        error={errors.curp}
        onClearError={() => clearError("curp")}
      />
      <CurpNotices curp={draft.curp} birthDate={draft.birthDate} />
      {/* Surnames first, as the school reads its lists (T3). The two surnames
          are one logical entity, so they may sit side by side. */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))", gap: 14 }}>
        <TextField
          label="Apellido paterno"
          value={draft.lastName}
          onChange={(v) => set("lastName", v)}
          uppercase
          required
          readOnly={created}
          help="El primer apellido del alumno, el del padre."
          error={errors.lastName}
          onClearError={() => clearError("lastName")}
        />
        <TextField
          label="Apellido materno"
          value={draft.maternalLastName}
          onChange={(v) => set("maternalLastName", v)}
          uppercase
          readOnly={created}
          help="El segundo apellido, el de la madre. Déjalo vacío si el acta no lo trae."
        />
      </div>
      <TextField
        label="Nombre(s)"
        value={draft.firstName}
        onChange={(v) => set("firstName", v)}
        uppercase
        required
        readOnly={created}
        help="Sólo los nombres de pila, sin apellidos."
        error={errors.firstName}
        onClearError={() => clearError("firstName")}
      />
      <TextField
        label="Tutor"
        value={draft.guardianName}
        onChange={(v) => set("guardianName", v)}
        uppercase
        required
        readOnly={created}
        help="Quién responde por el alumno ante el colegio: nombre completo de la madre, el padre o el tutor legal."
        error={errors.guardianName}
        onClearError={() => clearError("guardianName")}
      />
      <MoreData form={form} />
    </>
  );
}

/** T11: the 80% case creates the pupil without a birth date, so it starts collapsed. */
function MoreData({ form }: { form: AltaForm }): ReactElement {
  const { draft, showMore, setShowMore } = form;
  return (
    <div>
      <button
        type="button"
        aria-expanded={showMore}
        aria-controls="alta-mas-datos"
        onClick={() => setShowMore(!showMore)}
        style={{ display: "inline-flex", alignItems: "center", gap: 6, minHeight: 44, padding: "0 4px", background: "none", border: "none", color: "var(--forest)", fontSize: 14.5, fontWeight: 700, cursor: "pointer" }}
      >
        <span style={{ display: "inline-flex", transform: showMore ? "rotate(90deg)" : "none", transition: "transform .15s" }}>
          <Icon name="chevron" size={16} stroke={2.4} />
        </span>
        Más datos (opcional)
      </button>
      {showMore && (
        <div id="alta-mas-datos" style={{ marginTop: 8 }}>
          <DateField
            label="Fecha de nacimiento"
            value={draft.birthDate}
            onChange={(v) => form.set("birthDate", v)}
            max={todayISO()}
            readOnly={form.created}
            error={form.errors.birthDate}
            onClearError={() => form.clearError("birthDate")}
            help="Con ella el sistema felicita al alumno en la app de sus papás el día de su cumpleaños. Se puede capturar después en su ficha."
          />
        </div>
      )}
    </div>
  );
}
