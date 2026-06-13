import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { announcements, catMeta, type ChipKind } from "../data/mock";
import { Icon } from "../icons";
import { useTheme } from "../theme";
import { Button, Card, Chip, EmptyState, TintIcon } from "../ui";

export interface AnnouncementDetailScreenProps {
  id: number;
}

/** Full announcement view: category chip, title, author/time, body, CTA. */
export function AnnouncementDetailScreen({ id }: AnnouncementDetailScreenProps): ReactElement {
  const theme = useTheme();
  const ann = announcements.find((a) => a.id === id);

  if (!ann) {
    return <EmptyState icon="inbox" title="Aviso no disponible" subtitle="Es posible que se haya retirado." />;
  }

  const cat = catMeta[ann.cat];
  const chipKind: ChipKind = cat.tint === "purple" ? "purple" : cat.tint === "forest" ? "ok" : "warn";

  return (
    <>
      <View style={styles.head}>
        <TintIcon name={cat.icon} tint={cat.tint} size={46} icon={24} />
        <Chip kind={chipKind}>{cat.label}</Chip>
      </View>

      <Text style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.display }]}>{ann.title}</Text>

      <View style={styles.meta}>
        <Icon name="user" size={15} color={theme.colors.text2} />
        <Text style={[styles.from, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>{ann.from}</Text>
        <Text style={[styles.sep, { color: theme.colors.text3 }]}>·</Text>
        <Text style={[styles.time, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{ann.time}</Text>
      </View>

      <Card padded style={{ marginTop: 16 }}>
        <Text style={[styles.body, { color: theme.colors.text2, fontFamily: theme.fonts.uiRegular }]}>{ann.body}</Text>
      </Card>

      {ann.cat === "evento" ? (
        <Button label="Confirmar asistencia" icon="check" style={{ marginTop: 16 }} />
      ) : null}
    </>
  );
}

const styles = StyleSheet.create({
  head: { flexDirection: "row", alignItems: "center", gap: 10, marginBottom: 14 },
  title: { fontSize: 24, fontWeight: "700", lineHeight: 28, letterSpacing: -0.4 },
  meta: { flexDirection: "row", alignItems: "center", gap: 7, marginTop: 10 },
  from: { fontSize: 13, fontWeight: "700" },
  sep: { fontSize: 13 },
  time: { fontSize: 13, fontWeight: "600" },
  body: { fontSize: 14.5, lineHeight: 23, fontWeight: "500" },
});
