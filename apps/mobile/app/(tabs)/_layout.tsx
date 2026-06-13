import { Tabs } from "expo-router";
import { type ReactNode } from "react";
import { View } from "react-native";
import { AppHeader } from "@/src/components/AppHeader";
import { TabBar } from "@/src/components/TabBar";
import { useTheme } from "@/src/theme";

/**
 * Parent-app bottom-tab navigation: Inicio, Asistencia, Notas, Cuenta, Avisos
 * (+ a "Más" entry in the bar that pushes the module grid). The persistent
 * child-context header sits above the tab content; the bar itself is the custom
 * Moderno bottom bar (TabBar).
 *
 * Module tabs are UX-gated by their flag in the screen bodies; the backend
 * gateway remains the security boundary.
 */
export default function TabsLayout(): ReactNode {
  const theme = useTheme();
  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <AppHeader />
      <Tabs
        tabBar={(props) => <TabBar {...props} />}
        screenOptions={{ headerShown: false, sceneStyle: { backgroundColor: theme.colors.bg } }}
      >
        <Tabs.Screen name="index" options={{ title: "Inicio" }} />
        <Tabs.Screen name="asistencia" options={{ title: "Asistencia" }} />
        <Tabs.Screen name="calificaciones" options={{ title: "Calificaciones" }} />
        <Tabs.Screen name="cuenta" options={{ title: "Cuenta" }} />
        <Tabs.Screen name="avisos" options={{ title: "Avisos" }} />
      </Tabs>
    </View>
  );
}
