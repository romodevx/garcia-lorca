import { formatStudentName } from "@garcia-lorca/domain/student-name";
import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { FamilyProfile } from "../api";
import { useAppState } from "../state/AppState";
import { useChildProfile } from "../state/familyData";
import { useTheme } from "../theme";
import { Card, EmptyState, ListRow } from "../ui";
import { AsyncErrorState, AsyncLoading } from "./AsyncStates";
import { formatDayWithYear } from "./format";

/**
 * Perfil del alumno: the selected child's record as the school keeps it —
 * name, level, group and since when they are in it — from
 * `GET /api/v1/family/students/{id}/profile`. Read-only.
 */
export function ProfileScreen(): ReactElement {
  const { selectedChildId, childLoading, childError, refreshChildren } = useAppState();
  const { data, loading, error, reload } = useChildProfile(selectedChildId);

  // The route's header already says "Perfil del alumno".
  if (childError || error) {
    return (
      <AsyncErrorState
        title="No pudimos cargar el perfil"
        subtitle="Revisa tu conexión e inténtalo de nuevo."
        onRetry={childError ? refreshChildren : reload}
      />
    );
  }
  if ((childLoading || loading) && data === null) {
    return <AsyncLoading />;
  }
  // No child selected (the family has none linked): say so, never a blank page.
  if (data === null) {
    return <EmptyState icon="user" title="Sin alumno vinculado" subtitle="Cuando la escuela vincule a tu hijo(a), aquí verás su perfil." />;
  }
  return <ProfileCard profile={data} />;
}

function ProfileCard({ profile }: { profile: FamilyProfile }): ReactElement {
  const theme = useTheme();
  const group = (profile.groupLabel ?? "").trim() || profile.gradeGroup;
  return (
    <>
      <Text style={[styles.name, { color: theme.colors.text, fontFamily: theme.fonts.display }]}>
        {formatStudentName(profile)}
      </Text>
      <Card style={{ marginTop: 12 }}>
        <Field label="Nivel" value={profile.level} />
        <Field label="Grupo" value={group} note={placementNote(profile)} divider />
      </Card>
    </>
  );
}

/**
 * Since when the child is in their group. It is the placement, not the
 * admission: the school's admission status (e.g. "Trámite" while the
 * inscription is unpaid) is not on the family profile, and an "Inscrito" read
 * from the placement contradicted it.
 */
function placementNote({ currentEnrollment }: FamilyProfile): string {
  return currentEnrollment === null ? "Sin grupo asignado" : `En el grupo desde el ${formatDayWithYear(currentEnrollment.enrolledAt)}`;
}

interface FieldProps {
  label: string;
  value: string;
  note?: string;
  divider?: boolean;
}

function Field({ label, value, note, divider = false }: FieldProps): ReactElement {
  const theme = useTheme();
  return (
    <ListRow divider={divider}>
      <View style={{ flex: 1 }}>
        <Text style={[styles.label, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{label}</Text>
        <Text style={[styles.value, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{value || "—"}</Text>
        {note !== undefined && (
          <Text style={[styles.note, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>{note}</Text>
        )}
      </View>
    </ListRow>
  );
}

const styles = StyleSheet.create({
  name: { fontSize: 24, fontWeight: "800", marginTop: 8, paddingHorizontal: 4 },
  label: { fontSize: 13, fontWeight: "600" },
  value: { fontSize: 16, fontWeight: "800", marginTop: 2 },
  note: { fontSize: 13, fontWeight: "600", marginTop: 2 },
});
