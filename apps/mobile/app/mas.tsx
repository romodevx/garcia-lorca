import { router } from "expo-router";
import { type ReactNode } from "react";
import { View } from "react-native";
import { signOut } from "@/src/api";
import { DetailHeader } from "@/src/components/DetailHeader";
import { ScreenScroll } from "@/src/components/ScreenScroll";
import { MoreScreen } from "@/src/screens/MoreScreen";
import { useAppState } from "@/src/state/AppState";
import { useTheme } from "@/src/theme";

/** "Más" module grid — pushed from the bottom bar's last entry. */
export default function MasRoute(): ReactNode {
  const theme = useTheme();
  const { refreshNotifications, refreshChildren } = useAppState();

  // End BOTH halves of the session — our token pair and, when this build uses
  // it, the provider's own. Leaving the provider signed in would let a fresh ID
  // token be minted with no password and exchanged for a whole new session, so
  // "Cerrar sesión" would log nobody out. Then reset the child list and inbox so
  // the next guardian never sees the previous one, and return to the login
  // screen.
  const onLogout = async (): Promise<void> => {
    await signOut();
    await refreshChildren();
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
