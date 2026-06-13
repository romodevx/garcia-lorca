import { Stack } from "expo-router";
import { type ReactNode } from "react";
import { ActivityIndicator, View } from "react-native";
import { Providers } from "@/src/Providers";
import { useAppFonts } from "@/src/theme/useAppFonts";

/**
 * Root layout. Composes the app-wide providers (theme, flags, app state, toast,
 * safe area) and gates first paint on the warm/rounded font families being
 * ready, so titles never flash in a fallback face.
 *
 * Flags: on boot the real app fetches GET `<EXPO_PUBLIC_API_URL>/features` into
 * the FlagsProvider (single source). The backend gateway is the security
 * boundary; these flags are UX-only. The demo seed lives in src/flags/bootstrap.
 */
function RootNavigator(): ReactNode {
  const fontsReady = useAppFonts();
  if (!fontsReady) {
    return (
      <View style={{ flex: 1, alignItems: "center", justifyContent: "center" }}>
        <ActivityIndicator />
      </View>
    );
  }
  return (
    <Stack screenOptions={{ headerShown: false, animation: "fade" }}>
      <Stack.Screen name="index" />
      <Stack.Screen name="(tabs)" />
    </Stack>
  );
}

export default function RootLayout(): ReactNode {
  return (
    <Providers>
      <RootNavigator />
    </Providers>
  );
}
