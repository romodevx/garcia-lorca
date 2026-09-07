import { router } from "expo-router";
import { type ReactNode } from "react";
import { ScrollView } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { signIn } from "@/src/api";
import { LoginScreen } from "@/src/screens/LoginScreen";
import { useAppState } from "@/src/state/AppState";
import { useTheme } from "@/src/theme";

/**
 * App entry: the parent login. `signIn` decides which door the credentials go
 * through — our own password route, or the identity provider when this build is
 * configured for it — and either way ends with OUR token pair stored (Bearer
 * posture, no cookies). Only then does it route into the tab navigator. A failed
 * sign-in rejects so the LoginScreen shows an inline error and nothing
 * navigates.
 */
export default function IndexRoute(): ReactNode {
  const theme = useTheme();
  const insets = useSafeAreaInsets();
  const { refreshNotifications, refreshChildren } = useAppState();

  const onSubmit = async (email: string, password: string): Promise<void> => {
    const { mustChangePassword } = await signIn(email, password);
    // TODO(pendiente-mobile: change-password) — the gateway can flag a forced
    // password rotation. A dedicated change-password screen is out of scope for
    // this pass; for now we surface the flag by still entering the app. Wire a
    // /auth/change-password screen (mirrors apps/web) before enabling temp
    // passwords for guardians.
    void mustChangePassword;
    // The child list + inbox were skipped on boot (no token then); now that login
    // stored one, load them so the guardian lands on their real child and
    // notifications, not a stuck empty/error state.
    void refreshChildren();
    void refreshNotifications();
    router.replace("/(tabs)");
  };

  return (
    <ScrollView
      style={{ backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ flexGrow: 1, paddingTop: insets.top }}
      keyboardShouldPersistTaps="handled"
    >
      <LoginScreen onSubmit={onSubmit} />
    </ScrollView>
  );
}
