import { useState, type ReactElement } from "react";
import { Image, StyleSheet, Text, View } from "react-native";
import QRCode from "react-native-qrcode-svg";
import { type FamilyCard } from "../api";
import { useAppState } from "../state/AppState";
import { useChildCard } from "../state/familyData";
import { useStudentPhoto } from "../state/useStudentPhoto";
import { useTheme } from "../theme";
import { Card } from "../ui";
import { AsyncErrorState, AsyncLoading } from "./AsyncStates";

const QR_SIZE = 200;
const PHOTO_SIZE = 96;
const LOGO_HEIGHT = 48;
/** assets/logo.png is 299×168 px. */
const LOGO_ASPECT = 299 / 168;

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
      <Image
        testID="card-school-logo"
        // eslint-disable-next-line @typescript-eslint/no-require-imports -- Metro resolves static image assets via require()
        source={require("../../assets/logo.png")}
        style={styles.logo}
        resizeMode="contain"
        accessibilityLabel="Centro Educativo García Lorca"
      />
      {card.school.name !== "" && (
        <Text style={[styles.school, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{card.school.name}</Text>
      )}
      <Photo key={card.studentId} studentId={card.studentId} name={card.name} />
      <Text style={[styles.name, { color: theme.colors.text, fontFamily: theme.fonts.display }]}>{card.name}</Text>
      <Text style={[styles.line, muted]}>{groupLineOf(card)}</Text>
      {card.cycle !== "" && <Text style={[styles.line, muted]}>Ciclo {card.cycle}</Text>}
      <Text style={[styles.label, muted]}>Matrícula</Text>
      <Text style={[styles.number, { color: theme.colors.text, fontFamily: theme.fonts.display }]}>{card.enrollmentNumber}</Text>
      {card.suspended === true || card.qr === "" ? (
        // Revoked and not yet reissued (the owner's decision): no code at all.
        <Text style={[styles.suspended, { color: theme.colors.bad, fontFamily: theme.fonts.uiExtra }]}>
          Credencial suspendida. Acude a la dirección de la escuela para emitir una nueva.
        </Text>
      ) : (
        // Dark modules on white whatever the theme: a scanner needs the contrast.
        <View style={styles.qrPlate} accessibilityLabel="Código QR de la credencial">
          <QRCode value={card.qr} size={QR_SIZE} color="#000000" backgroundColor="#FFFFFF" />
        </View>
      )}
      <SchoolFooter school={card.school} />
    </Card>
  );
}

/**
 * How to reach the school, under the code: CCT, address and phone, each only
 * when the school filled it in. The RFC and legal name are fiscal data and stay
 * off a card that is shown at the door; the legend is the receipt's footer.
 */
function SchoolFooter({ school }: { school: FamilyCard["school"] }): ReactElement | null {
  const theme = useTheme();
  const lines = [
    { key: "cct", text: school.cct !== "" ? `CCT ${school.cct}` : "" },
    { key: "address", text: school.address },
    { key: "phone", text: school.phone !== "" ? `Tel. ${school.phone}` : "" },
  ].filter((line) => line.text !== "");
  if (lines.length === 0) return null;
  return (
    <View testID="card-school-footer" style={[styles.footer, { borderTopColor: theme.colors.line }]}>
      {lines.map((line) => (
        <Text key={line.key} style={[styles.footerLine, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>
          {line.text}
        </Text>
      ))}
    </View>
  );
}

/** The child's photo, or their initials when there is none (a 404), it fails or is still loading. */
function Photo({ studentId, name }: { studentId: string; name: string }): ReactElement {
  const theme = useTheme();
  const { data: uri } = useStudentPhoto(studentId);
  const [failed, setFailed] = useState(false);
  if (failed || uri === null) {
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
      source={{ uri }}
      onError={() => setFailed(true)}
      style={styles.photo}
    />
  );
}

/** The group in full when the gateway names it, else "PRIMARIA · A". */
function groupLineOf(card: FamilyCard): string {
  if (card.groupLabel !== undefined && card.groupLabel !== "") return card.groupLabel;
  return [card.level, card.gradeGroup].filter((x) => x !== "").join(" · ");
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
  logo: { width: LOGO_HEIGHT * LOGO_ASPECT, height: LOGO_HEIGHT },
  school: { fontSize: 14, textAlign: "center", marginBottom: 8 },
  photo: { width: PHOTO_SIZE, height: PHOTO_SIZE, borderRadius: PHOTO_SIZE / 2, marginBottom: 8 },
  initials: { alignItems: "center", justifyContent: "center" },
  initialsText: { fontSize: 32 },
  name: { fontSize: 22, textAlign: "center" },
  line: { fontSize: 15, textAlign: "center" },
  label: { fontSize: 13, marginTop: 10 },
  number: { fontSize: 20, letterSpacing: 1 },
  qrPlate: { marginTop: 14, padding: 12, borderRadius: 12, backgroundColor: "#FFFFFF" },
  suspended: { marginTop: 14, fontSize: 15, textAlign: "center" },
  footer: { alignSelf: "stretch", alignItems: "center", gap: 2, marginTop: 14, paddingTop: 12, borderTopWidth: StyleSheet.hairlineWidth },
  footerLine: { fontSize: 13, textAlign: "center" },
});
