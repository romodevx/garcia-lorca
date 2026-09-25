import { useState, type ReactElement } from "react";
import { Image, StyleSheet, Text, View } from "react-native";
import QRCode from "react-native-qrcode-svg";
import { apiUrl, cardPhotoPath, getAccessToken, type FamilyCard } from "../api";
import { useAppState } from "../state/AppState";
import { useChildCard } from "../state/familyData";
import { useTheme } from "../theme";
import { Card } from "../ui";
import { AsyncErrorState, AsyncLoading } from "./AsyncStates";

const QR_SIZE = 200;
const PHOTO_SIZE = 96;

/**
 * Credencial (task 13): the selected child's virtual card — photo, name, group,
 * cycle, matrícula and the school — with a QR the school's staff scan at the
 * door. The QR carries an opaque code and nothing else; scanned by anybody
 * without a session it says nothing about the child.
 */
export function CardScreen(): ReactElement {
  const { selectedChildId, childLoading, childError, refreshChildren } = useAppState();
  const { data, loading, error, reload } = useChildCard(selectedChildId);

  // The route's header already says "Credencial".
  if (childError || error) {
    return (
      <AsyncErrorState
        title="No pudimos cargar la credencial"
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
  return <StudentCard card={data} />;
}

function StudentCard({ card }: { card: FamilyCard }): ReactElement {
  const theme = useTheme();
  const muted = { color: theme.colors.text2, fontFamily: theme.fonts.ui };
  return (
    <Card padded style={styles.card} accessibilityLabel={`Credencial de ${card.name}`}>
      {card.school.name !== "" && (
        <Text style={[styles.school, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{card.school.name}</Text>
      )}
      <Photo studentId={card.studentId} name={card.name} />
      <Text style={[styles.name, { color: theme.colors.text, fontFamily: theme.fonts.display }]}>{card.name}</Text>
      <Text style={[styles.line, muted]}>{[card.level, card.gradeGroup].filter((x) => x !== "").join(" · ")}</Text>
      {card.cycle !== "" && <Text style={[styles.line, muted]}>Ciclo {card.cycle}</Text>}
      <Text style={[styles.label, muted]}>Matrícula</Text>
      <Text style={[styles.number, { color: theme.colors.text, fontFamily: theme.fonts.display }]}>{card.enrollmentNumber}</Text>
      {/* Dark modules on white whatever the theme: a scanner needs the contrast. */}
      <View style={styles.qrPlate} accessibilityLabel="Código QR de la credencial">
        <QRCode value={card.qr} size={QR_SIZE} color="#000000" backgroundColor="#FFFFFF" />
      </View>
    </Card>
  );
}

/** The child's photo, or their initials when there is none (a 404) or it fails. */
function Photo({ studentId, name }: { studentId: string; name: string }): ReactElement {
  const theme = useTheme();
  const [failed, setFailed] = useState(false);
  const token = getAccessToken();
  if (failed || token === null) {
    return (
      <View style={[styles.photo, styles.initials, { backgroundColor: theme.colors.primarySoft }]}>
        <Text style={[styles.initialsText, { color: theme.colors.primaryInk, fontFamily: theme.fonts.display }]}>{initialsOf(name)}</Text>
      </View>
    );
  }
  return (
    <Image
      testID="card-photo"
      accessibilityLabel={`Foto de ${name}`}
      source={{ uri: apiUrl(cardPhotoPath(studentId)), headers: { Authorization: `Bearer ${token}` } }}
      onError={() => setFailed(true)}
      style={styles.photo}
    />
  );
}

/** "Zoe Villalobos" → "ZV". */
function initialsOf(name: string): string {
  return name
    .split(/\s+/)
    .filter((part) => part !== "")
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? "")
    .join("");
}

const styles = StyleSheet.create({
  card: { alignItems: "center", gap: 6 },
  school: { fontSize: 14, textAlign: "center", marginBottom: 8 },
  photo: { width: PHOTO_SIZE, height: PHOTO_SIZE, borderRadius: PHOTO_SIZE / 2, marginBottom: 8 },
  initials: { alignItems: "center", justifyContent: "center" },
  initialsText: { fontSize: 32 },
  name: { fontSize: 22, textAlign: "center" },
  line: { fontSize: 15, textAlign: "center" },
  label: { fontSize: 13, marginTop: 10 },
  number: { fontSize: 20, letterSpacing: 1 },
  qrPlate: { marginTop: 14, padding: 12, borderRadius: 12, backgroundColor: "#FFFFFF" },
});
