import { useFlags, type FlagKey } from "@garcia-lorca/ui/flags";
import { router } from "expo-router";
import { type ReactElement } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { modules, type ModuleTile } from "../data/mock";
import { Icon } from "../icons";
import { useAppState } from "../state/AppState";
import { toChildView } from "../state/childView";
import { useTheme } from "../theme";
import { Avatar, Card, Chip, ListRow, SectionLabel, TintIcon } from "../ui";

/**
 * Maps each "Más" module tile to its feature flag. A tile whose flag is OFF is
 * NOT rendered at all — modules with no backend stay hidden until they land, so
 * the parent never sees mock data (the backend gateway is the security boundary
 * regardless). The mock content in `mock.ts` is kept future-ready, just
 * unreachable.
 */
const MODULE_FLAGS: Record<ModuleTile["id"], FlagKey> = {
  documentos: "module_docs",
  uniformes: "module_uniforms",
  facturas: "module_billing",
  becas: "module_scholarships",
};

/** Más: student profile, module grid (flag-gated), and an account list. */
export function MoreScreen({ onLogout }: { onLogout: () => void }): ReactElement {
  const theme = useTheme();
  const { child } = useAppState();
  const view = toChildView(child);

  return (
    <>
      <Card padded style={[styles.profile, { marginTop: 8 }]}>
        <Avatar initials={view.initials} size={52} />
        <View style={{ flex: 1, minWidth: 0 }}>
          <Text style={[styles.name, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]} numberOfLines={1}>
            {view.present ? view.headerName : "Portal de familias"}
          </Text>
          <Text style={[styles.teacher, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>
            {view.present ? view.subtitle : "—"}
          </Text>
        </View>
        {view.present ? <Chip kind="lime">{view.short}</Chip> : null}
      </Card>

      <ModuleGrid />

      <SectionLabel>Cuenta</SectionLabel>
      <Card>
        <ListRow onPress={() => undefined} accessibilityLabel="Perfil del alumno">
          <TintIcon name="user" tint="forest" />
          <View style={{ flex: 1 }}>
            <Text style={[styles.itemText, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>Perfil del alumno</Text>
          </View>
          <Icon name="chevron-right" size={20} stroke={2.4} color={theme.colors.text3} />
        </ListRow>
        <ListRow divider onPress={() => undefined} accessibilityLabel="Notificaciones">
          <TintIcon name="bell" tint="forest" />
          <View style={{ flex: 1 }}>
            <Text style={[styles.itemText, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>Notificaciones</Text>
          </View>
          <Icon name="chevron-right" size={20} stroke={2.4} color={theme.colors.text3} />
        </ListRow>
        <ListRow divider onPress={() => undefined} accessibilityLabel="Ayuda y soporte">
          <TintIcon name="phone" tint="forest" />
          <View style={{ flex: 1 }}>
            <Text style={[styles.itemText, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>Ayuda y soporte</Text>
          </View>
          <Icon name="chevron-right" size={20} stroke={2.4} color={theme.colors.text3} />
        </ListRow>
        <ListRow divider onPress={onLogout} accessibilityLabel="Cerrar sesión">
          <View style={[styles.logoutIcon, { backgroundColor: theme.colors.redSoft }]}>
            <Icon name="lock" size={20} color={theme.colors.tintRedInk} />
          </View>
          <View style={{ flex: 1 }}>
            <Text style={[styles.itemText, { color: theme.colors.bad, fontFamily: theme.fonts.uiExtra }]}>Cerrar sesión</Text>
          </View>
        </ListRow>
      </Card>
    </>
  );
}

/**
 * The flag-gated module tiles under their "Módulos" label. With no tile visible
 * the whole section is absent — a label over an empty grid reads as a broken
 * screen (QA 21-oct).
 */
function ModuleGrid(): ReactElement | null {
  const flags = useFlags();
  const visible = modules.filter((m) => !m.hidden && flags[MODULE_FLAGS[m.id]] === true);
  if (visible.length === 0) {
    return null;
  }
  return (
    <>
      <SectionLabel>Módulos</SectionLabel>
      <View style={styles.grid}>
        {visible.map((m) => (
          <ModuleCard key={m.id} module={m} />
        ))}
      </View>
    </>
  );
}

function ModuleCard({ module }: { module: ModuleTile }): ReactElement {
  const theme = useTheme();
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={module.title}
      onPress={() => router.push({ pathname: "/modulo/[id]", params: { id: module.id } })}
      style={({ pressed }) => [
        styles.modCard,
        {
          backgroundColor: theme.colors.surface,
          borderColor: theme.colors.line,
          transform: [{ scale: pressed ? 0.97 : 1 }],
        },
      ]}
    >
      <TintIcon name={module.icon} tint={module.tint} size={48} icon={26} />
      <View>
        <Text style={[styles.modTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{module.title}</Text>
        <Text style={[styles.modSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{module.sub}</Text>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  profile: { flexDirection: "row", alignItems: "center", gap: 14 },
  name: { fontSize: 17, fontWeight: "800" },
  teacher: { fontSize: 13, fontWeight: "600", marginTop: 2 },
  grid: { flexDirection: "row", flexWrap: "wrap", gap: 13 },
  modCard: {
    width: "47.5%",
    flexGrow: 1,
    borderWidth: 1,
    borderRadius: 20,
    padding: 18,
    gap: 12,
    minHeight: 116,
    justifyContent: "space-between",
  },
  modTitle: { fontSize: 16, fontWeight: "800" },
  modSub: { fontSize: 12, fontWeight: "600", marginTop: 2 },
  itemText: { fontSize: 15, fontWeight: "800" },
  logoutIcon: { width: 40, height: 40, borderRadius: 20, alignItems: "center", justifyContent: "center" },
});
