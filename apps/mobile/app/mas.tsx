import { router } from "expo-router";
import { type ReactNode } from "react";
import { View } from "react-native";
import { logout } from "@/src/api";
import { DetailHeader } from "@/src/components/DetailHeader";
import { ScreenScroll } from "@/src/components/ScreenScroll";
import { MoreScreen } from "@/src/screens/MoreScreen";
import { useAppState } from "@/src/state/AppState";
import { useTheme } from "@/src/theme";

/** "Más" module grid — pushed from the bottom bar's last entry. */
export default function MasRoute(): ReactNode {
  const theme = useTheme();
  const { refreshNotifications } = useAppState();

  // Clear the token pair (privacy on a shared family device), reset the inbox so
  // the next guardian never sees the previous one (refreshNotifications empties
  // it once the token is gone), then return to the login screen.
  const onLogout = async (): Promise<void> => {
    await logout();
    await refreshNotifications();
    router.replace("/");
  };

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <DetailHeader title="Más" />
      <ScreenScroll refreshable={false}>
        <MoreScreen onLogout={onLogout} />
      </ScreenScroll>
    </View>
  );
}
