import { type ReactNode } from "react";
import { View } from "react-native";
import { DetailHeader } from "@/src/components/DetailHeader";
import { ScreenScroll } from "@/src/components/ScreenScroll";
import { EventsScreen } from "@/src/screens/EventsScreen";
import { useTheme } from "@/src/theme";

/** Eventos / agenda route — pushed from the Inicio "próximo evento" tile. */
export default function EventosRoute(): ReactNode {
  const theme = useTheme();
  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <DetailHeader title="Eventos" />
      <ScreenScroll refreshable={false}>
        <EventsScreen />
      </ScreenScroll>
    </View>
  );
}
