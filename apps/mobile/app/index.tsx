import { router } from "expo-router";
import { type ReactNode } from "react";
import { ScrollView } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { login } from "@/src/api";
import { LoginScreen } from "@/src/screens/LoginScreen";
import { useTheme } from "@/src/theme";

/**
 * App entry: the parent login. It POSTs the credentials to the gateway's mobile
 * auth route (`/api/v1/auth/mobile/login`, Bearer posture), stores the returned
 * token pair, and only then routes into the tab navigator. A failed sign-in
 * rejects so the LoginScreen shows an inline error and no navigation happens.
 */
export default function IndexRoute(): ReactNode {
  const theme = useTheme();
  const insets = useSafeAreaInsets();

  const onSubmit = async (email: string, password: string): Promise<void> => {
    const { mustChangePassword } = await login(email, password);
    // TODO(pendiente-mobile: change-password) — the gateway can flag a forced
    // password rotation. A dedicated change-password screen is out of scope for
    // this pass; for now we surface the flag by still entering the app. Wire a
    // /auth/change-password screen (mirrors apps/web) before enabling temp
    // passwords for guardians.
    void mustChangePassword;
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
