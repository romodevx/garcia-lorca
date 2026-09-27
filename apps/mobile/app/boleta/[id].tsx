import { useLocalSearchParams } from "expo-router";
import { type ReactNode } from "react";
import { View } from "react-native";
import { DetailHeader } from "@/src/components/DetailHeader";
import { GatedScreen } from "@/src/components/GatedScreen";
import { ReportCardScreen } from "@/src/screens/ReportCardScreen";
import { useTheme } from "@/src/theme";

/** One boleta's preview + PDF — gated by `module_grades` (UX only). */
export default function BoletaRoute(): ReactNode {
  const theme = useTheme();
  const { id } = useLocalSearchParams<{ id: string }>();
  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <DetailHeader title="Boleta" />
      <GatedScreen flag="module_grades" refreshable={false}>
        <ReportCardScreen id={id} />
      </GatedScreen>
    </View>
  );
}
