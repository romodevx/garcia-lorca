import { type ReactElement } from "react";
import { StyleSheet, Text } from "react-native";
import { useTheme } from "../../theme";
import type { PdfDownloadState } from "../grades/usePdfDownload";

/** A download's progress or failure, in words; nothing while idle. */
export function PdfStatus({ state }: { state: PdfDownloadState }): ReactElement | null {
  const theme = useTheme();
  if (state.status === "downloading") {
    return (
      <Text accessibilityLiveRegion="polite" style={[styles.status, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>
        Descargando…
      </Text>
    );
  }
  if (state.status === "failed") {
    return (
      <Text accessibilityRole="alert" style={[styles.status, { color: theme.colors.bad, fontFamily: theme.fonts.uiSemi }]}>
        {state.message}
      </Text>
    );
  }
  return null;
}

const styles = StyleSheet.create({
  status: { fontSize: 16, fontWeight: "600", textAlign: "center" },
});
