import { type ReactNode } from "react";
import { View } from "react-native";
import { DetailHeader } from "@/src/components/DetailHeader";
import { ScreenScroll } from "@/src/components/ScreenScroll";
import { ProfileScreen } from "@/src/screens/ProfileScreen";
import { useTheme } from "@/src/theme";

/** Perfil del alumno route — pushed from Más. */
export default function PerfilRoute(): ReactNode {
  const theme = useTheme();
  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <DetailHeader title="Perfil del alumno" />
      <ScreenScroll>
        <ProfileScreen />
      </ScreenScroll>
    </View>
  );
}
