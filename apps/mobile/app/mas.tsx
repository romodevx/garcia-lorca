import { router } from "expo-router";
import { type ReactNode } from "react";
import { View } from "react-native";
import { DetailHeader } from "@/src/components/DetailHeader";
import { ScreenScroll } from "@/src/components/ScreenScroll";
import { MoreScreen } from "@/src/screens/MoreScreen";
import { useTheme } from "@/src/theme";

/** "Más" module grid — pushed from the bottom bar's last entry. */
export default function MasRoute(): ReactNode {
  const theme = useTheme();
  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <DetailHeader title="Más" />
      <ScreenScroll refreshable={false}>
        <MoreScreen onLogout={() => router.replace("/")} />
      </ScreenScroll>
    </View>
  );
}
