/**
 * The permission taxonomy the role editor offers as a checklist. This is the
 * `resource:action` catalog formalized in Phase 6 (see FLAGS.md / the plan's
 * "Decisions taken"): the admin composes roles by selecting from exactly this
 * set. The gateway validates permission strings server-side — this list is the
 * UX surface, not the security boundary.
 *
 * Grouped by resource for a scannable checklist; labels are Spanish (the UI
 * language). Keep in sync with the backend's permission-format validator.
 */

export interface PermissionDef {
  /** The `resource:action` string sent to the gateway verbatim. */
  value: string;
  /** Human label shown next to the checkbox. */
  label: string;
}

export interface PermissionGroup {
  resource: string;
  permissions: readonly PermissionDef[];
}

export const PERMISSION_CATALOG: readonly PermissionGroup[] = [
  {
    resource: "Usuarios",
    permissions: [
      { value: "users:read", label: "Ver usuarios" },
      { value: "users:write", label: "Crear y editar usuarios" },
    ],
  },
  {
    resource: "Roles",
    permissions: [
      { value: "roles:read", label: "Ver roles" },
      { value: "roles:write", label: "Crear y editar roles" },
    ],
  },
  {
    resource: "Configuración",
    permissions: [
      { value: "config:read", label: "Ver configuración académica" },
      { value: "config:write", label: "Editar configuración académica" },
    ],
  },
  {
    resource: "Estudiantes",
    permissions: [
      { value: "students:read", label: "Ver alumnos" },
      { value: "students:list", label: "Listar alumnos" },
      { value: "students:create", label: "Alta de alumnos" },
      { value: "students:update", label: "Editar alumnos" },
      { value: "students:enroll", label: "Inscribir alumnos" },
      { value: "students:notes_write", label: "Escribir notas del alumno" },
    ],
  },
  {
    resource: "Calificaciones",
    permissions: [
      { value: "grades:read", label: "Ver calificaciones" },
      { value: "grades:write", label: "Capturar calificaciones" },
    ],
  },
  {
    // The Maestro (teacher) role: self-scoped capture over ONLY the teacher's own
    // assigned groups, distinct from the school-wide grades:* above.
    resource: "Docencia",
    permissions: [
      { value: "teaching:read", label: "Calificaciones (mis grupos) — ver" },
      { value: "teaching:write", label: "Calificaciones (mis grupos) — capturar" },
      { value: "attendance_own:read", label: "Asistencia (mis grupos) — ver" },
      { value: "attendance_own:write", label: "Asistencia (mis grupos) — pasar lista" },
    ],
  },
];

/** Flat list of every permission string in the catalog (stable order). */
export const ALL_PERMISSIONS: readonly string[] = PERMISSION_CATALOG.flatMap((g) =>
  g.permissions.map((p) => p.value),
);
