"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type FormEvent,
  type KeyboardEvent,
  type ReactElement,
  type ReactNode,
} from "react";

import { API_URL } from "../../api-url";
import { Btn } from "../../_design/Controls";
import { CreatePage } from "../../_design/CreatePage";
import {
  DateField,
  EmailField,
  SelectField,
  TextArea,
  TextField,
  useEmailShapeOnSave,
  useFocusFirstInvalid,
  useFormatError,
  useRequired,
} from "../../_design/form";
import {
  deletePhoto,
  getMedical,
  getPhotoBlob,
  getStudent,
  StudentsError,
  updateMedical,
  updateStudent,
  uploadPhoto,
  type MedicalRecord,
  type PhoneKind,
  type Sex,
  type Student,
  type StudentAddress,
  type StudentPhone,
} from "../../students/api";
import { resizeToJpeg } from "../../students/photoResize";
import { birthDateError, MEXICAN_STATES, phoneDigits, phoneError, postalCodeError } from "../../students/profileRules";
import { ContactsPanel } from "./ContactsPanel";
import { StudentNotesPanel } from "../../_design/students/StudentNotesPanel";
import { useStudentNotes } from "../../_design/students/useStudentNotes";
import { formatStudentName } from "../../_design/students/studentName";

/**
 * The whole file of one pupil, corrected on its own page (`/alumnos/{id}/editar`).
 *
 * More than eight fields in five sections: by the forms rule that is a page,
 * never a drawer. One column, one "Guardar ficha" that sends everything the
 * page holds — names, profile, photo and, for whoever may, the medical record.
 * Contacts save on their own row, as they always have: each is its own record.
 */
export interface StudentEditScreenProps {
  studentId: string;
  /** The signed-in account: it may void its own notes for 15 minutes. */
  userId: string;
  permissions?: readonly string[];
  onBack: () => void;
  /** The school's date (aaaa-mm-dd); injected by tests. Defaults to the local day. */
  today?: string;
}

const MAX_PHONES = 3;
const REQUIRED = ["lastName", "firstName", "curp"] as const;
const ACCEPTED_PHOTO_TYPES = ["image/jpeg", "image/png"];
const BLOOD_TYPES = ["A+", "A-", "B+", "B-", "AB+", "AB-", "O+", "O-"] as const;

const SEX_OPTIONS = [
  { value: "F", label: "Femenino" },
  { value: "M", label: "Masculino" },
  { value: "X", label: "X (no binario)" },
] as const;

const PHONE_KINDS: { value: PhoneKind; label: string }[] = [
  { value: "celular", label: "Celular" },
  { value: "casa", label: "Casa" },
  { value: "trabajo", label: "Trabajo" },
  { value: "otro", label: "Otro" },
];

const EMPTY_ADDRESS: StudentAddress = {
  street: "",
  extNumber: "",
  intNumber: "",
  neighborhood: "",
  postalCode: "",
  municipality: "",
  state: "",
};

const EMPTY_MEDICAL: MedicalRecord = { bloodType: "", allergies: "", conditions: "" };

const SECTIONS = [
  { id: "ficha-generales", title: "Datos generales" },
  { id: "ficha-foto", title: "Foto" },
  { id: "ficha-contacto", title: "Contacto y domicilio" },
  { id: "ficha-medicos", title: "Datos médicos" },
  { id: "ficha-contactos", title: "Tutores y emergencias" },
  { id: "ficha-notas", title: "Notas" },
] as const;

/** What happens to the photo when the file is saved. */
type PhotoChange = { kind: "keep" } | { kind: "replace"; image: Blob } | { kind: "remove" };

function localToday(): string {
  const now = new Date();
  const pad = (n: number): string => String(n).padStart(2, "0");
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}

function failureMessage(cause: unknown): string {
  if (cause instanceof StudentsError) {
    if (cause.isConflict) return "Ya hay otro alumno con ese CURP.";
    if (cause.isInvalid) return "El servidor no aceptó algún dato de la ficha. Revísala.";
    if (cause.status === 403) return "No tienes permiso para editar esta ficha.";
    if (cause.status === 413) return "La foto pesa más de 2 MB.";
  }
  return "No se pudo guardar la ficha. Intenta de nuevo.";
}

export function StudentEditScreen({ studentId, userId, permissions, onBack, today }: StudentEditScreenProps): ReactElement {
  const day = today ?? localToday();
  const canReadMedical = permissions?.includes("medical:read") === true;
  const canWriteMedical = canReadMedical && permissions?.includes("medical:write") === true;
  // The pupil's notes: the same source the list's modal and the payment counter read (T6).
  const notes = useStudentNotes(studentId);

  const [student, setStudent] = useState<Student | undefined>(undefined);
  const [loadFailed, setLoadFailed] = useState(false);

  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [maternalLastName, setMaternalLastName] = useState("");
  const [curp, setCurp] = useState("");
  const [birthDate, setBirthDate] = useState("");
  const [sex, setSex] = useState<Sex>("");
  const [email, setEmail] = useState("");
  const [phones, setPhones] = useState<StudentPhone[]>([]);
  const [address, setAddress] = useState<StudentAddress>(EMPTY_ADDRESS);

  const [savedPhoto, setSavedPhoto] = useState<Blob | null>(null);
  const [photoChange, setPhotoChange] = useState<PhotoChange>({ kind: "keep" });

  const [medical, setMedical] = useState<MedicalRecord>(EMPTY_MEDICAL);
  const [savedMedical, setSavedMedical] = useState<MedicalRecord>(EMPTY_MEDICAL);
  // No field is offered until the record arrives: typed into earlier, it would
  // be overwritten by the response.
  const [medicalState, setMedicalState] = useState<"loading" | "ready" | "failed">("loading");

  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [failure, setFailure] = useState<string | undefined>(undefined);

  const required = useRequired(REQUIRED);
  const emailShape = useEmailShapeOnSave();
  const focusFirstInvalid = useFocusFirstInvalid();
  const rules = useMemo(
    () => ({
      birthDate: (value: string) => birthDateError(value, day),
      postalCode: postalCodeError,
      phone0: phoneError,
      phone1: phoneError,
      phone2: phoneError,
    }),
    [day],
  );
  const format = useFormatError(rules);

  useEffect(() => {
    let live = true;
    getStudent(API_URL, studentId)
      .then((st) => {
        if (!live) return;
        const p = st.profile;
        setStudent(st);
        setFirstName(st.firstName);
        setLastName(st.lastName);
        setMaternalLastName(st.maternalLastName ?? "");
        setCurp(st.curp);
        setBirthDate(p?.birthDate ?? "");
        setSex(p?.sex ?? "");
        setEmail(p?.email ?? "");
        setPhones(p?.phones ?? []);
        setAddress({ ...EMPTY_ADDRESS, ...p?.address });
      })
      .catch(() => live && setLoadFailed(true));
    getPhotoBlob(API_URL, studentId)
      .then((blob) => live && setSavedPhoto(blob))
      .catch(() => undefined); // no photo is shown; the rest of the file still works
    if (canReadMedical) {
      getMedical(API_URL, studentId)
        .then((record) => {
          if (!live) return;
          setMedical(record);
          setSavedMedical(record);
          setMedicalState("ready");
        })
        .catch(() => live && setMedicalState("failed"));
    }
    return () => {
      live = false;
    };
  }, [studentId, canReadMedical]);

  const formatValues = useCallback(
    (): Record<string, string> => ({
      birthDate,
      postalCode: address.postalCode,
      phone0: phones[0]?.number ?? "",
      phone1: phones[1]?.number ?? "",
      phone2: phones[2]?.number ?? "",
    }),
    [birthDate, address.postalCode, phones],
  );

  const medicalChanged =
    medical.bloodType !== savedMedical.bloodType ||
    medical.allergies !== savedMedical.allergies ||
    medical.conditions !== savedMedical.conditions;

  const submit = async (): Promise<void> => {
    if (saving || student === undefined) return;
    // Every check runs, so everything wrong is marked at once.
    const complete = required.check({ firstName, lastName, curp });
    const shaped = emailShape.check(email);
    const formatted = format.checkAll(formatValues());
    if (!complete || !shaped || !formatted) {
      focusFirstInvalid();
      return;
    }
    setSaving(true);
    setSaved(false);
    setFailure(undefined);
    try {
      const updated = await updateStudent(API_URL, studentId, {
        firstName: firstName.trim(),
        lastName: lastName.trim(),
        maternalLastName: maternalLastName.trim(),
        curp: curp.trim(),
        profile: {
          birthDate,
          sex,
          email: email.trim(),
          phones: phones
            .filter((ph) => ph.number.trim() !== "")
            .map((ph) => ({ kind: ph.kind, number: phoneDigits(ph.number) })),
          address,
        },
      });
      setStudent(updated);
      await savePhoto();
      if (canWriteMedical && medicalChanged) {
        setSavedMedical(await updateMedical(API_URL, studentId, medical));
      }
      setSaved(true);
    } catch (cause) {
      setFailure(failureMessage(cause));
    } finally {
      setSaving(false);
    }
  };

  const savePhoto = async (): Promise<void> => {
    if (photoChange.kind === "replace") {
      await uploadPhoto(API_URL, studentId, photoChange.image);
      setSavedPhoto(photoChange.image);
    } else if (photoChange.kind === "remove") {
      await deletePhoto(API_URL, studentId);
      setSavedPhoto(null);
    }
    setPhotoChange({ kind: "keep" });
  };

  const onKeyDown = (e: KeyboardEvent): void => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      void submit();
    }
  };

  const touched = (): void => setSaved(false);

  if (loadFailed) {
    return (
      <div style={{ padding: 26 }}>
        <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 15, marginBottom: 14 }}>
          No pudimos abrir la ficha de este alumno.
        </p>
        <Btn kind="ghost" onClick={onBack}>Volver a alumnos</Btn>
      </div>
    );
  }
  if (student === undefined) {
    return <p style={{ padding: 26, color: "var(--text-muted)" }}>Cargando la ficha…</p>;
  }

  const fullName = formatStudentName({ firstName, lastName, maternalLastName });
  const shownPhoto = photoChange.kind === "replace" ? photoChange.image : photoChange.kind === "remove" ? null : savedPhoto;

  return (
    <CreatePage
      breadcrumb={["Alumnos", formatStudentName(student)]}
      title="Editar ficha"
      aside={<SectionIndex showMedical={canReadMedical} />}
      footer={
        // Wraps on a phone: at 360 px the three do not fit on one line, and
        // "Guardar ficha" was pushed off the screen.
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12, width: "100%" }}>
          <Btn kind="ghost" onClick={onBack}>Volver a alumnos</Btn>
          <div style={{ flex: "1 1 0", minWidth: 0 }} role="status">
            {saved && <span style={{ fontSize: 14, color: "var(--ok-fg)", fontWeight: 600 }}>Cambios guardados.</span>}
          </div>
          {/* Never disabled for an incomplete form: saving explains what is missing. */}
          <Btn icon="check" onClick={() => void submit()} style={{ marginLeft: "auto", opacity: saving ? 0.6 : 1 }}>
            {saving ? "Guardando…" : "Guardar ficha"}
          </Btn>
        </div>
      }
    >
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          void submit();
        }}
        onKeyDown={onKeyDown}
        onChange={touched}
        style={{ display: "flex", flexDirection: "column", gap: 16 }}
      >
        <Section id="ficha-generales" title="Datos generales">
          {/* Surnames first, as the school reads its lists (T3). A pupil
              captured before the maternal field existed may carry both
              surnames in "Apellido paterno": move the second one down here. */}
          <TextField label="Apellido paterno" value={lastName} onChange={setLastName} uppercase required error={required.errors.lastName} onClearError={() => required.clear("lastName")} help="El primer apellido, el del padre. Si aquí aparecen los dos, deja sólo el paterno y pasa el otro a «Apellido materno»." />
          <TextField label="Apellido materno" value={maternalLastName} onChange={setMaternalLastName} uppercase help="El segundo apellido, el de la madre. Déjalo vacío si el acta no lo trae." />
          <TextField label="Nombre(s)" value={firstName} onChange={setFirstName} uppercase required error={required.errors.firstName} onClearError={() => required.clear("firstName")} />
          <TextField label="CURP" value={curp} onChange={setCurp} uppercase required error={required.errors.curp} onClearError={() => required.clear("curp")} help="Los 18 caracteres de la CURP del alumno." />
          <DateField
            label="Fecha de nacimiento"
            value={birthDate}
            max={day}
            onChange={(v) => {
              setBirthDate(v);
              format.onChange("birthDate", v, {});
            }}
            onBlur={() => format.onBlur("birthDate", birthDate, {})}
            error={format.errors.birthDate}
          />
          <SelectField label="Sexo" value={sex} onChange={(v) => setSex(v as Sex)} options={SEX_OPTIONS} />
          {student.enrollmentNumber !== undefined && student.enrollmentNumber !== "" && (
            <div>
              <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>Matrícula</span>
              <p style={{ fontSize: 15, fontWeight: 600, color: "var(--text)", margin: "4px 0 0" }}>{student.enrollmentNumber}</p>
            </div>
          )}
          <div>
            <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>Nivel y grupo</span>
            <p style={{ fontSize: 15, fontWeight: 600, color: "var(--text)", margin: "4px 0 0" }}>{`${student.level} · ${student.gradeGroup}`}</p>
            <p style={{ fontSize: 13, color: "var(--text-faint)", margin: "2px 0 0" }}>El grupo se cambia desde la ficha del alumno, con «Cambiar de grupo».</p>
          </div>
        </Section>

        <Section id="ficha-foto" title="Foto">
          <PhotoEditor
            name={fullName}
            photo={shownPhoto}
            onChange={(change) => {
              setPhotoChange(change);
              touched();
            }}
          />
        </Section>

        <Section id="ficha-contacto" title="Contacto y domicilio">
          <EmailField label="Correo del alumno" value={email} onChange={(v) => { setEmail(v); emailShape.forgive(v); }} error={emailShape.error} help="Puede quedar vacío." />
          <PhoneList
            phones={phones}
            errors={format.errors}
            onChange={(next) => {
              setPhones(next);
              next.forEach((ph, i) => format.onChange(`phone${i}`, ph.number, {}));
            }}
            onBlur={(i, value) => format.onBlur(`phone${i}`, value, {})}
            onRemove={(i) => {
              setPhones((prev) => prev.filter((_, at) => at !== i));
              format.reset();
            }}
          />
          <AddressFields
            address={address}
            postalCodeError={format.errors.postalCode}
            onChange={(next) => {
              setAddress(next);
              format.onChange("postalCode", next.postalCode, {});
            }}
            onPostalCodeBlur={() => format.onBlur("postalCode", address.postalCode, {})}
          />
        </Section>

        {canReadMedical && (
          <Section id="ficha-medicos" title="Datos médicos">
            {medicalState === "failed" && (
              <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14 }}>No pudimos leer los datos médicos.</p>
            )}
            {medicalState === "loading" && <p style={{ fontSize: 14, color: "var(--text-muted)" }}>Cargando datos médicos…</p>}
            {medicalState === "ready" && <MedicalFields record={medical} disabled={!canWriteMedical} onChange={setMedical} />}
          </Section>
        )}
      </form>

      {/* Outside the form: each contact is its own record with its own save. */}
      <Section id="ficha-contactos" title="Tutores y emergencias">
        <ContactsPanel studentId={studentId} />
      </Section>

      {/* Each note is its own record with its own save, like the contacts. */}
      <Section id="ficha-notas" title="Notas">
        <StudentNotesPanel
          notes={notes}
          studentId={studentId}
          userId={userId}
          canWrite={permissions?.includes("students:notes_write") === true}
          canVoidAny={permissions?.includes("students:notes_void") === true}
        />
      </Section>

      {failure !== undefined && (
        <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14.5, fontWeight: 600 }}>{failure}</p>
      )}
    </CreatePage>
  );
}

function Section({ id, title, children }: { id: string; title: string; children: ReactNode }): ReactElement {
  return (
    <section
      id={id}
      aria-labelledby={`${id}-title`}
      style={{ display: "grid", gap: 14, padding: "18px 20px", background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)" }}
    >
      <h2 id={`${id}-title`} style={{ fontSize: 16, fontWeight: 800, color: "var(--text)" }}>{title}</h2>
      {children}
    </section>
  );
}

/** Jumps to a section; buttons, not links, so the route never changes. */
function SectionIndex({ showMedical }: { showMedical: boolean }): ReactElement {
  return (
    <nav aria-label="Secciones de la ficha" style={{ display: "grid", gap: 4 }}>
      {SECTIONS.filter((s) => showMedical || s.id !== "ficha-medicos").map((s) => (
        <button
          key={s.id}
          type="button"
          onClick={() => document.getElementById(s.id)?.scrollIntoView({ behavior: "smooth", block: "start" })}
          style={{ textAlign: "left", minHeight: 44, padding: "0 12px", borderRadius: 10, fontSize: 14, fontWeight: 600, color: "var(--text-muted)", cursor: "pointer", background: "transparent" }}
        >
          {s.title}
        </button>
      ))}
    </nav>
  );
}

function PhoneList({
  phones,
  errors,
  onChange,
  onBlur,
  onRemove,
}: {
  phones: StudentPhone[];
  errors: Readonly<Record<string, string>>;
  onChange: (next: StudentPhone[]) => void;
  onBlur: (index: number, value: string) => void;
  onRemove: (index: number) => void;
}): ReactElement {
  const set = (index: number, patch: Partial<StudentPhone>): void =>
    onChange(phones.map((ph, at) => (at === index ? { ...ph, ...patch } : ph)));
  return (
    <div style={{ display: "grid", gap: 10 }}>
      {phones.map((ph, i) => (
        <div key={i} style={{ display: "flex", gap: 8, alignItems: "flex-end", flexWrap: "wrap" }}>
          <div style={{ flex: "1 1 120px", minWidth: 0 }}>
            <SelectField label={`Tipo de teléfono ${i + 1}`} value={ph.kind} onChange={(v) => set(i, { kind: v as PhoneKind })} options={PHONE_KINDS} />
          </div>
          <div style={{ flex: "2 1 160px", minWidth: 0 }}>
            <TextField
              label={`Teléfono ${i + 1}`}
              value={ph.number}
              onChange={(v) => set(i, { number: v })}
              onBlur={() => onBlur(i, ph.number)}
              error={errors[`phone${i}`]}
              help="Diez dígitos."
            />
          </div>
          <Btn kind="ghost" onClick={() => onRemove(i)} aria-label={`Quitar teléfono ${i + 1}`} style={{ height: 44, padding: "0 14px", fontSize: 14 }}>Quitar</Btn>
        </div>
      ))}
      {phones.length < MAX_PHONES && (
        <div>
          <Btn kind="ghost" icon="plus" onClick={() => onChange([...phones, { kind: "celular", number: "" }])} style={{ height: 44, fontSize: 14 }}>
            Agregar teléfono
          </Btn>
        </div>
      )}
    </div>
  );
}

function AddressFields({
  address,
  postalCodeError: cpError,
  onChange,
  onPostalCodeBlur,
}: {
  address: StudentAddress;
  postalCodeError: string | undefined;
  onChange: (next: StudentAddress) => void;
  onPostalCodeBlur: () => void;
}): ReactElement {
  const field = (key: keyof StudentAddress) => (value: string) => onChange({ ...address, [key]: value });
  return (
    <>
      <TextField label="Calle" value={address.street} onChange={field("street")} />
      {/* One logical entity: the number of a street address. */}
      <div style={{ display: "flex", gap: 10 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <TextField label="Número exterior" value={address.extNumber} onChange={field("extNumber")} />
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <TextField label="Número interior" value={address.intNumber} onChange={field("intNumber")} />
        </div>
      </div>
      <TextField label="Colonia" value={address.neighborhood} onChange={field("neighborhood")} />
      <TextField label="Código postal" value={address.postalCode} onChange={field("postalCode")} onBlur={onPostalCodeBlur} error={cpError} help="Cinco dígitos." />
      <TextField label="Municipio o alcaldía" value={address.municipality} onChange={field("municipality")} />
      <SelectField label="Estado" value={address.state} onChange={field("state")} options={MEXICAN_STATES.map((s) => ({ value: s, label: s }))} />
    </>
  );
}

function MedicalFields({
  record,
  disabled,
  onChange,
}: {
  record: MedicalRecord;
  disabled: boolean;
  onChange: (next: MedicalRecord) => void;
}): ReactElement {
  return (
    <>
      <SelectField
        label="Tipo de sangre"
        value={record.bloodType}
        onChange={(v) => onChange({ ...record, bloodType: v })}
        options={BLOOD_TYPES.map((b) => ({ value: b, label: b }))}
        disabled={disabled}
      />
      <TextArea label="Alergias" value={record.allergies} onChange={(v) => onChange({ ...record, allergies: v })} disabled={disabled} help="A medicamentos, alimentos o lo que sea. Puede quedar vacío." />
      <TextArea label="Padecimientos" value={record.conditions} onChange={(v) => onChange({ ...record, conditions: v })} disabled={disabled} help="Enfermedades o condiciones que el colegio debe conocer." />
      {disabled && <p style={{ fontSize: 13, color: "var(--text-faint)" }}>Puedes ver estos datos, pero no cambiarlos.</p>}
    </>
  );
}

/**
 * The photo: from a file or from the computer's camera, shrunk to 800 px in the
 * browser and sent only when the file is saved.
 */
function PhotoEditor({
  name,
  photo,
  onChange,
}: {
  name: string;
  photo: Blob | null;
  onChange: (change: PhotoChange) => void;
}): ReactElement {
  const [preview, setPreview] = useState<string | undefined>(undefined);
  const [camera, setCamera] = useState(false);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const fileInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (photo === null) {
      setPreview(undefined);
      return;
    }
    const objectUrl = URL.createObjectURL(photo);
    setPreview(objectUrl);
    return () => URL.revokeObjectURL(objectUrl);
  }, [photo]);

  const take = async (source: Blob | HTMLVideoElement): Promise<void> => {
    try {
      onChange({ kind: "replace", image: await resizeToJpeg(source) });
      setProblem(undefined);
    } catch {
      setProblem("No pudimos leer la imagen. Prueba con otra.");
    }
  };

  const chosen = (e: ChangeEvent<HTMLInputElement>): void => {
    const file = e.target.files?.[0];
    e.target.value = ""; // the same file chosen twice still fires
    if (file === undefined) return;
    if (!ACCEPTED_PHOTO_TYPES.includes(file.type)) {
      setProblem("La foto tiene que ser JPG o PNG.");
      return;
    }
    void take(file);
  };

  return (
    <div style={{ display: "flex", gap: 18, alignItems: "flex-start", flexWrap: "wrap" }}>
      <div style={{ width: 140, height: 170, borderRadius: 14, overflow: "hidden", background: "var(--surface-3)", border: "1px solid var(--border)", display: "grid", placeItems: "center" }}>
        {preview !== undefined ? (
          <img src={preview} alt={`Foto de ${name}`} style={{ width: "100%", height: "100%", objectFit: "cover" }} />
        ) : (
          <span style={{ fontSize: 13, color: "var(--text-faint)", textAlign: "center", padding: 10 }}>Sin foto</span>
        )}
      </div>
      <div style={{ display: "grid", gap: 10, flex: "1 1 200px" }}>
        {camera ? (
          <CameraCapture
            onCapture={(video) => {
              setCamera(false);
              void take(video);
            }}
            onClose={() => setCamera(false)}
          />
        ) : (
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <Btn kind="ghost" icon="user" onClick={() => setCamera(true)} style={{ height: 44, fontSize: 14 }}>Tomar foto</Btn>
            <Btn kind="ghost" icon="download" onClick={() => fileInput.current?.click()} style={{ height: 44, fontSize: 14 }}>Elegir archivo</Btn>
            {photo !== null && (
              <Btn kind="ghost" onClick={() => onChange({ kind: "remove" })} style={{ height: 44, fontSize: 14 }}>Quitar foto</Btn>
            )}
          </div>
        )}
        <input ref={fileInput} type="file" accept="image/jpeg,image/png" aria-label="Subir foto" onChange={chosen} style={{ display: "none" }} />
        <p style={{ fontSize: 13, color: "var(--text-faint)" }}>JPG o PNG. Se guarda al guardar la ficha.</p>
        {problem !== undefined && <p role="alert" style={{ fontSize: 13.5, color: "var(--bad-fg)" }}>{problem}</p>}
      </div>
    </div>
  );
}

/** The computer's camera, live, until a frame is taken or the camera is closed. */
function CameraCapture({ onCapture, onClose }: { onCapture: (video: HTMLVideoElement) => void; onClose: () => void }): ReactElement {
  const video = useRef<HTMLVideoElement>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const media = typeof navigator === "undefined" ? undefined : navigator.mediaDevices;
    if (media === undefined) {
      setFailed(true);
      return;
    }
    let stream: MediaStream | undefined;
    let closed = false;
    media
      .getUserMedia({ video: { facingMode: "user" }, audio: false })
      .then((s) => {
        stream = s;
        if (closed) {
          s.getTracks().forEach((t) => t.stop());
          return;
        }
        if (video.current !== null) {
          video.current.srcObject = s;
          void video.current.play();
        }
      })
      .catch(() => setFailed(true));
    // The camera light goes off the moment this closes, whatever closed it.
    return () => {
      closed = true;
      stream?.getTracks().forEach((t) => t.stop());
    };
  }, []);

  if (failed) {
    return (
      <div style={{ display: "grid", gap: 8 }}>
        <p role="alert" style={{ fontSize: 13.5, color: "var(--bad-fg)" }}>
          No pudimos abrir la cámara. Revisa el permiso del navegador o elige un archivo.
        </p>
        <div><Btn kind="ghost" onClick={onClose} style={{ height: 44, fontSize: 14 }}>Cerrar cámara</Btn></div>
      </div>
    );
  }
  return (
    <div style={{ display: "grid", gap: 8 }}>
      <video ref={video} muted playsInline aria-label="Vista de la cámara" style={{ width: "100%", maxWidth: 320, borderRadius: 12, background: "#000" }} />
      <div style={{ display: "flex", gap: 8 }}>
        <Btn icon="check" onClick={() => video.current !== null && onCapture(video.current)} style={{ height: 44, fontSize: 14 }}>Capturar</Btn>
        <Btn kind="ghost" onClick={onClose} style={{ height: 44, fontSize: 14 }}>Cerrar cámara</Btn>
      </div>
    </div>
  );
}
