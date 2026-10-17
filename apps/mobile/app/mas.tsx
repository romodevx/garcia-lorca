import { router } from "expo-router";
import { type ReactNode } from "react";
import { View } from "react-native";
import { signOut } from "@/src/api";
import { DetailHeader } from "@/src/components/DetailHeader";
import { ScreenScroll } from "@/src/components/ScreenScroll";
import { unregisterPush } from "@/src/push/unregister";
import { LOGIN_ROUTE } from "@/src/navigation/routes";
import { MoreScreen } from "@/src/screens/MoreScreen";
import { useAppState } from "@/src/state/AppState";
import { forgetSelectedChild } from "@/src/state/selectedChildStore";
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
  // screen. The push token goes first: its DELETE still needs the Bearer, and a
  // signed-out phone must stop receiving the child's attendance news.
  const onLogout = async (): Promise<void> => {
    await unregisterPush();
    await signOut();
    await forgetSelectedChild();
    await refreshChildren();
    await refreshNotifications();
    router.replace(LOGIN_ROUTE);
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
