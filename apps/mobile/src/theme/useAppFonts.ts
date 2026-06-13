import { Baloo2_600SemiBold, Baloo2_700Bold } from "@expo-google-fonts/baloo-2";
import {
  Nunito_500Medium,
  Nunito_600SemiBold,
  Nunito_700Bold,
  Nunito_800ExtraBold,
} from "@expo-google-fonts/nunito";
import { useFonts } from "expo-font";

/**
 * Loads the app font families (Baloo 2 display + Nunito UI). The keys here must
 * match the family names referenced in `tokens.ts` (`fonts` roles).
 */
export function useAppFonts(): boolean {
  const [loaded] = useFonts({
    Baloo2_700Bold,
    Baloo2_600SemiBold,
    Nunito_700Bold,
    Nunito_600SemiBold,
    Nunito_800ExtraBold,
    Nunito_500Medium,
  });
  return loaded;
}
