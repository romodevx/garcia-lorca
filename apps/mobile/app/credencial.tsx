import { type ReactNode } from "react";
import { View } from "react-native";
import { DetailHeader } from "@/src/components/DetailHeader";
import { ScreenScroll } from "@/src/components/ScreenScroll";
import { CardScreen } from "@/src/screens/CardScreen";
import { useTheme } from "@/src/theme";

/** Credencial route (task 13) — pushed from Cuenta when module_student_card is on. */
export default function CredencialRoute(): ReactNode {
  const theme = useTheme();
  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <DetailHeader title="Credencial" />
      <ScreenScroll>
        <CardScreen />
      </ScreenScroll>
    </View>
  );
}
