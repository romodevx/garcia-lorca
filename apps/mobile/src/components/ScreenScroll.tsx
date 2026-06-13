import { useState, type ReactElement, type ReactNode } from "react";
import { RefreshControl, ScrollView, StyleSheet } from "react-native";
import { useTheme } from "../theme";

export interface ScreenScrollProps {
  children: ReactNode;
  /** Enable pull-to-refresh (most list/feed screens). */
  refreshable?: boolean;
}

/**
 * Standard scrollable screen body: horizontal padding, generous bottom inset so
 * content clears the tab bar + toast, and optional pull-to-refresh. The refresh
 * is a simulated round-trip (mock data) that settles after a short delay.
 */
export function ScreenScroll({ children, refreshable = true }: ScreenScrollProps): ReactElement {
  const theme = useTheme();
  const [refreshing, setRefreshing] = useState(false);

  const onRefresh = (): void => {
    setRefreshing(true);
    setTimeout(() => setRefreshing(false), 900);
  };

  return (
    <ScrollView
      style={{ backgroundColor: theme.colors.bg }}
      contentContainerStyle={styles.content}
      showsVerticalScrollIndicator={false}
      refreshControl={
        refreshable ? (
          <RefreshControl
            refreshing={refreshing}
            onRefresh={onRefresh}
            tintColor={theme.colors.primary}
            colors={[theme.colors.primary]}
          />
        ) : undefined
      }
    >
      {children}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  content: { paddingHorizontal: 16, paddingTop: 2, paddingBottom: 40 },
});
