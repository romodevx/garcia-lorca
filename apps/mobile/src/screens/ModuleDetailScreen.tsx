import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { type ModuleTile } from "../data/mock";
import { Icon } from "../icons";
import { useTheme } from "../theme";
import { Button, Card, Chip, EmptyState, ListRow, TintIcon } from "../ui";

export interface ModuleDetailScreenProps {
  id: ModuleTile["id"] | string;
}

// TODO(pendiente-backend: docs — docs service is a shell, no RPCs)
// — static placeholder data. Do not delete. See .claude/docs/PLAN-mock-data-audit.md #B8.
const DOCS: ReadonlyArray<[string, string]> = [
  ["Constancia de estudios", "PDF · 180 KB"],
  ["Reglamento escolar", "PDF · 1.2 MB"],
];

// TODO(pendiente-backend: CFDI invoices — billing is out of MVP)
// — static placeholder data. Do not delete. See .claude/docs/PLAN-mock-data-audit.md #B9.
const INVOICES: ReadonlyArray<[string, string]> = [
  ["Colegiatura mayo", "CFDI · 02 jun"],
  ["Colegiatura abril", "CFDI · 02 may"],
  ["Inscripción", "CFDI · 12 ago"],
];

/** Per-module detail content (Documentos, Uniformes, Facturas, Becas). */
export function ModuleDetailScreen({ id }: ModuleDetailScreenProps): ReactElement {
  const theme = useTheme();

  if (id === "documentos") {
    return (
      <Card>
        {DOCS.map(([t, s], i) => (
          <ListRow key={t} divider={i > 0} onPress={() => undefined} accessibilityLabel={t}>
            <TintIcon name="document" tint="forest" />
            <View style={{ flex: 1 }}>
              <Text style={[styles.rowTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{t}</Text>
              <Text style={[styles.rowSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{s}</Text>
            </View>
            <Icon name="download" size={20} color={theme.colors.text3} />
          </ListRow>
        ))}
      </Card>
    );
  }

  if (id === "uniformes") {
    return (
      <>
        {/* TODO(pendiente-backend: uniform sizes — uniforms service is a shell)
            — static placeholder data. Do not delete. See .claude/docs/PLAN-mock-data-audit.md #B10. */}
        <Card padded style={styles.tile}>
          <TintIcon name="shirt" tint="lime" size={48} icon={26} />
          <View>
            <Text style={[styles.tileTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>Tallas registradas</Text>
            <Text style={[styles.rowSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>
              Playera 10 · Pants 12 · Calzado 24
            </Text>
          </View>
        </Card>
        <Button label="Ir a la tienda de uniformes" icon="arrow-up-right" style={{ marginTop: 14 }} />
      </>
    );
  }

  if (id === "facturas") {
    return (
      <Card>
        {INVOICES.map(([t, s], i) => (
          <ListRow key={t} divider={i > 0} onPress={() => undefined} accessibilityLabel={t}>
            <TintIcon name="receipt" tint="orange" />
            <View style={{ flex: 1 }}>
              <Text style={[styles.rowTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{t}</Text>
              <Text style={[styles.rowSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{s}</Text>
            </View>
            <Icon name="download" size={20} color={theme.colors.text3} />
          </ListRow>
        ))}
      </Card>
    );
  }

  if (id === "becas") {
    return (
      <>
        {/* TODO(pendiente-backend: scholarships — scholarships service is a shell, no RPCs)
            — static placeholder data. Do not delete. See .claude/docs/PLAN-mock-data-audit.md #B11. */}
        <Card style={styles.balanceHero}>
          <Text style={[styles.cap, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra }]}>Beca aplicada</Text>
          <Text style={[styles.amount, { color: theme.colors.tintPurpleInk, fontFamily: theme.fonts.display }]}>23%</Text>
          <View style={{ marginTop: 12 }}>
            <Chip kind="purple" icon="check">Vigente · Ciclo 2025–2026</Chip>
          </View>
        </Card>
        <Card padded style={{ marginTop: 12 }}>
          <Text style={[styles.body, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>
            Tu beca por desempeño académico se renueva manteniendo un promedio mínimo de 9.0. Próxima revisión: julio 2026.
          </Text>
        </Card>
      </>
    );
  }

  return <EmptyState icon="grid" title="Módulo no disponible" subtitle="Vuelve a intentarlo más tarde." />;
}

const styles = StyleSheet.create({
  rowTitle: { fontSize: 15, fontWeight: "800" },
  rowSub: { fontSize: 12.5, fontWeight: "600", marginTop: 3 },
  tile: { flexDirection: "row", alignItems: "center", gap: 12 },
  tileTitle: { fontSize: 15, fontWeight: "800" },
  balanceHero: { padding: 22, alignItems: "center" },
  cap: { fontSize: 13, fontWeight: "700", marginBottom: 4 },
  amount: { fontSize: 40, fontWeight: "800", letterSpacing: -1 },
  body: { fontSize: 13.5, fontWeight: "600", lineHeight: 20 },
});
