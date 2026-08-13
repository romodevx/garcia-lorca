import { Stack } from "expo-router";
import { type ReactNode } from "react";
import { ActivityIndicator, View } from "react-native";
import { Providers } from "@/src/Providers";
import { useTokenBootstrap } from "@/src/boot/useTokenBootstrap";
import { useAppFonts } from "@/src/theme/useAppFonts";

/** Full-screen splash shown while a boot dependency (fonts, tokens) settles. */
function BootSplash(): ReactNode {
  return (
    <View style={{ flex: 1, alignItems: "center", justifyContent: "center" }}>
      <ActivityIndicator />
    </View>
  );
}

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
    return <BootSplash />;
  }
  return (
    <Stack screenOptions={{ headerShown: false, animation: "fade" }}>
      <Stack.Screen name="index" />
      <Stack.Screen name="(tabs)" />
    </Stack>
  );
}

/**
 * Gates the whole app on the token cache being hydrated from durable storage
 * BEFORE the providers mount. AppState's mount effect fetches the family inbox
 * with a Bearer header, so the stored access token must already be in memory —
 * otherwise a cold start would fire that first request unauthenticated (401).
 * This runs OUTSIDE <Providers> on purpose, so hydration finishes before
 * AppStateProvider's mount effect can read the token.
 */
export default function RootLayout(): ReactNode {
  const tokensReady = useTokenBootstrap();
  if (!tokensReady) {
    return <BootSplash />;
  }
  return (
    <Providers>
      <RootNavigator />
    </Providers>
  );
}
