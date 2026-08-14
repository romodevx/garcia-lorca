import { useLocalSearchParams } from "expo-router";
import { type ReactNode } from "react";
import { View } from "react-native";
import { DetailHeader } from "@/src/components/DetailHeader";
import { ScreenScroll } from "@/src/components/ScreenScroll";
import { AnnouncementDetailScreen } from "@/src/screens/AnnouncementDetailScreen";
import { useTheme } from "@/src/theme";

/** Announcement detail route. */
export default function AvisoRoute(): ReactNode {
  const theme = useTheme();
  const { id } = useLocalSearchParams<{ id: string }>();
  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <DetailHeader title="Aviso" />
      <ScreenScroll refreshable={false}>
        <AnnouncementDetailScreen id={id} />
      </ScreenScroll>
    </View>
  );
}
