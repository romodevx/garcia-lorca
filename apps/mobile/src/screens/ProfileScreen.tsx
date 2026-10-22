import { formatStudentName } from "@garcia-lorca/domain/student-name";
import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { FamilyProfile } from "../api";
import { useAppState } from "../state/AppState";
import { useChildProfile } from "../state/familyData";
import { useTheme } from "../theme";
import { Card, Chip, ListRow } from "../ui";
import { AsyncErrorState, AsyncLoading } from "./AsyncStates";
import { formatDayWithYear } from "./format";

/**
 * Perfil del alumno: the selected child's record as the school keeps it —
 * name, level, group and whether they hold a current enrollment — from
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
  if (data === null) {
    return <></>;
  }
  return <ProfileCard profile={data} />;
}

function ProfileCard({ profile }: { profile: FamilyProfile }): ReactElement {
  const theme = useTheme();
  const enrollment = profile.currentEnrollment;
  const group = (profile.groupLabel ?? "").trim() || profile.gradeGroup;
  return (
    <>
      <Text style={[styles.name, { color: theme.colors.text, fontFamily: theme.fonts.display }]}>
        {formatStudentName(profile)}
      </Text>
      <Card style={{ marginTop: 12 }}>
        <Field label="Nivel" value={profile.level} />
        <Field label="Grupo" value={group} divider />
        <ListRow divider>
          <View style={{ flex: 1 }}>
            <Text style={[styles.label, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>Inscripción</Text>
            {enrollment !== null && (
              <Text style={[styles.value, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
                {`Desde el ${formatDayWithYear(enrollment.enrolledAt)}`}
              </Text>
            )}
          </View>
          {enrollment !== null ? (
            <Chip kind="ok" icon="check">Inscrito</Chip>
          ) : (
            <Chip kind="warn">Sin inscripción vigente</Chip>
          )}
        </ListRow>
      </Card>
    </>
  );
}

function Field({ label, value, divider = false }: { label: string; value: string; divider?: boolean }): ReactElement {
  const theme = useTheme();
  return (
    <ListRow divider={divider}>
      <View style={{ flex: 1 }}>
        <Text style={[styles.label, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{label}</Text>
        <Text style={[styles.value, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{value || "—"}</Text>
      </View>
    </ListRow>
  );
}

const styles = StyleSheet.create({
  name: { fontSize: 24, fontWeight: "800", marginTop: 8, paddingHorizontal: 4 },
  label: { fontSize: 13, fontWeight: "600" },
  value: { fontSize: 16, fontWeight: "800", marginTop: 2 },
});
