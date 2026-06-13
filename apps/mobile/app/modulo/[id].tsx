import { useLocalSearchParams } from "expo-router";
import { type ReactNode } from "react";
import { View } from "react-native";
import { DetailHeader } from "@/src/components/DetailHeader";
import { ScreenScroll } from "@/src/components/ScreenScroll";
import { modules } from "@/src/data/mock";
import { ModuleDetailScreen } from "@/src/screens/ModuleDetailScreen";
import { useTheme } from "@/src/theme";

/** Per-module detail route (Documentos, Uniformes, Facturas, Becas). */
export default function ModuloRoute(): ReactNode {
  const theme = useTheme();
  const { id } = useLocalSearchParams<{ id: string }>();
  const title = modules.find((m) => m.id === id)?.title ?? "Módulo";
  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <DetailHeader title={title} />
      <ScreenScroll refreshable={false}>
        <ModuleDetailScreen id={id} />
      </ScreenScroll>
    </View>
  );
}
