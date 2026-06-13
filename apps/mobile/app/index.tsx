import { router } from "expo-router";
import { type ReactNode } from "react";
import { ScrollView } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { LoginScreen } from "@/src/screens/LoginScreen";
import { useTheme } from "@/src/theme";

/**
 * App entry: the parent login. On submit it routes into the tab navigator. Real
 * authentication lands with the IAM module; this is the design-faithful shell.
 */
export default function IndexRoute(): ReactNode {
  const theme = useTheme();
  const insets = useSafeAreaInsets();
  return (
    <ScrollView
      style={{ backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ flexGrow: 1, paddingTop: insets.top }}
      keyboardShouldPersistTaps="handled"
    >
      <LoginScreen onLogin={() => router.replace("/(tabs)")} />
    </ScrollView>
  );
}
