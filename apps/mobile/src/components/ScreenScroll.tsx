import { useCallback, useRef, useState, type ReactElement, type ReactNode } from "react";
import { RefreshControl, ScrollView, StyleSheet } from "react-native";
import { useTheme } from "../theme";
import { RefreshRegistryContext, type RefreshHandler } from "./refresh";

export interface ScreenScrollProps {
  children: ReactNode;
  /** Allow pull-to-refresh (on by default; it only appears when something can reload). */
  refreshable?: boolean;
  /** A reload of the container's own; screens can also register theirs via useRefreshHandler. */
  onRefresh?: RefreshHandler;
}

/**
 * Standard scrollable screen body: horizontal padding, generous bottom inset so
 * content clears the tab bar + toast, and a REAL pull-to-refresh: the spinner
 * stays up exactly as long as the reloads take. With nothing to reload there is
 * no pull at all — a spinner that reloads nothing would tell the parent the data
 * is fresh when it is not.
 */
export function ScreenScroll({ children, refreshable = true, onRefresh }: ScreenScrollProps): ReactElement {
  const theme = useTheme();
  const [refreshing, setRefreshing] = useState(false);
  const handlers = useRef(new Set<RefreshHandler>());
  const [handlerCount, setHandlerCount] = useState(0);

  const register = useCallback((handler: RefreshHandler) => {
    handlers.current.add(handler);
    setHandlerCount(handlers.current.size);
    return () => {
      handlers.current.delete(handler);
      setHandlerCount(handlers.current.size);
    };
  }, []);

  const runRefresh = useCallback(async (): Promise<void> => {
    setRefreshing(true);
    const all = onRefresh ? [onRefresh, ...handlers.current] : [...handlers.current];
    await Promise.allSettled(all.map((reload) => reload()));
    setRefreshing(false);
  }, [onRefresh]);

  const canRefresh = refreshable && (onRefresh !== undefined || handlerCount > 0);

  return (
    <RefreshRegistryContext.Provider value={register}>
      <ScrollView
        testID="screen-scroll"
        style={{ backgroundColor: theme.colors.bg }}
        contentContainerStyle={styles.content}
        showsVerticalScrollIndicator={false}
        refreshControl={
          canRefresh ? (
            <RefreshControl
              refreshing={refreshing}
              onRefresh={() => void runRefresh()}
              tintColor={theme.colors.primary}
              colors={[theme.colors.primary]}
            />
          ) : undefined
        }
      >
        {children}
      </ScrollView>
    </RefreshRegistryContext.Provider>
  );
}

const styles = StyleSheet.create({
  content: { paddingHorizontal: 16, paddingTop: 2, paddingBottom: 40 },
});
